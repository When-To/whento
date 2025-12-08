.PHONY: dev dev-fullstack dev-backend dev-frontend dev-db dev-app test build clean migrate-up migrate-down migrate-reset migrate-status sync docker-build docker-build-versioned docker-build-multiarch docker-test-build docker-up docker-down docker-logs docker-ps swagger swagger-generate swagger-clean docs-serve docs-validate build-licensegen keys help format format-check

# BUILD_TYPE can be 'cloud' or 'selfhosted' (default: selfhosted)
BUILD_TYPE ?= selfhosted

# Load environment variables from .env file if it exists
-include .env
export

# Default target
help:
	@echo "WhenTo - Development Commands"
	@echo ""
	@echo "Build Mode:"
	@echo "  All commands use BUILD_TYPE=selfhosted by default"
	@echo "  Override with: make <command> BUILD_TYPE=cloud"
	@echo ""
	@echo "Usage:"
	@echo "  make dev              - Start backend only on :8080"
	@echo "  make dev-fullstack    - Start backend (:5173) + frontend (:8080) concurrently"
	@echo "  make dev-backend      - Start backend on :5173 (for use with frontend)"
	@echo "  make dev-frontend     - Start frontend dev server on :8080"
	@echo "  make dev-db           - Start only database services (PostgreSQL + Redis)"
	@echo "  make test             - Run all tests"
	@echo "  make build            - Build unified WhenTo binary"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make sync             - Sync Go workspace and download dependencies"
	@echo "  make migrate-up       - Apply all database migrations"
	@echo "  make migrate-down     - Rollback last migration"
	@echo "  make migrate-reset    - Rollback and reapply migrations"
	@echo "  make migrate-status   - Show migration status"
	@echo "  make docker-build     - Build production Docker image"
	@echo "  make docker-build-versioned - Build with version tag (VERSION=x.x.x)"
	@echo "  make docker-build-multiarch - Build multi-arch image (amd64+arm64)"
	@echo "  make docker-test-build - Test Docker build locally (dry-run)"
	@echo "  make docker-up        - Start production stack with docker-compose"
	@echo "  make docker-down      - Stop production stack"
	@echo "  make docker-logs      - View production logs"
	@echo "  make docker-ps        - Show production container status"
	@echo "  make swagger          - Generate Swagger documentation from Go comments"
	@echo "  make swagger-clean    - Remove generated Swagger files"
	@echo "  make docs-serve       - Info on accessing embedded Swagger UI"
	@echo "  make build-licensegen - Build license generator tool (for e-commerce)"
	@echo "  make format           - Format all Go files with goimports"
	@echo "  make format-check     - Check Go file formatting without modifying"

# Development
dev-db:
	docker compose -f docker-compose.dev.yml up -d postgres redis
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@echo "PostgreSQL: localhost:5432"
	@echo "Redis: localhost:6379"

dev-app:
	@echo "Starting WhenTo Application on :8080 ($(BUILD_TYPE) mode)..."
	@echo "Note: In Dev Container, PostgreSQL and Redis are already running"
	go run -tags $(BUILD_TYPE) ./cmd/main.go

dev: dev-app

dev-fullstack:
	@echo "Starting Full Stack Development ($(BUILD_TYPE) mode):"
	@echo "  - Backend API on :5173"
	@echo "  - Frontend on :8080 (proxies /api to :5173)"
	@echo ""
	@echo "Access the app at http://localhost:8080"
	@echo ""
	@trap 'kill 0' SIGINT; \
	PORT=5173 go run -tags $(BUILD_TYPE) ./cmd/ & \
	cd frontend && npm run dev:$(BUILD_TYPE)

dev-backend:
	@echo "Starting backend on :5173 ($(BUILD_TYPE) mode, for use with frontend dev server)..."
	PORT=5173 go run -tags $(BUILD_TYPE) ./cmd/

dev-frontend:
	@echo "Starting frontend on :8080 (proxies /api to :5173)..."
	@cd frontend && npm run dev:$(BUILD_TYPE)

# Testing
test:
	@echo "Running all tests ($(BUILD_TYPE) mode)..."
	go test -tags $(BUILD_TYPE) ./... -v

# Building
build:
	@echo "Building WhenTo unified binary ($(BUILD_TYPE) mode)..."
	@mkdir -p bin
	CGO_ENABLED=0 go build -tags $(BUILD_TYPE) -ldflags="-s -w" -o bin/whento ./cmd
	@echo "✓ Binary built: bin/whento"

build-licensegen:
	@echo "Building License Generator tool..."
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/licensegen ./cmd/licensegen
	@echo "✓ License Generator built: bin/licensegen"
	@echo ""
	@echo "Usage:"
	@echo "  bin/licensegen keygen                    # Generate key pair"
	@echo "  bin/licensegen generate --help           # See license generation options"

clean:
	rm -rf bin/
	docker compose -f docker-compose.dev.yml down -v

# Formatting
format:
	@echo "Formatting Go files with goimports..."
	@goimports -w -local github.com/whento $$(find . -type f -name '*.go' -not -path './.go-cache/*' -not -path './vendor/*')
	@echo "✓ All Go files formatted"

format-check:
	@echo "Checking Go file formatting..."
	@UNFORMATTED=$$(goimports -l -local github.com/whento $$(find . -type f -name '*.go' -not -path './.go-cache/*' -not -path './vendor/*')); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "The following files need formatting:"; \
		echo "$$UNFORMATTED"; \
		echo ""; \
		echo "Run 'make format' to fix formatting"; \
		exit 1; \
	fi
	@echo "✓ All Go files are properly formatted"

# Go workspace sync (fixes "is not in your go.mod" errors in VS Code)
sync:
	@echo "Syncing Go workspace..."
	go work sync
	@echo "Downloading dependencies..."
	go mod download
	@echo ""
	@echo "✓ Workspace synced! Now reload VS Code's Go Language Server:"
	@echo "  Ctrl+Shift+P → 'Go: Restart Language Server'"

# Migrations (using golang-migrate directly)
migrate-build:
	@echo "Building $(BUILD_TYPE) migrations..."
	@bash scripts/build-migrations.sh $(BUILD_TYPE) ./migrations-build

migrate-up: migrate-build
	migrate -path ./migrations-build -database "$$DATABASE_URL" up
	@rm -rf ./migrations-build

migrate-down: migrate-build
	migrate -path ./migrations-build -database "$$DATABASE_URL" down 1
	@rm -rf ./migrations-build

migrate-reset: migrate-build
	migrate -path ./migrations-build -database "$$DATABASE_URL" down
	migrate -path ./migrations-build -database "$$DATABASE_URL" up
	@rm -rf ./migrations-build

migrate-status: migrate-build
	@echo "Checking migration status..."
	@migrate -path ./migrations-build -database "$$DATABASE_URL" version || echo "No migrations applied yet"
	@rm -rf ./migrations-build

# Docker Production
docker-build:
	@echo "Building Docker image: whento:latest ($(BUILD_TYPE) mode)"
	docker build -t whento:latest \
		--build-arg VERSION=latest \
		--build-arg BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
		--build-arg VCS_REF=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown") \
		--build-arg BUILD_TYPE=$(BUILD_TYPE) \
		.
	@echo "✓ Image built successfully"

docker-build-versioned:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make docker-build-versioned VERSION=1.0.0"; \
		exit 1; \
	fi
	@echo "Building Docker image: whento:$(VERSION) ($(BUILD_TYPE) mode)"
	docker build -t whento:$(VERSION) -t whento:latest \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
		--build-arg VCS_REF=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown") \
		--build-arg BUILD_TYPE=$(BUILD_TYPE) \
		.
	@echo "✓ Image whento:$(VERSION) built successfully"

docker-build-multiarch:
	@echo "Building multi-architecture Docker image (amd64 + arm64, $(BUILD_TYPE) mode)..."
	@echo "Note: This requires Docker Buildx and may take 10-15 minutes"
	@echo ""
	@# Create buildx builder if it doesn't exist
	@docker buildx inspect whento-builder >/dev/null 2>&1 || docker buildx create --name whento-builder --use
	@docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(shell git describe --tags --always 2>/dev/null || echo "dev") \
		--build-arg BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
		--build-arg VCS_REF=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown") \
		--build-arg BUILD_TYPE=$(BUILD_TYPE) \
		-t whento:latest \
		--load \
		.
	@echo "✓ Multi-arch image built successfully"

docker-test-build:
	@echo "Testing Docker build (simulating CI/CD workflow, $(BUILD_TYPE) mode)..."
	@echo ""
	@echo "Build Arguments:"
	@echo "  VERSION: $(shell git describe --tags --always 2>/dev/null || echo "dev")"
	@echo "  BUILD_DATE: $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")"
	@echo "  VCS_REF: $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")"
	@echo "  BUILD_TYPE: $(BUILD_TYPE)"
	@echo ""
	@echo "Starting build..."
	docker build \
		--build-arg VERSION=$(shell git describe --tags --always 2>/dev/null || echo "dev") \
		--build-arg BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
		--build-arg VCS_REF=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown") \
		--build-arg BUILD_TYPE=$(BUILD_TYPE) \
		-t whento:test \
		.
	@echo ""
	@echo "✓ Build successful!"
	@echo ""
	@echo "Inspecting image metadata:"
	@docker inspect whento:test | grep -A 10 "Labels"
	@echo ""
	@echo "Image size:"
	@docker images whento:test --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
	@echo ""
	@echo "To test the image:"
	@echo "  docker run --rm -p 8080:8080 whento:test"

docker-up:
	@echo "Starting WhenTo production stack..."
	@if [ ! -f .env ]; then \
		echo "Warning: .env file not found. Creating from .env.example..."; \
		cp .env.example .env; \
		echo "Please edit .env with your configuration before proceeding."; \
		exit 1; \
	fi
	docker compose up -d
	@echo ""
	@echo "✓ WhenTo is starting..."
	@echo "  - App: http://localhost:8080"
	@echo "  - PostgreSQL: localhost:5432"
	@echo "  - Redis: localhost:6379"
	@echo ""
	@echo "View logs: make docker-logs"

docker-down:
	@echo "Stopping WhenTo production stack..."
	docker compose down
	@echo "✓ Stack stopped"

docker-logs:
	docker compose logs -f app

docker-ps:
	docker compose ps

# Documentation
swagger-generate:
	@echo "Generating Swagger documentation from Go comments..."
	swag init -g cmd/main.go -o docs/swagger --parseInternal --generatedTime=false
	@echo "✓ Swagger documentation generated in docs/swagger/"
	@echo ""
	@echo "Files created:"
	@echo "  - docs/swagger/swagger.json"
	@echo "  - docs/swagger/swagger.yaml"
	@echo "  - docs/swagger/docs.go"

swagger-clean:
	@echo "Cleaning generated Swagger files..."
	@rm -rf docs/swagger
	@echo "✓ Swagger files cleaned"

swagger: swagger-generate

# Aliases for compatibility
docs-serve:
	@echo "Note: Swagger is now embedded in the application!"
	@echo "Start the app with 'make dev' and visit http://localhost:8080/swagger/"
	@echo ""
	@echo "Or generate static docs with 'make swagger-generate'"

docs-validate: swagger-generate
	@echo "✓ Swagger documentation generated successfully (validation passed)"
