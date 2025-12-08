#!/bin/bash

# migrate.sh - Database migration script for WhenTo
# Usage: ./scripts/migrate.sh [up|down|reset|status|create]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Load environment variables from .env if it exists
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Default database URL if not set
if [ -z "$DATABASE_URL" ]; then
    DATABASE_URL="postgres://whento:whento@postgres:5432/whento?sslmode=disable"
    echo -e "${YELLOW}Using default DATABASE_URL: $DATABASE_URL${NC}"
fi

MIGRATIONS_PATH="./migrations"

# Function to check if migrate is installed
check_migrate() {
    if ! command -v migrate &> /dev/null; then
        echo -e "${RED}Error: golang-migrate is not installed${NC}"
        echo "Install it with: go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest"
        exit 1
    fi
}

# Function to apply all migrations
migrate_up() {
    echo -e "${GREEN}Applying all migrations...${NC}"
    migrate -path "$MIGRATIONS_PATH" -database "$DATABASE_URL" up
    echo -e "${GREEN}✓ Migrations applied successfully${NC}"
}

# Function to rollback last migration
migrate_down() {
    echo -e "${YELLOW}Rolling back last migration...${NC}"
    migrate -path "$MIGRATIONS_PATH" -database "$DATABASE_URL" down 1
    echo -e "${GREEN}✓ Migration rolled back${NC}"
}

# Function to reset database (rollback all then apply all)
migrate_reset() {
    echo -e "${YELLOW}Resetting database...${NC}"
    migrate -path "$MIGRATIONS_PATH" -database "$DATABASE_URL" down -all || true
    echo -e "${GREEN}All migrations rolled back${NC}"
    migrate -path "$MIGRATIONS_PATH" -database "$DATABASE_URL" up
    echo -e "${GREEN}✓ Database reset complete${NC}"
}

# Function to show migration status
migrate_status() {
    echo -e "${GREEN}Current migration status:${NC}"
    migrate -path "$MIGRATIONS_PATH" -database "$DATABASE_URL" version || echo "No migrations applied yet"
}

# Function to create a new migration
migrate_create() {
    if [ -z "$1" ]; then
        echo -e "${RED}Error: Migration name required${NC}"
        echo "Usage: ./scripts/migrate.sh create <migration_name>"
        exit 1
    fi

    echo -e "${GREEN}Creating new migration: $1${NC}"
    migrate create -ext sql -dir "$MIGRATIONS_PATH" -seq "$1"
    echo -e "${GREEN}✓ Migration files created${NC}"
}

# Main script
check_migrate

case "${1:-}" in
    up)
        migrate_up
        ;;
    down)
        migrate_down
        ;;
    reset)
        migrate_reset
        ;;
    status)
        migrate_status
        ;;
    create)
        migrate_create "$2"
        ;;
    *)
        echo "Usage: $0 {up|down|reset|status|create <name>}"
        echo ""
        echo "Commands:"
        echo "  up      - Apply all pending migrations"
        echo "  down    - Rollback the last migration"
        echo "  reset   - Rollback all migrations then reapply them"
        echo "  status  - Show current migration version"
        echo "  create  - Create a new migration file"
        exit 1
        ;;
esac
