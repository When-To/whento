# Database Migrations

WhenTo uses a conditional migration system to support both Cloud (SaaS) and Self-hosted deployments.

## Structure

```
migrations/
├── common/          # Migrations applied to both cloud and selfhosted
│   └── 001_init.*   # Initial schema (users, calendars, etc.)
├── cloud/           # Cloud-only migrations
│   ├── 005_ecommerce.*
│   ├── 011_order_shop_session.*
│   └── 013_drop_billing.*
└── selfhosted/      # Self-hosted only migrations
    ├── 005_licenses.*
    └── 013_drop_licensing.*
```

Both variant directories now exist largely to undo themselves. Licensing, subscriptions
and payments were removed from the product, and `013_drop_billing` / `013_drop_licensing`
are what take the tables out of a database that already ran the earlier ones. Neither
variant has anything left to add on top of `common/`.

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

- **Common**: `001_init`, `008_notification_log`, `012_unified_ics_feed`
- **Cloud**: `005_ecommerce`, `011_order_shop_session`, `013_drop_billing`
- **Self-hosted**: `005_licenses`, `013_drop_licensing`

> **Note**: the numbering space is shared, but only one variant directory is ever copied
> into a build, so a number may be reused between `cloud/` and `selfhosted/` — `005` and
> `013` both are. It must stay unique against `common/`.

The latest common migration is `012`, so **the next common migration is `014`**: `013` is
taken by the two per-variant drop migrations.

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
