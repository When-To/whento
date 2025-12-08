# WhenTo Cloud - Dockerfile for SaaS version with Stripe integration
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

# Build stage
FROM golang:1.25-alpine3.22 AS builder

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

# Build with cloud tag
ARG VERSION=dev
RUN go build \
    -tags cloud \
    -ldflags "-X main.Version=${VERSION} -X main.BuildType=cloud" \
    -o whento \
    ./cmd/

# Download migrate CLI (use BUILDPLATFORM to avoid QEMU issues)
FROM --platform=$BUILDPLATFORM alpine:3.19 AS migrate-builder

ARG TARGETARCH

# Download golang-migrate binary for target architecture
RUN apk add --no-cache curl && \
    curl -fsSL https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-${TARGETARCH}.tar.gz | tar xvz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata openssl netcat-openbsd

WORKDIR /app

# Copy migrate CLI
COPY --from=migrate-builder /usr/local/bin/migrate /usr/local/bin/migrate

# Copy binary
COPY --from=builder /build/whento /app/whento

# Copy cloud-specific migrations
COPY --from=builder /build/migrations-build /app/migrations

# Copy frontend build
COPY frontend/dist /app/web

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
