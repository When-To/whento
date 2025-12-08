FROM mcr.microsoft.com/devcontainers/go:1.25-bookworm

# Node.js for the frontend
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y nodejs \
    && npm install -g npm@latest

# Configure Go to use /home/vscode/go
ENV GOPATH=/home/vscode/go
ENV PATH=$GOPATH/bin:$PATH

# Create directories for Go (as vscode user)
RUN mkdir -p /home/vscode/go/pkg /home/vscode/go/bin /home/vscode/go/src \
    && chown -R vscode:vscode /home/vscode/go

# Install Go tools as vscode user to avoid permission issues
USER vscode
RUN go install github.com/air-verse/air@latest \
    && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest \
    && go install github.com/swaggo/swag/cmd/swag@latest

# Install golang-migrate with PostgreSQL support (requires tags)
USER root
RUN apt-get update && apt-get install -y gcc musl-dev && rm -rf /var/lib/apt/lists/*
USER vscode
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

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
