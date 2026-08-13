# Rolling back a migration in production

## What happens today, whether you asked for it or not

[`scripts/docker-entrypoint.sh`](../scripts/docker-entrypoint.sh) runs, on
**every single container start**:

```sh
migrate -path /app/migrations -database "$DATABASE_URL" up
```

There is no flag to skip it and no confirmation. Pulling a new image, restarting
after a config change, or a `restart: unless-stopped` recovery all apply every
pending migration in the image before the application starts. That is convenient
for the common case and it is the reason this page exists for the uncommon one.

Two consequences follow directly:

- **The schema version is a property of the image tag.** Rolling the application
  back to an older image does *not* roll the schema back; the older image simply
  finds a newer schema and starts against it.
- **Rolling the schema back while the new image is still deployed achieves
  nothing.** The next container start re-applies the very migration you reverted.
  Pinning the image is part of the rollback, not an afterthought.

If `migrate` fails, the container exits and the application never starts. That is
deliberate: serving requests against a half-migrated schema is worse than being
down. The exit is now accompanied by an explicit `[Migrations] ERROR:` line in
the container log.

---

## Before anything: is a rollback the right move?

Reverting a migration is the *destructive* option far more often than people
expect, because a `.down.sql` restores structure, never data (see
[the limits](#the-limits-a-down-migration-is-not-an-undo) below). Work through
this first.

| Situation                                                                            | Do this                                                                                                        |
| ------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| The migration failed part-way; the database is marked dirty and the container will not start. | Repair forward: §2. Rolling back is usually not even possible until the dirty flag is cleared.                  |
| The migration applied cleanly, but the new application version is broken.            | Roll back the **image**, not the schema — if the old code tolerates the new schema (added nullable columns and new tables usually qualify). |
| The migration applied cleanly and the *schema change itself* is wrong.               | Roll forward with a new migration that fixes it. This is the default answer in production.                     |
| The migration applied cleanly, is wrong, and dropped or rewrote data.                | Neither: **restore from backup** ([backup-restore.md](backup-restore.md)). A down migration will not bring the data back. |
| The migration applied minutes ago, added structure only, and nothing has written to it yet. | A genuine `down 1` is reasonable: §3.                                                                          |

---

## 1. Always: back up first

A rollback is a schema change made under time pressure, which is exactly when a
backup is worth most.

```bash
cd /opt/whento
docker compose exec -T postgres sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  > /var/backups/whento/pre-rollback-$(date -u +%Y%m%dT%H%M%SZ).dump
```

Record the current state as well — you will want to compare afterwards:

```bash
docker compose exec -T postgres psql -U whento -d whento -c "SELECT * FROM schema_migrations;"
```

`schema_migrations` has two columns: `version` (the numeric prefix of the last
migration applied) and `dirty`.

---

## 2. Repairing a failed migration (`dirty = t`)

When a migration fails part-way, golang-migrate leaves the version marked dirty
and refuses to do anything else:

```
error: Dirty database version 14. Fix and force version.
```

The container is in a restart loop at this point, so nothing is writing to the
database — which makes this the calmest of the emergencies.

**Step 1: find out what actually got applied.** Postgres runs a multi-statement
migration as a single implicit transaction, so most failures roll themselves back
entirely and the schema is untouched. The exceptions are migrations containing
statements that cannot run inside a transaction (`CREATE INDEX CONCURRENTLY`,
`ALTER TYPE … ADD VALUE` on older servers), or that manage transactions
themselves. Read the failing `.up.sql` and check the objects it touches:

```bash
docker compose logs postgres | tail -50
docker compose exec -T postgres psql -U whento -d whento -c "\d+ calendars"
```

**Step 2, case A — nothing was applied** (the usual case). Tell migrate the
schema is back at the previous version, fix the SQL, ship a new image:

```bash
docker compose run --rm --no-deps --entrypoint sh app -c \
  'migrate -path /app/migrations -database "$DATABASE_URL" force 13'
```

The single quotes matter: `DATABASE_URL` is assembled by Compose inside the
container and is not set in your host shell. Expanding it on the host would pass
an empty `-database` and the command would fail — or worse, be retyped with the
password on the host command line.

`force` only rewrites the bookkeeping row. It runs no SQL against your tables, so
it is safe exactly to the extent that your reading in step 1 was correct.

**Step 2, case B — some statements landed.** Undo them by hand in `psql` until
the schema matches version 13, *then* `force 13`. Or, if the migration touched
data and you cannot reconstruct the previous state: restore the backup instead.

**Step 3: do not restart the old image and hope.** The broken migration is still
in it, and the next start re-runs it. Fix the migration, build, deploy.

---

## 3. Reverting one applied migration

Only after the decision table above says so.

```bash
cd /opt/whento

# 1. Stop the application, so nothing writes to a schema that is about to move
#    and nothing re-runs `migrate up` behind your back.
docker compose stop app

# 2. Back up (§1).

# 3. Revert exactly one step, using the SAME image that applied it — the
#    migration files live inside the image, at /app/migrations, and they are the
#    variant (selfhosted or cloud) that image was built for.
docker compose run --rm --no-deps --entrypoint sh app -c \
  'migrate -path /app/migrations -database "$DATABASE_URL" down 1'

# 4. Verify.
docker compose exec -T postgres psql -U whento -d whento -c "SELECT * FROM schema_migrations;"

# 5. Pin the image to the version whose migrations match this schema, in
#    docker-compose.yml:
#        image: ghcr.io/when-to/whento:v1.6.2
#    Leaving it on :latest re-applies the migration on the next start.

# 6. Start.
docker compose up -d app
docker compose logs -f app
```

`migrate down` without a number targets **every** migration, all the way to an
empty schema. Recent versions ask for confirmation first; older ones do not, and
you may not have a terminal attached either way. Always write `down 1`.

`--no-deps` keeps Compose from starting a second application container while you
work.

---

## The limits: a down migration is not an undo

`golang-migrate` reverses **schema**. It has no idea what your rows contained.
Three real examples from this repository:

- `migrations/common/010_allow_anonymous_participants.down.sql` is
  `ALTER TABLE calendars DROP COLUMN allow_anonymous_participants;`. Running it
  erases that setting for every calendar. Re-applying the `up` afterwards brings
  the column back with its default of `false` — not the values your users chose.
- `migrations/common/012_unified_ics_feed.down.sql` drops `user_unified_feeds`
  and `unified_feed_calendars`. Every unified ICS feed and every subscription in
  it is gone, permanently. The `up` recreates the tables empty.
- `migrations/selfhosted/013_drop_licensing.down.sql` faithfully recreates the
  `licenses` table with the right columns and indexes — and no rows.

So:

- **A down migration that drops a column or a table is not reversible by itself.**
  The only real undo for those is the backup taken in §1.
- **Data written since the migration may not survive the round trip.** Rows
  inserted into a table the down migration drops are lost even if you immediately
  re-apply the up.
- **Down migrations get far less testing than up migrations.** They run in CI and
  in development; they run in production about once a year. Read the actual
  `.down.sql` file before executing it — it is a few lines, and it is the thing
  you are about to run against your users' data.
- **Some migrations have no honest down.** A migration that consolidates or
  transforms data can only restore the structure, not the information it threw
  away. Where that is the case, treat "restore from backup" as the documented
  rollback and do not pretend otherwise.

---

## Reducing the number of times you need this page

- Prefer expand/contract: add nullable columns and new tables in one release,
  backfill, and only drop the old shape in a later release once the previous
  image is no longer deployed anywhere. Each step is then compatible with the
  image on either side of it, and rollback means changing a tag.
- Take a dump before deploying a release that contains migrations. The dump from
  §1, taken *before* the deploy, is the only thing that makes a destructive
  migration recoverable.
- Try the deploy against a restored copy of production first — the drill in
  [backup-restore.md](backup-restore.md) already builds one.

---

## Local development

None of the above applies to a development database. There,
`make migrate-down` (`BUILD_TYPE=selfhosted` by default) reverts one step against
the merged `migrations-build/` directory, and `make migrate-status` prints the
current version. See [`migrations/README.md`](../migrations/README.md).
