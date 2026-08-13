# Logging, telemetry and data at rest

WhenTo is a scheduling tool: the people using a calendar link are frequently not
users of the product, never agreed to anything, and often are not even asked for
a name. The logging posture is built around that. This page states exactly what
is written, what is deliberately not written, what lands in Redis and in what
shape, and where the gaps still are.

Everything here is a statement about the current code, with file references, so
it can be re-checked rather than believed.

---

## 1. There is no telemetry

WhenTo sends nothing anywhere. No analytics, no crash reporting, no phone-home,
no update check, no vendor SDK. The only outbound connections a self-hosted
instance makes are to its own PostgreSQL, its own Redis, and the SMTP server you
configured.

Logs are written to **stdout** as structured JSON
([`pkg/logger`](../pkg/logger/logger.go), Go's `log/slog`). Where they go from
there is entirely your container runtime's business — WhenTo neither ships nor
rotates them.

If the Prometheus metrics listener is enabled (it is off unless you turn it on,
and when on it listens on a port of its own rather than on the application
router), the same rule applies to it: the exposition is **pulled** by your
monitoring, never pushed anywhere, and its series are labelled by HTTP method,
chi route **pattern** and status code — no path, no token, no user id, no IP.
Publish that port to your monitoring network only; the exposition is operational
data and is not meant to be public.

---

## 2. The HTTP access log

One line per request, from `middleware.Logger` in
[`pkg/middleware/middleware.go`](../pkg/middleware/middleware.go):

| Field         | Example                                       | Note                                                     |
| ------------- | --------------------------------------------- | -------------------------------------------------------- |
| `method`      | `POST`                                        |                                                          |
| `route`       | `/calendar/{token}/participant/{pid}`         | The chi route **pattern**, not the path. See below.       |
| `status`      | `200`                                         |                                                          |
| `duration_ms` | `12`                                          |                                                          |
| `bytes`       | `431`                                         |                                                          |
| `request_id`  | `9f1c…`                                       | From `X-Request-ID`, or a fresh UUID. Ties lines together. |
| `user_id`     | `3ac7…`                                       | **Only for authenticated requests**, from the JWT.        |

### What is not in it, and why

- **No IP address.** Not in this line, not anywhere else.
- **No User-Agent.** Nothing that fingerprints a browser or a device.
- **No request path.** This is the important one. In WhenTo the path *is* the
  credential: `/calendar/{token}/participant/{pid}`,
  `/magic-link/verify/{token}`, `/verify-email/{token}`. A log line carrying the
  path would carry a replayable secret, and log files travel — to a collector, to
  a support ticket, to a screenshot. Logging the route pattern keeps the
  placeholders and drops the values. When chi matched no route (a 404), the field
  reads `unrouted` rather than falling back to the raw path.
- **No query strings, no request bodies, no response bodies, no headers, no
  cookies.**

`user_id` is a database UUID and is personal data in the sense that it identifies
an account holder — someone who has registered and has a row in `users` anyway.
It is not linked to any IP or device identifier, so a log archive on its own does
not tell you where anyone connected from.

### Anonymous participants are not identified at all

Someone opening a calendar link and filling in their availability produces access
log lines that contain no `user_id`, no IP, no user agent, and a route pattern
without the token. The log records that a request happened, and nothing about who
made it.

---

## 3. Application logs: correlation tags instead of identities

Application logging — the lines the handlers and services write, as opposed to
the one-per-request access log of §2 — used to carry email addresses, calendar
tokens, participant ids, participant names and, in one case, a complete
participant link. It now carries none of those. Where a line would be useless
without some way to group it, it carries a **correlation tag** instead, produced
by `logger.Fingerprint` in
[`pkg/logger/fingerprint.go`](../pkg/logger/fingerprint.go).

A tag is the first 64 bits of an HMAC-SHA256 of the value, under a **salt that is
random per process** and never written down. Fields carrying one are named
`*_ref`:

| Field              | Stands for                                     |
| ------------------ | ---------------------------------------------- |
| `calendar_ref`     | A calendar's public token                       |
| `account_ref`      | An email address at sign-in or password reset   |
| `recipient_ref`    | The recipient address of an outgoing email      |
| `participant_ref`  | A participant UUID                              |
| `recipient_id_ref` | A notification recipient, owner or participant  |
| `chat_ref`         | A Telegram chat id                              |
| `challenge_ref`    | A passkey login ceremony's challenge id          |

**What a tag can be used for:** telling, inside one run of one process, whether
twenty failures are one subject retrying or twenty different subjects. That is
what an operator reading a log during an incident actually needs.

**What it cannot be used for, by design:** anything else. It does not survive a
restart, it differs between two instances, and it cannot be matched against a
list of candidate addresses, because there is nothing to compute the candidates'
tags with. If you need to follow one account across restarts, use `user_id`,
which is logged wherever an authenticated user is known; there is deliberately no
equivalent for anonymous participants.

Where an internal `user_id` is available, it is logged in preference to a tag —
it is already in the access log for authenticated requests, and unlike a tag it
is stable and resolvable against the database by whoever runs the instance.

A test in [`pkg/logger/logfields_test.go`](../pkg/logger/logfields_test.go) parses
every non-test `.go` file under `internal/`, `pkg/` and `cmd/` and fails the build
if any log call passes a field named `email`, `to`, `token`, `participant_id`,
`name`, `url`, `ip`, `user_agent` and so on. Its two exceptions are listed in §6.

---

## 4. Rate limiting: what Redis holds

Rate limiting keys on something that identifies a client: an IP address, a user
id, or a path that carries a calendar token and a participant UUID
(`CombinedKeyFunc`). None of that is stored.

Before any backend sees the key, `hashRateLimitKey` in
[`pkg/middleware/ratelimit.go`](../pkg/middleware/ratelimit.go) replaces it with
the first 128 bits of an **HMAC-SHA256** under a per-process salt, hex-encoded.
Redis holds:

```
ratelimit:<32 hex chars>   →  sorted set of request timestamps,  TTL = the limit window
```

There is no IP, no token and no user id in the key, and no way to recover one:
without the salt there is nothing to compare a guess against, and the salt is
random per process by default (`crypto/rand`, 32 bytes) and never written down.
Setting `RATE_LIMIT_KEY_SALT` pins it to a shared secret, which several instances
sharing one Redis must do to share their buckets — keep it as you would any other
secret, since it is what makes the digests non-testable.

When Redis is unavailable the same digest is used as the key of an in-memory
token bucket, evicted after 10 minutes of inactivity. Same property: the process
never holds the plaintext beyond the lifetime of the request.

---

## 5. What else is in Redis, and for how long

The rate limiter is not the only thing using Redis. Two questions have to be kept
apart here, because they have different answers: what is in the **keys**, and
what is in the **values**.

### Keys

Every key built by [`pkg/cache/keys.go`](../pkg/cache/keys.go) hashes its
variable part with `HashKeyPart` — 128 bits of HMAC-SHA256, hex-encoded — so
`KEYS *` and `dump.rdb` show no token, no UUID and no address. The prefix stays
readable, so a key is still identifiable as a calendar row or an ICS feed.

Unlike the rate limiter's, this salt is **deterministic by default**, and it has
to be: two instances must derive the same key for the same row, or one
instance's invalidation never reaches the other and it goes on serving a stale
copy until the TTL runs out. The default salt is therefore a fixed constant —
non-reversible, but *testable*: someone holding a `dump.rdb` and a list of
candidate email addresses can hash the candidates and look for a match. Setting
`RATE_LIMIT_KEY_SALT` closes that, since the same secret salts the cache keys.
Give every instance sharing a Redis the same value.

### Values

The values are **not** anonymised, and cannot be: the cache's entire job is to
hand back the row the request asked for. Being precise about that matters more
than claiming otherwise.

| Key                                              | Value                                                              | TTL                               |
| ------------------------------------------------ | ------------------------------------------------------------------ | --------------------------------- |
| `calendar:id:<hash>`, `calendar:token:<hash>`    | The calendar row as JSON, **including its public and ICS tokens**   | 5 min                             |
| `calendar:participants:<hash>`                   | Participants as JSON: **names and email addresses**                 | 5 min                             |
| `calendar:user:<hash>`                           | That user's calendars                                               | 5 min                             |
| `availability:participant:<hash>`                | Availabilities                                                      | 2 min                             |
| `availability:summary:<hash>:<date>`, `availability:range:<hash>:<from>:<to>` | Aggregated availability   | 2 min                             |
| `ics:feed:<hash>`                                | A rendered ICS feed                                                 | 1 min                             |
| `login_attempts:<hash>`                          | A failed-login counter, keyed by a digest of the address            | 15 min                            |
| `mfa_attempts:<hash>`                            | A failed-MFA counter, keyed by a digest of the user id               | lockout window                    |
| `mfa_used_jti:<hash>`                            | Replay protection for a used MFA token                              | 6 min                             |
| `passkey:registration:<hash>`, `passkey:authentication:challenge:<hash>` | WebAuthn ceremony state          | 5 min                             |
| `auth:pwd_changed:<hash>`                        | Timestamp of the last password change, used to revoke older tokens  | `JWT_ACCESS_EXPIRY` (default 15m) |

Note what the first row means: hashing that key buys less than it appears to,
because the **value** hands back the calendar's public and ICS tokens in clear.
For a few minutes at a time, a Redis console is a way to read calendar
credentials. The hashed key stops `KEYS *` from being a token dump; it does not
stop `GET` from being one.

Live calendar updates use Redis pub/sub. Channel names are not stored — they
exist only for the duration of a publish — but they are visible to
`PUBSUB CHANNELS` and to `MONITOR`, so anyone with a Redis console sees them, and
the channel used to be `whento:broadcast:<calendar public token>`. It is now
`whento:broadcast:<hash>`, derived by the single `channelName` function in
[`pkg/broadcast/redis.go`](../pkg/broadcast/redis.go) through which the publisher,
the subscriber and the receive loop all go. The notices themselves carry no
payload at all: a browser that receives one refetches through the ordinary read
path, so nothing about who changed what crosses the fan-out.

That digest uses the same deterministic `HashKeyPart` salt as the cache keys, and
has to: with a per-process salt an instance would publish on a channel none of
its peers subscribes to, and live updates would silently stop crossing a
multi-instance deployment while every single-instance test went on passing.

Three conclusions follow:

- **Redis is in scope for your data protection assessment.** It is short-lived
  and self-healing, but for a few minutes at a time it holds names, email
  addresses and calendar tokens. Treat it as you treat the database: password
  set (the stack requires `REDIS_PASSWORD`), no port published to the internet
  (the shipped compose file publishes none), and on a private network. Hashing
  the keys does not change this conclusion; it only removes the shortcut of
  reading them off the key space.
- **Set `RATE_LIMIT_KEY_SALT`.** It is the difference between key digests that
  cannot be read and key digests that cannot be *guessed*, and on more than one
  instance it is required anyway for the rate limiter to share its buckets.
- **Redis persistence writes that to disk.** `redis:7-alpine` keeps its default
  RDB save points, so a `dump.rdb` in the `redis_data` volume can contain cached
  personal data. Nothing in WhenTo needs it: everything above is either a cache
  rebuilt from Postgres or state that expires within minutes. If you would rather
  have no personal data at rest in Redis, add `save ""` to the generated config
  in the `redis` service's `command:`. The one thing you lose is the
  `auth:pwd_changed` record across a Redis restart, so a token issued shortly
  before a password change would stay valid until it expires on its own (at most
  `JWT_ACCESS_EXPIRY`). Both choices are defensible; pick deliberately.

---

## 6. Known gaps

The gaps this page used to list — email addresses in application logs, the
calendar token in the SSE handler's log lines, personal data in Redis key names,
the calendar token as a pub/sub channel name, and the two key families that still
spelled out a user UUID — are closed. What follows is what is left, including the
two exceptions that were kept on purpose.

### Deliberate exceptions in the logs

- **The password reset link, when SMTP is not configured.**
  `internal/auth/service/password_reset_service.go` writes the full reset URL —
  token included — at `warn`, in the branch taken when no mail server is set up.
  On such an instance the log *is* the delivery channel; there is no other way
  for an operator to complete a reset. The line no longer carries the address or
  the display name, only `user_id`. Configure SMTP and this branch is never
  reached.
- **`webhook_url` for Discord and Slack**, truncated to its first twenty
  characters. That is the scheme and host and none of the secret path.

Both are exempted by name in
[`pkg/logger/logfields_test.go`](../pkg/logger/logfields_test.go); adding a third
means editing that list, which is the point.

### Still open, and where

- **Cache values are not anonymised**, and one of them returns calendar
  credentials. See §5.
- **The default cache key salt is a fixed constant** unless you set
  `RATE_LIMIT_KEY_SALT`. See §5.
- **`user_id` is logged for authenticated requests**, in the access log and in
  application lines. That is a deliberate choice, described in §2: it identifies
  an account holder who has a row in `users` anyway, and it is not linked to any
  IP or device identifier.
- **The static guard checks field *names*, so it only catches the names it knows.**
  `pkg/logger/logfields_test.go` would not have flagged `"topic"` — the field the
  broadcast broker used to log the calendar token under — because nothing about
  the word says "credential". Two such names were found and fixed by reading the
  code (`topic` in `pkg/broadcast/redis.go`, `challenge_id` in
  `internal/passkey/service/passkey_service.go`), and both now log a `*_ref`
  fingerprint instead. Nothing stops a third from being introduced under another
  innocuous name; a new log field still has to be thought about, and a name that
  could carry an identifier belongs in that guard's list.
- **The pub/sub *values* in Redis are empty, but the subscription topology is
  not.** `PUBSUB CHANNELS` still shows how many distinct calendars are currently
  being watched, and `MONITOR` still shows the rate at which each digest is
  published to. That is traffic analysis over opaque identifiers, not personal
  data, and closing it would mean giving up pub/sub — it is recorded here rather
  than fixed.

`LOG_LEVEL=warn` suppresses the `info` and `debug` lines. Since none of them
carry an address or a token any more, that is now a question of volume rather
than of exposure — with the single exception of the SMTP-less reset link above,
which is written at `warn` and is not suppressed by any level.

---

## 7. Recommended retention

WhenTo has no log retention setting — it writes to stdout and stops there, so
retention is a property of your Docker logging driver or your log collector.

| Stream                    | Suggested retention | Reasoning                                                                              |
| ------------------------- | ------------------- | -------------------------------------------------------------------------------------- |
| Access log                | 30 days             | Enough to investigate an incident or a regression; short enough not to become an archive of who used what. |
| Application error logs    | 90 days             | Debugging value outlives the access log; see §6 for what they may contain.              |
| Postgres container logs   | 14 days             | Operational only. Do not enable `log_statement=all`: query text contains tokens.        |
| Backups                   | See [backup-restore.md](backup-restore.md) | Dumps contain everything, so they carry the longest obligation.       |

A sane default with the local driver, applied per service in the compose file:

```yaml
logging:
  driver: json-file
  options:
    max-size: "10m"
    max-file: "5"
```

If you forward logs to a collector, forward them to one you control, and check
that it does not enrich events with the client IP the collector saw.

---

## 8. Where personal data actually lives

For a data-protection register, the honest picture is:

| Store                       | Contains                                                                                    | Lifetime                                 |
| --------------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------- |
| PostgreSQL (`postgres_data`) | Users (email, password hash, MFA secrets, passkeys), calendars, participants (name, optional email), availabilities, notification log | Until deleted through the application     |
| Redis (`redis_data`)         | The cache and short-lived auth state of §5                                                   | Minutes                                  |
| Logs (stdout)                | §2, §3 and §6                                                                                    | Your retention                           |
| Backups                      | A full copy of PostgreSQL                                                                    | Your retention                           |
| Outbound email               | Recipient address, calendar name, links                                                      | Your SMTP provider's retention, not yours |

The last row is the one people forget. Every verification, magic-link, reset and
notification email goes through the SMTP provider configured in `SMTP_HOST`, and
that provider sees recipient addresses and the links themselves. Choose it as
carefully as you chose where to host the database.

## No third-party requests from the browser

The application makes no request to a host it does not control. This matters as
much as what the server logs: a stylesheet, a font or a script fetched from
another origin hands that origin the visitor's IP address, User-Agent and
referring page, on every page load, before any consent is possible.

Inter used to be fetched from `fonts.googleapis.com` as a render-blocking
stylesheet. It is bundled now (`@fontsource-variable/inter`, imported from
`src/main.ts`), served from the same origin as the application. Two things were
wrong with the old arrangement, and only one of them was about privacy:

- every visitor's address reached Google, which is precisely what the rest of
  this document promises does not happen to an address;
- a self-hosted instance on a network that cannot reach Google waited for that
  request to time out before painting anything. An 18-second first paint was
  observed in exactly that situation.

Both are closed by the same change, and neither can come back quietly: a new
external `<link>` or `@import` would show up in the network panel of any page
load, and there is nothing in the build that fetches one.
