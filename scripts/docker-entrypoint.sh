#!/bin/sh
set -e

# docker-entrypoint.sh - Initialize WhenTo on first run
# This script handles:
# 1. JWT key generation (if keys don't exist)
# 2. Database migrations
# 3. Starting the application

KEYS_DIR="${JWT_KEYS_DIR:-/app/keys}"
PRIVATE_KEY="$KEYS_DIR/private.pem"
PUBLIC_KEY="$KEYS_DIR/public.pem"

echo "=== WhenTo Docker Entrypoint ==="

# ============================================
# Step 0: Build DATABASE_URL if not set
# ============================================
build_database_url() {
    if [ -n "$DATABASE_URL" ]; then
        echo "[DB] Using provided DATABASE_URL"
        return 0
    fi

    # Check if we have the required DB_* variables.
    #
    # Returning non-zero here would abort the whole script: `set -e` above kills
    # it on the first failing simple command, and this function is called as
    # one. The container would then exit with no message beyond this line, even
    # though wait_for_db and run_migrations both handle an unset DATABASE_URL by
    # skipping, and the application reports the missing configuration itself
    # with a usable error. Warn and continue.
    if [ -z "$DB_HOST" ] && [ -z "$DB_USER" ]; then
        echo "[DB] WARNING: no DATABASE_URL and no DB_* variables set"
        return 0
    fi

    # Build DATABASE_URL from individual variables
    DB_HOST="${DB_HOST:-localhost}"
    DB_PORT="${DB_PORT:-5432}"
    DB_NAME="${DB_NAME:-whento}"
    DB_USER="${DB_USER:-whento}"
    DB_PASSWORD="${DB_PASSWORD:-whento}"
    DB_SSLMODE="${DB_SSLMODE:-disable}"

    export DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}"
    echo "[DB] Built DATABASE_URL from DB_* variables (host: $DB_HOST:$DB_PORT)"
}

# Build DATABASE_URL first
build_database_url

# ============================================
# Step 1: Generate JWT keys if they don't exist
# ============================================
generate_keys() {
    if [ -f "$PRIVATE_KEY" ] && [ -f "$PUBLIC_KEY" ]; then
        echo "[Keys] JWT keys already exist, skipping generation"
        return 0
    fi

    echo "[Keys] Generating JWT RSA key pair..."

    # Create keys directory if it doesn't exist
    if [ ! -d "$KEYS_DIR" ]; then
        mkdir -p "$KEYS_DIR"
        echo "[Keys] Created directory: $KEYS_DIR"
    fi

    # Generate private key (RSA 4096 bits)
    openssl genrsa -out "$PRIVATE_KEY" 4096 2>/dev/null

    # Generate public key from private key
    openssl rsa -in "$PRIVATE_KEY" -pubout -out "$PUBLIC_KEY" 2>/dev/null

    # Set proper permissions
    chmod 600 "$PRIVATE_KEY"
    chmod 644 "$PUBLIC_KEY"

    echo "[Keys] JWT keys generated successfully"
}

# ============================================
# Step 2: Wait for database to be ready
# ============================================
wait_for_db() {
    echo "[DB] Waiting for database to be ready..."

    if [ -z "$DATABASE_URL" ]; then
        echo "[DB] WARNING: DATABASE_URL not set, skipping database check"
        return 0
    fi

    MAX_RETRIES=30
    RETRY_COUNT=0

    while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
        if pg_isready -d "$DATABASE_URL" -q -t 2; then
            echo "[DB] Database is ready"
            return 0
        fi
        RETRY_COUNT=$((RETRY_COUNT + 1))
        echo "[DB] Waiting for database... ($RETRY_COUNT/$MAX_RETRIES)"
        sleep 2
    done

    echo "[DB] ERROR: Database not available after $MAX_RETRIES attempts"
    return 1
}

# ============================================
# Step 3: Run database migrations
# ============================================
run_migrations() {
    if [ -z "$DATABASE_URL" ]; then
        echo "[Migrations] WARNING: DATABASE_URL not set, skipping migrations"
        return 0
    fi

    if [ ! -d "/app/migrations" ]; then
        echo "[Migrations] No migrations directory found, skipping"
        return 0
    fi

    echo "[Migrations] Running database migrations..."

    # Run migrations using golang-migrate.
    #
    # `migrate up` exits 0 when there was nothing to apply — it prints
    # "no change" and treats it as success — so a non-zero status is a real
    # failure: an unreachable database, a dirty schema version, or SQL that did
    # not apply. The container must not start the application on top of a
    # half-migrated schema, so the failure is deliberately fatal.
    #
    # This used to be written as a bare call followed by `if [ $? -eq 0 ]`. With
    # `set -e` at the top of the file, a failing `migrate` ended the script
    # before the test could run, which made both branches unreachable and the
    # "or already up to date" fallback a comment about behaviour that could not
    # happen. Testing the command directly is what makes the message real.
    # `|| status=$?` is what captures the real exit code: after a plain
    # `if cmd; then ... fi` whose condition failed, `$?` is the status of the
    # compound statement (0), not the status of cmd.
    status=0
    migrate -path /app/migrations -database "$DATABASE_URL" up || status=$?

    if [ "$status" -eq 0 ]; then
        echo "[Migrations] Migrations are up to date"
        return 0
    fi

    echo "[Migrations] ERROR: migrate exited with status $status; refusing to start" >&2
    echo "[Migrations] Recovery procedure: docs/migration-rollback.md in the WhenTo repository" >&2
    return "$status"
}

# ============================================
# Main execution
# ============================================

# Step 1: Generate keys
generate_keys

# Step 2: Wait for database
wait_for_db

# Step 3: Run migrations
run_migrations

echo "=== Starting WhenTo Application ==="

# Execute the main application (or any command passed as argument)
exec "$@"
