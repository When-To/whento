FROM mcr.microsoft.com/devcontainers/go:1.26-trixie

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

# Install golang-migrate with PostgreSQL support (requires tags)
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
