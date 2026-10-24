#!/bin/bash
# WhenTo - Collaborative event calendar for self-hosted environments
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1

# init-db.sh - Initialize a database by applying pending migrations (non-destructive).
#
# Usage: ./scripts/init-db.sh [--build-type selfhosted|cloud] [--reset] [--yes --confirm-database <name>]
#
# Normal run: resolves ONE canonical DATABASE_URL, connects, and applies pending
# migrations with `up`. Tables existing does NOT trigger a reset — an initialized
# database is a normal upgrade case and an empty schema is a normal first-run
# case. A genuine reset is only ever performed through `--reset`, which delegates
# to the guarded reset in scripts/migrate.sh (explicit consent required; never
# silently chosen because tables exist).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck disable=SC1091
. "$SCRIPT_DIR/migration-common.sh" || exit $?

BUILD_TYPE="${BUILD_TYPE:-selfhosted}"
RESET=0
EXTRA_ARGS=()

while [ $# -gt 0 ]; do
    case "$1" in
        --build-type)
            if [ $# -lt 2 ]; then
                echo 'error: --build-type needs a value' >&2
                exit 2
            fi
            BUILD_TYPE="$2"
            shift 2
            ;;
        --reset)
            RESET=1
            shift
            ;;
        --yes)
            EXTRA_ARGS+=(--yes)
            shift
            ;;
        --confirm-database)
            if [ $# -lt 2 ]; then
                echo 'error: --confirm-database needs a value' >&2
                exit 2
            fi
            EXTRA_ARGS+=(--confirm-database "$2")
            shift 2
            ;;
        --confirm-database=*)
            EXTRA_ARGS+=("$1")
            shift
            ;;
        -h | --help)
            echo "Usage: $0 [--build-type selfhosted|cloud] [--reset] [--yes --confirm-database <name>]"
            echo ""
            echo "Applies pending migrations to the connected database. Never resets by default."
            echo "--reset delegates to the guarded reset (explicit consent required)."
            exit 0
            ;;
        *)
            echo "error: unknown option '$1'" >&2
            exit 2
            ;;
    esac
done

migration_resolve_database_url

# Confirm we can reach the database before doing anything (read-only).
identity="$(migration_connected_identity)"
if [ "$identity" = "unknown" ]; then
    echo "error: cannot connect to the database (DATABASE_URL is redacted: $(migration_redact_url "$MIGRATION_DATABASE_URL"))" >&2
    exit 1
fi

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}Connected to PostgreSQL: $identity${NC}"

# --reset delegates to the guarded reset wrapper: consent is always required.
if [ "$RESET" = "1" ]; then
    # shellcheck disable=SC2086
    bash "$SCRIPT_DIR/migrate.sh" --build-type "$BUILD_TYPE" ${EXTRA_ARGS[@]+"${EXTRA_ARGS[@]}"} reset
    exit 0
fi

echo -e "${YELLOW}Applying pending migrations (up) — existing data is preserved.${NC}"
bash "$SCRIPT_DIR/migrate.sh" --build-type "$BUILD_TYPE" up

echo -e "${GREEN}✓ Migrations applied. Database tables:${NC}"
psql "$MIGRATION_DATABASE_URL" -c "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;"
