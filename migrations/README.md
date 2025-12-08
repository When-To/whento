# Database Migrations

WhenTo uses a conditional migration system to support both Cloud (SaaS) and Self-hosted deployments.

## Structure

```
migrations/
├── common/          # Migrations applied to both cloud and selfhosted
│   └── 001_init.*   # Initial schema (users, calendars, etc.)
├── cloud/           # Cloud-only migrations (Stripe subscriptions)
│   └── 002_subscriptions.*
└── selfhosted/      # Self-hosted only migrations (License management)
    └── 003_licenses.*
```

## How It Works

### Docker Builds

During Docker build, the `scripts/build-migrations.sh` script combines the appropriate migrations:

- **Cloud build**: `common` + `cloud` migrations
- **Self-hosted build**: `common` + `selfhosted` migrations

The result is placed in `/app/migrations` inside the container.

### Local Development

Use Makefile commands with `BUILD_TYPE` environment variable:

```bash
# Self-hosted migrations (default)
make migrate-up

# Cloud migrations
BUILD_TYPE=cloud make migrate-up

# Check status
BUILD_TYPE=cloud make migrate-status

# Rollback
BUILD_TYPE=selfhosted make migrate-down
```

### All Migrations (Legacy)

If you need to apply all migrations (not recommended for production):

```bash
make migrate-up-all
make migrate-status-all
```

## Adding New Migrations

### Common Migration (both builds)

```bash
# Create migration files
migrate create -ext sql -dir migrations/common -seq migration_name
```

### Cloud-only Migration

```bash
# Create migration files
migrate create -ext sql -dir migrations/cloud -seq migration_name
```

### Self-hosted-only Migration

```bash
# Create migration files
migrate create -ext sql -dir migrations/selfhosted -seq migration_name
```

## Migration Naming

- **Common**: `001_init`, `004_add_notifications`, etc.
- **Cloud**: `002_subscriptions`, `005_stripe_webhooks`, etc.
- **Self-hosted**: `003_licenses`, `006_license_audit_log`, etc.

> **Note**: Migration numbers should be unique across all directories to avoid conflicts when they're combined.

## Testing

Test the build script manually:

```bash
# Test cloud build
bash scripts/build-migrations.sh cloud /tmp/test-cloud

# Test selfhosted build
bash scripts/build-migrations.sh selfhosted /tmp/test-selfhosted

# Check output
ls -la /tmp/test-cloud
ls -la /tmp/test-selfhosted
```
