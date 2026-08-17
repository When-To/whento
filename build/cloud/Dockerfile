# WhenTo Cloud - release image for the hosted deployment
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1

# This image must never be published to a public registry: see the warning in
# .github/workflows/build-cloud.yml.
#
# frontend/dist and docs/swagger are expected in the build context, exactly as
# for build/selfhosted/Dockerfile. The only differences between the two files are
# the migration set, the build tag, and the runtime LABEL block; everything else
# is deliberately identical, line for line, because Docker has no include
# mechanism and the workflows pin these paths. cmd/dockerfiles_test.go fails the
# build when the copies drift apart.

# Base images are pinned by digest as well as by tag: a floating tag can be
# re-published upstream and silently change the runtime, which makes a build
# non-reproducible. The tag stays in front of the digest so the version is still
# readable, and Dependabot (package-ecosystem: docker) keeps both in step.
# Digests resolved 2026-08-12 from registry-1.docker.io; they are the multi-arch
# index digests, so multi-platform builds keep working.

# Build stage
FROM golang:1.26-alpine3.24@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
COPY pkg/go.mod pkg/go.sum ./pkg/
RUN go mod download

# Copy source code
COPY . .

# Copy frontend build to web/dist for embedding
COPY frontend/dist ./web/dist

# Build migrations for cloud
RUN chmod +x scripts/build-migrations.sh && \
    sh scripts/build-migrations.sh cloud ./migrations-build

# Build with cloud tag.
#
# The -X targets must match the variables declared in cmd/buildinfo.go: the linker
# silently ignores -X for an unknown symbol, so a typo here costs nothing at build
# time and reports "dev" forever at runtime.
# The build type is not injected — it comes from the -tags value above.
ARG VERSION=dev
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown
RUN go build \
    -tags cloud \
    -ldflags "-X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.VCSRef=${VCS_REF}" \
    -o whento \
    ./cmd/

# Download migrate CLI (use BUILDPLATFORM to avoid QEMU issues)
FROM --platform=$BUILDPLATFORM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS migrate-builder

ARG TARGETARCH

# golang-migrate CLI, pinned by version *and* by SHA-256.
#
# The archive used to be piped straight into tar, so whatever the release CDN
# returned was unpacked and later executed without any integrity check. The
# checksums below come from the release's own sha256sum.txt and were
# re-computed from the downloaded archives; `sha256sum -c` turns a mismatch
# into a build failure instead of a silently swapped binary.
#
# The release is multi-arch, so the expected digest depends on TARGETARCH. Only
# the platforms the release workflow actually builds are mapped; anything else
# stops the build rather than skipping the verification. Keep MIGRATE_VERSION in
# step with .github/workflows/ci.yml and .devcontainer/Dockerfile — the binary
# that validates the migrations must be the one that applies them.
ARG MIGRATE_VERSION=v4.19.1
ARG MIGRATE_SHA256_AMD64=2ac648fbd1b127b69ab5a7b33cf96212178f71e22379fc50573630c6f4c7ce18
ARG MIGRATE_SHA256_ARM64=2fea2455c0f3f07cc3f4b98471c951ad1a716059574b20b6416bd1e9058751c5

RUN apk add --no-cache curl && \
    case "${TARGETARCH}" in \
    amd64) expected="${MIGRATE_SHA256_AMD64}" ;; \
    arm64) expected="${MIGRATE_SHA256_ARM64}" ;; \
    *) echo "no pinned golang-migrate checksum for TARGETARCH='${TARGETARCH}'" >&2; exit 1 ;; \
    esac && \
    curl -fsSL -o /tmp/migrate.tar.gz \
    "https://github.com/golang-migrate/migrate/releases/download/${MIGRATE_VERSION}/migrate.linux-${TARGETARCH}.tar.gz" && \
    echo "${expected}  /tmp/migrate.tar.gz" | sha256sum -c - && \
    tar -xzf /tmp/migrate.tar.gz -C /tmp migrate && \
    mv /tmp/migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate && \
    rm -f /tmp/migrate.tar.gz

# Runtime stage
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata openssl postgresql-client

WORKDIR /app

# Copy migrate CLI
COPY --from=migrate-builder /usr/local/bin/migrate /usr/local/bin/migrate

# Copy binary
COPY --from=builder /build/whento /app/whento

# Copy cloud-specific migrations
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
