# Pinned by digest as well as by tag so a re-published upstream tag cannot
# change the toolchain under an existing clone. Digest resolved 2026-08-12 from
# mcr.microsoft.com (multi-arch index digest). Dependabot's docker ecosystem
# already watches /.devcontainer and will bump tag and digest together.
FROM mcr.microsoft.com/devcontainers/go:1.26-trixie@sha256:a05706c25c951718fec16e6fed496218b01f8fdb0b0b6d16bb45fdda62645c5c

# Configure Go to use /home/vscode/go
ENV GOPATH=/home/vscode/go
ENV PATH=$GOPATH/bin:$PATH

# Create directories for Go (as vscode user)
RUN mkdir -p /home/vscode/go/pkg /home/vscode/go/bin /home/vscode/go/src \
    && chown -R vscode:vscode /home/vscode/go

# Install Go tools as vscode user to avoid permission issues
USER vscode
RUN go install github.com/air-verse/air@v1.67.4 \
    && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 \
    && go install github.com/swaggo/swag/cmd/swag@v1.16.6 \
    && go install golang.org/x/tools/cmd/goimports@v0.48.0

# Install golang-migrate with PostgreSQL support (requires tags).
#
# Built from source with `go install`, so the module and every dependency are
# verified against the Go checksum database — there is no unverified release
# tarball here, unlike the Dockerfiles, which download the prebuilt binary and
# check it against a pinned SHA-256.
# v4.19.1 is the canonical version for the whole repository: the CLI that
# validates migrations in CI must be the one that applies them in production.
USER root
RUN apt-get update && apt-get install -y gcc musl-dev && rm -rf /var/lib/apt/lists/*
USER vscode
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

# Switch back to root to install system tools
USER root
RUN apt-get update && apt-get install -y \
    postgresql-client \
    redis-tools \
    jq \
    httpie \
    && rm -rf /var/lib/apt/lists/*

# Set vscode as default user
USER vscode
