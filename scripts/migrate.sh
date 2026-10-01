#!/bin/bash
#
# migrate.sh - Database migration script for WhenTo
# Usage: ./scripts/migrate.sh [up|down|reset|status|create <scope> <name>]
#
# The source tree keeps migrations in three directories (common/, cloud/,
# selfhosted/), one per build type, because each build ships a different set.
# golang-migrate cannot address that layout directly, so every command first
# assembles the set for BUILD_TYPE (default: selfhosted) into its own temporary
# directory, runs against it, and removes it. This mirrors what `make migrate-*`
# does and keeps concurrent invocations from colliding on a shared scratch dir.

set -euo pipefail

# Locate the repository root from the script's own path, so the script works
# regardless of the caller's cwd.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Source a trusted local .env if present. `set -a` makes exported variables
# visible to `migrate` and to the expansion below without the word-splitting
# footgun of `export $(cat .env)` — values containing spaces, #, or $ survive.
if [ -f .env ]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
fi

# Default database URL if not set
if [ -z "${DATABASE_URL:-}" ]; then
    DATABASE_URL="postgres://whento:whento@postgres:5432/whento?sslmode=disable"
    echo -e "${YELLOW}Using default DATABASE_URL: $DATABASE_URL${NC}"
fi

BUILD_TYPE="${BUILD_TYPE:-selfhosted}"

# Function to check if migrate is installed
check_migrate() {
    if ! command -v migrate &> /dev/null; then
        echo -e "${RED}Error: golang-migrate is not installed${NC}" >&2
        echo "Install it with: go install golang.org/x/tools/cmd/migrate@... (see docs/development.md)" >&2
        exit 1
    fi
}

# build_migrations assembles the merge of common/ and build-specific dirs into a
# fresh temporary directory and prints its path. The caller is responsible for
# removing it (the run_* functions below install a cleanup trap).
build_migrations() {
    local tmp
    tmp="$(mktemp -d /tmp/whento-migrations.XXXXXX)"
    bash scripts/build-migrations.sh "$BUILD_TYPE" "$tmp"
    echo "$tmp"
}

# remove_dir removes a scratch migration directory, best-effort.
remove_dir() {
    rm -rf "$1"
}

# run_with_cleanup runs a migrate command against the merged set and cleans up
# afterwards no matter how the command went.
run_with_cleanup() {
    local tmp
    tmp="$(build_migrations)"
    trap 'remove_dir "$tmp"' EXIT INT TERM
    migrate -path "$tmp" -database "$DATABASE_URL" "$@"
}

# Function to apply all migrations
migrate_up() {
    echo -e "${GREEN}Applying all migrations...${NC}"
    run_with_cleanup up
    echo -e "${GREEN}✓ Migrations applied successfully${NC}"
}

# Function to rollback last migration
migrate_down() {
    echo -e "${YELLOW}Rolling back last migration...${NC}"
    run_with_cleanup down 1
    echo -e "${GREEN}✓ Migration rolled back${NC}"
}

# Function to reset database (rollback all then apply all)
#
# Deliberately destructive. `down -all` exits non-zero when the schema has never
# been migrated (a normal first run), so the failure is tolerated only there —
# everything else still aborts.
migrate_reset() {
    echo -e "${YELLOW}Resetting database (drops all tables, then reapplies)...${NC}"
    local tmp
    tmp="$(build_migrations)"
    trap 'remove_dir "$tmp"' EXIT INT TERM
    migrate -path "$tmp" -database "$DATABASE_URL" down -all || \
        echo -e "${YELLOW}No existing schema to roll back (first run).${NC}"
    migrate -path "$tmp" -database "$DATABASE_URL" up
    echo -e "${GREEN}✓ Database reset complete${NC}"
}

# Function to show migration status
migrate_status() {
    echo -e "${GREEN}Current migration status:${NC}"
    run_with_cleanup version || echo "No migrations applied yet"
}

# Function to create a new migration
#
# Usage: ./scripts/migrate.sh create <scope> <migration_name>
# scope is one of: common (default), cloud, selfhosted.
migrate_create() {
    local scope="${1:-common}"
    local name="${2:-}"
    if [ -z "$name" ]; then
        echo -e "${RED}Error: Migration name required${NC}" >&2
        echo "Usage: $0 create [common|cloud|selfhosted] <migration_name>" >&2
        exit 1
    fi

    case "$scope" in
        common|cloud|selfhosted) ;;
        *)
            echo -e "${RED}Error: scope must be common, cloud or selfhosted.${NC}" >&2
            exit 1 ;;
    esac

    echo -e "${GREEN}Creating new migration in migrations/$scope: $name${NC}"
    migrate create -ext sql -dir "./migrations/$scope" -seq "$name"
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
        migrate_create "${2:-common}" "${3:-}"
        ;;
    *)
        echo "Usage: $0 {up|down|reset|status|create [common|cloud|selfhosted] <name>}"
        echo ""
        echo "Commands:"
        echo "  up                     - Apply all pending migrations"
        echo "  down                   - Rollback the last migration"
        echo "  reset                  - Rollback all migrations then reapply them (destructive)"
        echo "  status                 - Show current migration version"
        echo "  create <scope> <name>  - Create a new migration in the given scope dir"
        exit 1
        ;;
esac
