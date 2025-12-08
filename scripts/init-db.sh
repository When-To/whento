#!/bin/bash

# init-db.sh - Initialize database for WhenTo (first time setup)
# Usage: ./scripts/init-db.sh

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Load environment variables from .env if it exists
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Default database connection parameters
DB_HOST="${DB_HOST:-postgres}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-whento}"
DB_USER="${DB_USER:-whento}"
DB_PASSWORD="${DB_PASSWORD:-whento}"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  WhenTo - Database Initialization${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Check if we're in devcontainer
if [ -n "$DEVCONTAINER" ]; then
    echo -e "${GREEN}✓ Running in DevContainer${NC}"
fi

# Check PostgreSQL connection
echo -e "${YELLOW}Checking PostgreSQL connection...${NC}"
if ! PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c '\q' 2>/dev/null; then
    echo -e "${RED}Error: Cannot connect to PostgreSQL${NC}"
    echo "Connection details:"
    echo "  Host: $DB_HOST"
    echo "  Port: $DB_PORT"
    echo "  Database: $DB_NAME"
    echo "  User: $DB_USER"
    echo ""
    echo "Make sure PostgreSQL is running:"
    echo "  docker compose -f docker-compose.dev.yml up -d postgres"
    exit 1
fi

echo -e "${GREEN}✓ Connected to PostgreSQL${NC}"
echo ""

# Check if database is already initialized
echo -e "${YELLOW}Checking if database is initialized...${NC}"
TABLE_COUNT=$(PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | xargs)

if [ "$TABLE_COUNT" -gt 0 ]; then
    echo -e "${YELLOW}Warning: Database already has $TABLE_COUNT table(s)${NC}"
    read -p "Do you want to reset the database? This will delete all data! (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${YELLOW}Aborted. Database unchanged.${NC}"
        exit 0
    fi
    echo -e "${YELLOW}Resetting database...${NC}"
fi

# Run migrations
echo -e "${GREEN}Running database migrations...${NC}"
export DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

if [ -x ./scripts/migrate.sh ]; then
    ./scripts/migrate.sh reset
else
    echo -e "${YELLOW}migrate.sh not found or not executable, using migrate directly...${NC}"
    migrate -path ./migrations -database "$DATABASE_URL" up
fi

echo ""

# Show created tables
echo -e "${GREEN}Database tables created:${NC}"
PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;"

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  ✓ Database initialization complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Next steps:"
echo "  1. Generate JWT keys: ./scripts/generate-keys.sh"
echo "  2. Start services: make dev-auth"
echo ""
