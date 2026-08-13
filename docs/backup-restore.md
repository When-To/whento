# Backup and restore

A backup procedure that has never been restored is not a backup. It is an
untested script and a belief. This page therefore ends with a restore drill, and
that drill is the part that matters: run it, on a schedule, or assume you have
nothing.

Everything below is written for the stack in
[`docker-compose.yml`](../docker-compose.yml), which runs **PostgreSQL 16**
(`postgres:16-alpine`) and **Redis 7** (`redis:7-alpine`).

---

## 1. What has to be backed up

| Volume          | Contents                                                    | Backed up?                       | Losing it means                                                                    |
| --------------- | ----------------------------------------------------------- | -------------------------------- | ---------------------------------------------------------------------------------- |
| `postgres_data` | Every user, calendar, participant, availability, ICS feed.   | **Yes.** This is the instance.   | Total data loss.                                                                   |
| `whento_keys`   | The RSA-4096 JWT signing key pair generated on first start.  | **Yes.** Small, changes rarely.  | Every signed-in user is logged out. Participant links keep working.                |
| `redis_data`    | Cache, rate-limit buckets, short-lived authentication state. | No. Rebuilt from Postgres.       | A cold cache, empty rate-limit counters, and the caveat in §6.                     |
| `.env`          | Database and Redis passwords, SMTP credentials.              | **Yes**, in a secrets manager.   | A restored volume you cannot start the application against.                        |

`postgres_data` and `whento_keys` must be captured **as a pair**. Restoring the
database with a different key pair works, it just logs everybody out at the worst
possible moment.

### The volumes are local, and `down -v` deletes them

> **⚠️ `docker compose down -v` destroys `postgres_data`, `redis_data` and
> `whento_keys`.** All three are `driver: local` named volumes. There is no
> recycle bin, no snapshot, and no confirmation prompt. The command that stops
> the stack safely is `docker compose down` — without `-v`.

The same applies to `docker volume rm`, to `docker volume prune` once the stack
is removed, and to deleting the project directory when the volume names are
derived from it. Compose prefixes volume names with the project name (the
directory name by default), so the real names look like
`whento_postgres_data`. Confirm yours before you script anything against them:

```bash
docker volume ls --filter name=postgres_data
```

---

## 2. Daily backup

The dump runs inside the `postgres` container, so it always uses a `pg_dump`
that matches the server, and the password never appears on the host's command
line or in the host's shell history — it is read from the container's own
environment.

```bash
#!/bin/sh
# /usr/local/bin/whento-backup.sh
set -eu

COMPOSE_DIR=/opt/whento          # directory holding docker-compose.yml and .env
DEST=/var/backups/whento
STAMP=$(date -u +%Y%m%dT%H%M%SZ)

mkdir -p "$DEST"
cd "$COMPOSE_DIR"

# Database: custom format (-Fc). It is compressed, restorable table by table,
# and readable by pg_restore, which is what makes the verification in §4 cheap.
docker compose exec -T postgres sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  > "$DEST/whento-$STAMP.dump"

# JWT signing keys: a tar of the volume, taken through a throwaway container so
# the volume never has to be found on the host filesystem.
docker run --rm \
  -v whento_keys:/keys:ro \
  -v "$DEST":/backup \
  alpine:3.24 tar czf "/backup/whento-keys-$STAMP.tar.gz" -C /keys .

chmod 600 "$DEST/whento-$STAMP.dump" "$DEST/whento-keys-$STAMP.tar.gz"

# Fail loudly on an empty dump rather than rotating a good backup out for it.
test -s "$DEST/whento-$STAMP.dump"
```

Adjust `whento_keys` to the real volume name if your project name differs
(`docker volume ls`).

Schedule it, and make sure a failure reaches a human — a cron job whose only
output is an unread mail is how backups die quietly:

```cron
17 2 * * *  /usr/local/bin/whento-backup.sh >> /var/log/whento-backup.log 2>&1 || echo "WhenTo backup FAILED" | mail -s "WhenTo backup" ops@example.com
```

`pg_dump` takes a consistent snapshot of an online database; the application does
not need to be stopped, and no user notices it.

### Encrypt anything that leaves the host

The dump contains every user's email address, every participant name, and every
calendar token. Encrypt it before it reaches remote storage:

```bash
age -r age1... < whento-$STAMP.dump > whento-$STAMP.dump.age
# or: gpg --encrypt --recipient ops@example.com whento-$STAMP.dump
```

---

## 3. Retention

A workable default, if nothing stricter applies to you:

| Tier    | Keep         | Why                                                                        |
| ------- | ------------ | -------------------------------------------------------------------------- |
| Daily   | 7 days       | Covers the usual "yesterday was fine" case.                                |
| Weekly  | 4 weeks      | Covers damage noticed a fortnight later — a bad migration, a bad delete.   |
| Monthly | 6 months     | Covers "when exactly did this record change?".                             |

At least one copy must be **off the machine that runs WhenTo**, and one copy must
be somewhere a compromise of that machine cannot reach (offline media, or object
storage with append-only / immutable retention). A backup an attacker can delete
is a backup an attacker will delete.

Retention is not only a durability question. These dumps are personal data: the
longer you keep them, the longer you carry a deletion obligation you cannot
satisfy, because a user erased from the live database still exists in every
dump. Six months of monthlies is already a long tail — do not keep years of them
by accident.

Rotation, matching the table above:

```bash
find /var/backups/whento -name 'whento-*.dump' -mtime +7 -delete
```

(Keep the weekly and monthly copies in separate directories, or the `find` above
will eat them.)

---

## 4. Verify every backup, cheaply

Take five seconds after each dump to prove the file is a dump and not a
truncated write or an error message:

```bash
docker compose exec -T postgres pg_restore --list < /var/backups/whento/whento-$STAMP.dump | head
```

If that prints a table of contents, the file's structure is intact. It does
**not** prove the data restores — only §6 does that.

---

## 5. Restore

### 5.1 Restoring into the existing instance

Losing the whole instance is the case this exists for. Work in order.

```bash
cd /opt/whento

# 1. Stop the application. Not the database: a restore into a database the
#    application is writing to produces a mixture of both.
docker compose stop app

# 2. Restore the JWT keys first, if they were lost too.
docker run --rm -v whento_keys:/keys -v /var/backups/whento:/backup:ro \
  alpine:3.24 sh -c 'rm -rf /keys/* && tar xzf /backup/whento-keys-STAMP.tar.gz -C /keys && chmod 600 /keys/private.pem'

# 3. Restore the database. --clean --if-exists drops the existing objects first,
#    so this is destructive by design: it replaces the current contents.
docker compose exec -T postgres sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --single-transaction' \
  < /var/backups/whento/whento-STAMP.dump

# 4. Redis holds nothing authoritative, but stale cache entries pointing at rows
#    that no longer exist are avoidable noise. Start clean.
docker compose restart redis

# 5. Bring the application back.
docker compose up -d app
docker compose logs -f app
```

`--single-transaction` is what makes step 3 all-or-nothing: a failure part-way
leaves the database as it was rather than half-replaced.

### 5.2 Restoring onto a new host

Same procedure, with two extra steps at the start:

1. Copy `.env` (or recreate it from your secrets manager) — the restore is
   useless without `DB_PASSWORD`, and `APP_URL` must match the DNS name users
   will actually use.
2. `docker compose up -d postgres redis` first, so the database exists and is
   healthy before `pg_restore` runs. Do **not** start `app` yet: its entrypoint
   runs `migrate up` on an empty database and creates a fresh schema, which then
   collides with the one in the dump.

---

## 6. The restore drill

Run this **monthly**, and after any change to the backup script, the Postgres
version, or the compose file. It uses the real backup file and never touches the
live database.

```bash
# 1. A scratch Postgres of the same major version, isolated from the stack.
docker run -d --name whento-restore-test \
  -e POSTGRES_PASSWORD=drill -e POSTGRES_USER=whento -e POSTGRES_DB=whento \
  postgres:16-alpine
sleep 10

# 2. Restore last night's dump into it.
docker exec -i whento-restore-test sh -c \
  'PGPASSWORD=drill pg_restore -U whento -d whento --no-owner --single-transaction' \
  < /var/backups/whento/whento-LATEST.dump

# 3. Prove the data is there, not just the schema.
docker exec -i whento-restore-test psql -U whento -d whento -c "
  SELECT 'users' AS table, count(*) FROM users
  UNION ALL SELECT 'calendars', count(*) FROM calendars
  UNION ALL SELECT 'participants', count(*) FROM participants
  UNION ALL SELECT 'availabilities', count(*) FROM availabilities;"

# 4. Prove the schema is the one the application expects: the version recorded
#    by golang-migrate, and dirty = false.
docker exec -i whento-restore-test psql -U whento -d whento \
  -c "SELECT * FROM schema_migrations;"

# 5. Clean up.
docker rm -f whento-restore-test
```

The drill passes when: `pg_restore` exits 0, the counts match the order of
magnitude you expect from production, and `schema_migrations` shows the version
your running image is at with `dirty = f`.

Write down the date of the last successful drill somewhere you will see it. A
drill that happened once, eight months ago, is worth about as much as no drill.

### Going further: restore into a real application

The strongest form of the drill points a full stack at the restored database:
copy the compose file, change the ports and the volume names, start `app`
against `whento-restore-test`, and log in. That also exercises the migration
step in the entrypoint. It costs more time; run it at least once before you
depend on this instance, and after every Postgres major upgrade.

---

## 7. What is *not* covered by these backups

- **Redis** is not backed up on purpose. It holds caches (1–5 minute TTLs),
  rate-limit counters, and short-lived authentication state. One consequence is
  worth knowing: the record that invalidates access tokens issued before a
  password change lives in Redis with a TTL equal to `JWT_ACCESS_EXPIRY`
  (15 minutes by default). Flushing Redis drops that record, so tokens issued
  just before a password change stay valid until they expire on their own.
  Passkey registration and login challenges (5-minute TTL) also live there; a
  restart makes an in-flight passkey ceremony fail, and the user simply retries.
- **Sent emails** — verification links, magic links, password resets — are not
  part of any backup. Links issued before a restore point remain valid only if
  their row survived the restore.
- **Uploaded files**: there are none. WhenTo stores no user uploads.
