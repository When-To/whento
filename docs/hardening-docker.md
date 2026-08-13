# Hardening the Docker stack

What [`docker-compose.yml`](../docker-compose.yml) does to contain the three
services, why each choice is there, and — just as important — what it
deliberately does not do.

None of it has been verified against a running Docker daemon in the environment
where these changes were written. Everything below is derived from the images'
documented behaviour and their entrypoints. Bring it up on a staging stack before
production, and read §5 first.

---

## 1. What is applied

### All three services

| Setting                              | Effect                                                                                                                 |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| `security_opt: [no-new-privileges:true]` | Sets `PR_SET_NO_NEW_PRIVS`. A setuid binary inside the container can no longer raise privileges — it closes the usual last step of a container escape chain. |
| `cap_drop: [ALL]`                    | Starts from no Linux capabilities at all, rather than from Docker's default set of fourteen.                            |
| `pids_limit`                         | Caps the number of processes, so a fork bomb in one container cannot take the host's process table with it.             |
| `deploy.resources.limits`            | Memory and CPU ceilings. Compose v2 applies these on a plain `docker compose up`; they are not swarm-only.              |

### Per service

**`app`** — the image already runs as the unprivileged `whento` user (uid 1000)
and listens on 8080, so it needs **no capability at all**: `cap_drop: ALL` with
nothing added back. Limits: 1.0 CPU, 512 MB, 256 pids.

**`postgres`** — `cap_drop: ALL` plus exactly five capabilities back:
`CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `SETGID`, `SETUID`. The official entrypoint
starts as root, fixes ownership of the data directory, and then drops to the
`postgres` user; that sequence needs all five, and it needs them most on the
*first* start against an empty volume. Trim the list only against a test of that
case. Limits: 1.0 CPU, 1 GB, 512 pids.

**`redis`** — runs as `user: redis` from the start, so the entrypoint's ownership
pass never runs and no capability is needed. Limits: 0.5 CPU, 512 MB, 128 pids.
The memory limit is deliberately double `maxmemory 256mb`: the limit has to cover
the dataset, the copy-on-write of a background save, and allocator fragmentation.
A limit equal to `maxmemory` gets the process OOM-killed instead of evicting keys.

### The Redis password is no longer on the command line

It used to be:

```yaml
command: redis-server --requirepass ${REDIS_PASSWORD} …
```

which put the password in the container's process list — readable by anything
running inside the container, and by `docker inspect` on the host. It is now
written at startup to a `0600` file on the container filesystem and read from
there:

```yaml
user: redis
command:
  - sh
  - -c
  - |
    umask 077
    printf 'requirepass %s\n…' "$$REDIS_PASSWORD" > /tmp/redis.conf
    exec redis-server /tmp/redis.conf
```

The value is written quoted (`requirepass "…"`), so spaces and `#` are safe. A
password containing a double quote or a backslash would need escaping for the
config parser — but it would already break `REDIS_URL` for the application,
which is a URL, so keep `REDIS_PASSWORD` to unreserved characters.

`user: redis` is **required** by this form and not decoration: the image's
entrypoint only drops privileges when its first argument is literally
`redis-server`. Passing `sh` instead means the entrypoint hands straight over,
and without `user:` the server would run as root — strictly worse than what we
started with.

The healthcheck was leaking the same secret through `redis-cli -a <password>`.
It now relies on `REDISCLI_AUTH`, which `redis-cli` reads from its environment:

```yaml
healthcheck:
  test: ["CMD-SHELL", "redis-cli ping | grep -q PONG"]
```

**What this does not fix:** the password is still an environment variable of the
redis container, so `docker inspect` shows it under `Config.Env`, as it does for
`DB_PASSWORD` and the SMTP credentials of the `app` service. Removing it from the
environment entirely means Docker or Compose secrets (`/run/secrets/…`), which is
worth doing if your threat model includes people with Docker socket access — but
they would then also be able to `exec` into any container, so the gain is
narrower than it looks. Getting the value out of `argv` was the part that was
cheap and unambiguous.

---

## 2. What is deliberately *not* applied: `read_only`

`read_only: true` is not enabled on any service. A compose file that no longer
starts is far worse than an unhardened one, and no container could be started
here to check.

What is known from reading the code:

- The **application binary writes nothing**. There is no `os.Create`,
  `os.WriteFile` or `os.MkdirAll` anywhere in `cmd/`, `internal/` or `pkg/`.
- The **entrypoint** ([`scripts/docker-entrypoint.sh`](../scripts/docker-entrypoint.sh))
  writes only the generated RSA key pair, into `/app/keys` — which is the
  `whento_keys` named volume and stays writable regardless.
- Whether the Go runtime, `openssl`, `wget` (the healthcheck) or the `migrate`
  CLI touch `/tmp` on some path that has not been exercised here is not something
  reading the source can settle.

So the `app` service carries the expected block, commented out, ready to try on a
staging stack:

```yaml
read_only: true
tmpfs:
  - /tmp:rw,noexec,nosuid,size=16m
```

Enable it, `docker compose up -d app`, and confirm the container reaches
`healthy` **and** that a fresh `whento_keys` volume still gets its key pair — the
first-start path is the one most likely to want a writable filesystem.

For `postgres` and `redis`, a read-only root needs more than a `/tmp` tmpfs
(`/var/run/postgresql` for the socket, among others) and the official images
document neither configuration as supported. Not attempted.

---

## 3. What was already right, and should stay that way

- **No database or cache port is published.** Only `app` maps a port. Postgres and
  Redis are reachable on the `whento-network` bridge and nowhere else.
  (`docker-compose.dev.yml` does publish them, on purpose, because the backend
  runs on the host in development. Do not copy it into production.)
- **Passwords are mandatory.** `${DB_PASSWORD:?…}` and `${REDIS_PASSWORD:?…}` fail
  the `up` rather than defaulting to something guessable.
- **The application image runs as a non-root user** and the JWT private key is
  written mode `0600`.
- **Healthchecks and `depends_on: service_healthy`** mean the app does not start
  against a database that is not ready.
- **`shm_size: 128mb`** on Postgres — the default 64 MB is small for parallel
  queries.

---

## 4. Going further

Worth doing, in roughly descending order of value:

1. **Do not publish the app port at all.** Put a reverse proxy on the same
   network and drop the `ports:` mapping from `app`
   ([reverse-proxy.md](reverse-proxy.md) has the override file).
2. **Pin the image by digest**, as the Dockerfiles already do for their base
   images: `image: ghcr.io/when-to/whento@sha256:…`. `:latest` means the next
   `docker compose pull` can change what runs without any change on your side.
3. **`internal: true` on a second network** for postgres/redis, so those
   containers have no route out to the internet at all.
4. **Secrets from files** (`secrets:` in Compose) rather than `.env`, if you have
   somewhere better to keep them.
5. **Log limits** — see [logging-and-privacy.md](logging-and-privacy.md) §6.
6. **`user: "1000:1000"` on `app`** is redundant today (the image already does it)
   but pins the behaviour if the image ever changes.

---

## 5. Verifying, because none of this was verified here

After the first `docker compose up -d` with these changes:

```bash
# All three healthy, none restarting.
docker compose ps

# The stack actually works: health, then a real login and a real calendar.
curl -fsS http://localhost:8080/api/health

# The capability sets are what the file asks for.
docker inspect whento-postgres -f '{{.HostConfig.CapDrop}} {{.HostConfig.CapAdd}}'
docker inspect whento-app      -f '{{.HostConfig.CapDrop}} {{.HostConfig.SecurityOpt}}'

# The Redis password is no longer in the process list.
docker compose exec redis ps -o args | grep -c requirepass   # expect 0
docker inspect whento-redis -f '{{json .Config.Cmd}}'        # expect $REDIS_PASSWORD, not its value

# Redis is not running as root.
docker compose exec redis id

# The limits landed.
docker stats --no-stream
```

The failure modes to watch for, and what they mean:

| Symptom                                                                | Cause                                                      |
| ---------------------------------------------------------------------- | ---------------------------------------------------------- |
| `postgres` exits immediately, `chown: Operation not permitted` in logs | A capability was trimmed too far; restore the five listed.  |
| `redis` logs `Permission denied` opening `/data`                       | The volume's files are not owned by uid 999; `chown` them once with a throwaway root container. |
| `app` container killed, exit code 137                                  | Memory limit too low for your instance; raise it.           |
| Postgres slow under load, no errors                                    | CPU limit; raise `cpus`.                                    |
