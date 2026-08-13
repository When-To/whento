# Upgrading PostgreSQL and Redis

[`.github/dependabot.yml`](../.github/dependabot.yml) deliberately ignores major
version bumps for the `postgres` and `redis` images:

> A Postgres major upgrade cannot read the existing data volume, so it needs a
> dump/restore and a release note for self-hosted users.

This page is that procedure. Minor and patch bumps (16.4 → 16.5, `postgres:16-alpine`
staying on 16) need none of it: pull the image and restart.

The stack currently pins **`postgres:16-alpine`** and **`redis:7-alpine`** in
[`docker-compose.yml`](../docker-compose.yml).

---

## Part 1 — PostgreSQL major upgrade (16 → 17 → …)

### Why a restart is not enough

A PostgreSQL data directory has an on-disk layout tied to its major version.
A PostgreSQL 17 server started against a directory initialised by 16 refuses to
start:

```
FATAL:  database files are incompatible with server
DETAIL: The data directory was initialized by PostgreSQL version 16, which is not compatible with this version 17.
```

It does not corrupt anything — it simply will not run. But under
`restart: unless-stopped`, the container enters a crash loop, and the `app`
container, which waits on `depends_on: service_healthy`, never starts either. So
the visible symptom of "I bumped the tag" is a completely dead stack.

The way across the version boundary is a logical dump from the old server and a
restore into a freshly initialised new one. (`pg_upgrade` also exists, but it
needs both major versions' binaries in one place, which the official single-version
images do not give you. Dump and restore is slower and much harder to get wrong.)

### Before you start

- Read [backup-restore.md](backup-restore.md) and take a backup you have
  actually verified. This procedure keeps the old volume intact, so it is
  reversible — but only if nothing else goes wrong.
- Budget downtime. WhenTo is unavailable from step 2 to the end. For a small
  instance this is minutes; the dump and restore both scale with database size,
  so measure on a copy first if that matters to you.
- Do it when someone can watch it, not on a Friday evening.

### The procedure

Replace `17` with the target major version throughout.

```bash
cd /opt/whento
```

**1. Announce and stop writes.** Stop the application only; the database must
stay up to be dumped.

```bash
docker compose stop app
```

**2. Dump with the *new* version's `pg_dump`.** PostgreSQL supports dumping an
older server with a newer `pg_dump`, and that is the supported direction — never
the reverse. Run it as a throwaway container on the stack's network:

```bash
NET=$(docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}' whento-postgres)

docker run --rm --network "$NET" \
  -e PGPASSWORD="$(grep -E '^DB_PASSWORD=' .env | cut -d= -f2-)" \
  postgres:17-alpine \
  pg_dump -h postgres -U whento -d whento -Fc \
  > /var/backups/whento/pre-pg17.dump

test -s /var/backups/whento/pre-pg17.dump
```

Use your real `DB_USER` / `DB_NAME` if you changed them from the `whento`
default.

**3. Stop the database and put the old volume aside.** Renaming the volume in
the compose file is what makes this reversible: the 16 data directory is never
written to again, and rolling back is a two-line edit.

```bash
docker compose stop postgres
```

Edit `docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:17-alpine # was 16-alpine
    volumes:
      - postgres_data_v17:/var/lib/postgresql/data # was postgres_data

volumes:
  postgres_data: # keep: this is the 16 data directory, untouched
    driver: local
  postgres_data_v17:
    driver: local
```

**4. Start the new server on an empty volume.** It initialises a fresh 17 data
directory and creates the database and role from the `POSTGRES_*` variables.

```bash
docker compose up -d postgres
docker compose logs -f postgres     # wait for "database system is ready to accept connections"
```

**5. Restore.**

```bash
docker compose exec -T postgres sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --single-transaction' \
  < /var/backups/whento/pre-pg17.dump
```

**6. Check before letting the application in.**

```bash
docker compose exec -T postgres psql -U whento -d whento -c "
  SELECT 'users' AS t, count(*) FROM users
  UNION ALL SELECT 'calendars', count(*) FROM calendars
  UNION ALL SELECT 'participants', count(*) FROM participants
  UNION ALL SELECT 'availabilities', count(*) FROM availabilities;"

docker compose exec -T postgres psql -U whento -d whento -c "SELECT * FROM schema_migrations;"
```

The counts must match what the old instance had, and `schema_migrations` must
show the same version with `dirty = f`. The application's entrypoint runs
`migrate up` on start; with the correct version restored it finds nothing to do.

**7. Start the application and use it.** Not just the health endpoint — log in,
open a calendar, save an availability.

```bash
docker compose up -d app
docker compose logs -f app
```

**8. Statistics.** A restored database has no planner statistics until it is
analysed, so the first queries can be noticeably slow:

```bash
docker compose exec -T postgres psql -U whento -d whento -c "ANALYZE;"
```

**9. Only then, and not before, clean up.** Keep the old volume for at least a
week of normal use:

```bash
docker volume rm whento_postgres_data     # the PostgreSQL 16 data directory
```

### Rolling back

Until step 9, rollback is: stop the stack, point `image:` back at
`postgres:16-alpine` and the volume back at `postgres_data`, `docker compose up -d`.
Anything the application wrote after step 7 is lost — which is the reason step 7
comes with real usage rather than a week of hoping.

### After the upgrade

- Update the pin in `docker-compose.yml` **and** in
  [`docker-compose.dev.yml`](../docker-compose.dev.yml), so development runs the
  same major version as production. CI pins its own service containers in
  [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) (`postgres:16`,
  `redis:7` at the time of writing) — move those in the same change, or the
  version the tests run against stops being the version that ships.
- Lift or re-target the `postgres` entry under `package-ecosystem: docker-compose`
  in [`.github/dependabot.yml`](../.github/dependabot.yml).
- Ship a release note. See the template at the end of this page.

---

## Part 2 — Redis major upgrade (7 → 8 → …)

Much simpler, for one reason that is specific to WhenTo: **nothing in Redis is
authoritative.** It holds caches with 1–5 minute TTLs, rate-limit counters, and
short-lived authentication state. The application runs without Redis at all —
`pkg/cache` falls back to a no-op implementation — so the upgrade does not need
the data to survive.

```bash
cd /opt/whento
# Edit docker-compose.yml: image: redis:8-alpine
docker compose up -d redis
docker compose logs -f redis
```

Two things to know:

- **Starting empty is fine, and is the clean option.** `docker compose down redis`
  followed by removing `whento_redis_data` costs a cold cache for a few minutes.
  The one visible effect: the record that invalidates access tokens issued before
  a password change lives in Redis with a TTL equal to `JWT_ACCESS_EXPIRY`
  (15 minutes by default). Losing it means a token issued shortly before a
  password change stays valid until it expires on its own. Avoid flushing Redis
  in the minutes right after a password reset campaign.
- **Redis reads an older RDB file, but never a newer one.** Upgrading in place
  works; downgrading afterwards does not. If you want the option of going back,
  copy the volume first:
  ```bash
  docker run --rm -v whento_redis_data:/data:ro -v /var/backups/whento:/backup \
    alpine:3.24 tar czf /backup/redis-pre-upgrade.tar.gz -C /data .
  ```
- Check the release notes for changed defaults. WhenTo sets only `requirepass`,
  `maxmemory` and `maxmemory-policy`; anything else comes from the image's
  defaults and can change across majors.

Then lift the `redis` entry in `.github/dependabot.yml` and update
`docker-compose.dev.yml` to match.

---

## Release note template for self-hosted users

A major bump of either image is a breaking change for every self-hosted
deployment, because it is their volume and their downtime. It must not ship as
an unannounced dependency update.

```markdown
### ⚠️ Breaking: PostgreSQL 17 required

This release moves the bundled stack from PostgreSQL 16 to 17.

**A PostgreSQL 17 server cannot read a PostgreSQL 16 data directory.** Pulling
this release without acting will leave the `postgres` container restarting in a
loop, and the application will not start.

You must dump and restore before or while upgrading. The procedure, with the
exact commands, is in [docs/upgrade-postgres-redis.md](docs/upgrade-postgres-redis.md).

Expect a few minutes of downtime, longer for large instances. Take a verified
backup first ([docs/backup-restore.md](docs/backup-restore.md)); the procedure
keeps the old volume so it can be rolled back.

Staying on PostgreSQL 16 for now is supported: pin `image: postgres:16-alpine`
in your own `docker-compose.yml`. Nothing in this release depends on 17.
```

The last paragraph matters. Some operators will not have a maintenance window
this month, and an upgrade path that forces the issue is how a self-hosted
instance ends up frozen on an old release instead.
