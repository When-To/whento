#!/bin/bash
set -e

echo "🚀 Setting up WhenTo environment..."

# Ensure Go binaries are in PATH
export PATH="/home/vscode/go/bin:$PATH"

# Install/reinstall Go tools if necessary
echo "🔧 Checking Go tools..."
if ! command -v migrate &> /dev/null; then
    echo "  → Installing migrate..."
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1
fi

if ! command -v air &> /dev/null; then
    echo "  → Installing air..."
    go install github.com/air-verse/air@v1.67.1
fi

if ! command -v golangci-lint &> /dev/null; then
    echo "  → Installing golangci-lint..."
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
fi

if ! command -v swag &> /dev/null; then
    echo "  → Installing swag..."
    go install github.com/swaggo/swag/cmd/swag@v1.16.6
fi

# Initialize Go workspace if necessary
cd /workspace
if [ ! -f "go.work" ]; then
    echo "📦 Initializing Go workspace..."
    go work init
    go work use ./services/auth ./services/calendar ./services/availability ./services/ics ./services/notify ./pkg 2>/dev/null || true
fi

# Install Go dependencies
echo "📦 Installing Go dependencies..."
for service in services/*/; do
    if [ -f "$service/go.mod" ]; then
        echo "  → $service"
        (cd "$service" && go mod download)
    fi
done

# Install frontend dependencies
if [ -f "frontend/package.json" ]; then
    echo "📦 Installing frontend dependencies..."
    (cd frontend && npm install)
fi

# Run migrations
echo "🗄️ Running migrations..."
if [ -d "migrations" ]; then
    if make migrate-up; then
        echo "  ✅ Migrations applied successfully"
    else
        echo "  ⚠️  Error applying migrations (may already be applied)"
    fi
fi

# Generate JWT keys
if [ -f "scripts/generate-keys.sh" ]; then
    echo "🔑 Generating JWT keys..."
    ./scripts/generate-keys.sh
fi

# Create local config files if missing
if [ ! -f ".env" ]; then
    echo "📝 Creating .env file..."
    cp .env.example .env 2>/dev/null || echo "  → .env.example not found"
fi

echo ""
echo "✅ Environment ready!"
echo ""
echo "📋 Useful commands:"
echo "  • make dev          - Run all services in watch mode"
echo "  • make test         - Run tests"
echo "  • make migrate-up   - Apply migrations"
echo "  • make lint         - Check code"
echo ""
echo "Happy coding! 🎉"
