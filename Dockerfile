# WhenTo - self-contained image for local builds (the `make docker-*` targets)
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1

# This file is the only one of the three that builds the frontend and generates
# the Swagger spec itself, which is what makes `docker build .` work from a bare
# checkout. build/selfhosted/Dockerfile and build/cloud/Dockerfile — the two CI
# builds — instead expect frontend/dist and docs/swagger to be in the build
# context already, produced by earlier workflow jobs.
#
# Everything after that first divergence is deliberately identical across the
# three files, line for line: Docker has no include mechanism, and the two build/
# paths are pinned in the workflows, so the base image digests and the
# golang-migrate pins genuinely have to be written out three times.
# cmd/dockerfiles_test.go fails the build when the three copies drift apart.

# Base images are pinned by digest as well as by tag: a floating tag can be
# re-published upstream and silently change the runtime, which makes a build
# non-reproducible. The tag stays in front of the digest so the version is still
# readable, and Dependabot (package-ecosystem: docker) keeps both in step.
# Digests resolved 2026-08-18 from registry-1.docker.io; they are the multi-arch
# index digests, so multi-platform builds keep working.

# Frontend build stage
FROM node:24-alpine@sha256:e67514e5d0f6c46656005e1b693b2ec9d52e80b641307de684d4a015ba7a4eaf AS frontend-builder

WORKDIR /frontend

# Copy package files
COPY frontend/package*.json ./

# Install dependencies
RUN npm ci

# Copy frontend source
COPY frontend/ ./

# Build frontend for selfhosted
RUN npm run build

# Go build stage
FROM golang:1.27-alpine3.24@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Install swag CLI for swagger documentation generation.
# Pinned to the same version as .devcontainer/Dockerfile so the spec this
# image ships is the one the annotations were written against.
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Copy go mod files
COPY go.mod go.sum ./
COPY pkg/go.mod pkg/go.sum ./pkg/
RUN go mod download

# Copy source code
COPY . .

# Copy frontend build to web/dist for embedding
COPY --from=frontend-builder /frontend/dist ./web/dist

# Generate swagger documentation
RUN swag init -g cmd/main.go -o docs/swagger --parseInternal --generatedTime=false

# Build migrations for selfhosted
RUN chmod +x scripts/build-migrations.sh && \
    sh scripts/build-migrations.sh selfhosted ./migrations-build

# Build with selfhosted tag.
#
# The -X targets must match the variables declared in cmd/buildinfo.go: the linker
# silently ignores -X for an unknown symbol, so a typo here costs nothing at build
# time and reports "dev" forever at runtime.
# The build type is not injected — it comes from the -tags value above.
ARG VERSION=dev
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown
RUN go build \
    -tags selfhosted \
    -ldflags "-X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.VCSRef=${VCS_REF}" \
    -o whento \
    ./cmd/

# Build the migrate CLI. On BUILDPLATFORM, as before, but now because Go
# cross-compiles natively rather than to dodge QEMU on a download.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine3.24@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS migrate-builder

ARG TARGETARCH

# golang-migrate, built from source rather than downloaded.
#
# The release archive used to be fetched and checked against a SHA-256 kept in this
# file. That pinned what was downloaded, not what was inside it: v4.19.1 — the latest,
# and the only release in nine months — is compiled with Go 1.25.4 and links every
# database driver it supports, which Trivy reports as 45 advisories, four of them
# critical. No checksum here can patch a binary somebody else compiled, and the release
# gate blocking on it is what stopped v2.0.0.
#
# Building it puts the CLI on the same pinned toolchain as the application binary, so
# stdlib fixes arrive with the base image. `-tags postgres` links only the driver this
# project uses: the result carries exactly one dependency, lib/pq, instead of the full
# driver set that dragged in x/crypto and x/net. Integrity now comes from the Go module
# checksum database, which covers the source rather than one archive.
#
# Keep MIGRATE_VERSION in step with .github/workflows/ci.yml and .devcontainer/Dockerfile
# — the binary that validates the migrations must be the one that applies them.
# cmd/dockerfiles_test.go fails the build when they disagree.
ARG MIGRATE_VERSION=v4.19.1

RUN go mod init migratebuild && \
    go get "github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION}" && \
    CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" go build \
    -tags postgres \
    -ldflags "-s -w -X main.Version=${MIGRATE_VERSION}" \
    -o /usr/local/bin/migrate \
    github.com/golang-migrate/migrate/v4/cmd/migrate

# Runtime stage
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

LABEL org.opencontainers.image.title="WhenTo" \
    org.opencontainers.image.description="Self-hosted scheduling and calendar application" \
    org.opencontainers.image.vendor="WhenTo Contributors" \
    org.opencontainers.image.licenses="BSL-1.1"

# Install runtime dependencies.
#
# `apk upgrade` first, and not for the usual reason. `--no-cache` fetches a fresh index
# every time this layer *runs*, but a digest-pinned base means the layer itself can be
# served from the build cache for weeks without running at all. That is how the v2.0.0
# release image came to carry libpq 18.4-r0 while Alpine had long shipped 18.6-r0: the
# packages were stale, the pin was current. Upgrading makes any rebuild self-healing
# instead of merely reproducible.
RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates tzdata openssl postgresql-client

WORKDIR /app

# Copy migrate CLI
COPY --from=migrate-builder /usr/local/bin/migrate /usr/local/bin/migrate

# Copy binary
COPY --from=builder /build/whento /app/whento

# Copy selfhosted-specific migrations
COPY --from=builder /build/migrations-build /app/migrations

# The frontend is not copied into the runtime image: it is embedded in the binary
# by web/embed.go, from the web/dist the build stage above filled in. A second
# copy under /app/web used to be shipped here and nothing has ever read it —
# neither the binary, which serves the embedded FS, nor docker-entrypoint.sh.

# Copy entrypoint script
COPY --from=builder /build/scripts/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Create keys directory
RUN mkdir -p /app/keys

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Run as non-root user
RUN addgroup -g 1000 whento && \
    adduser -D -u 1000 -G whento whento && \
    chown -R whento:whento /app

USER whento

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/whento"]
