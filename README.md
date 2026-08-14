# WhenTo

> **When to Play RPG?** • **When to Meet?** • **When to Hike?**

Self-hosted web application for organizing recurring events with friends through collaborative calendars. Participants indicate their availability, and when the threshold is reached, events automatically sync to your calendar apps via iCalendar subscriptions.

![License](https://img.shields.io/badge/license-BSL--1.1-blue.svg)
![Docker](https://img.shields.io/badge/docker-ready-brightgreen.svg)
![Free Self-Hosted](https://img.shields.io/badge/self--hosted-free-green.svg)

---

## ✨ Features

### Core Functionality

- **Collaborative Calendars** — Create permanent calendars for recurring activities (RPGs, sports, meetups...)
- **Flexible Availability** — One-time dates or recurring patterns ("every Friday evening")
- **Configurable Threshold** — Define minimum participants required for an event to be confirmed
- **iCalendar Subscription** — Sync URL for Google Calendar, Apple Calendar, Outlook, and more
- **Smart Recurrence** — Set weekly availability once with exceptions for special weeks
- **Live Updates** — Another participant's answer appears without reloading the page
- **Multi-channel Notifications** — Get notified when threshold is reached/lost via Email, Discord, Slack, or Telegram
- **Participant Email Verification** — Optional email verification for participants to receive notifications
- **Multi-language** — Interface available in French and English (including emails)
- **Timezone Support** — Each calendar can have its own timezone, which also decides the
  first day of the week (Monday in Berlin, Sunday in New York, Saturday in Cairo) so every
  participant sees the same grid whatever their own language
- **Holiday Policies** — Configure how public holidays are handled (ignore/allow/block)
- **Participant Locking** — Masks participant ids in the public view, so a visitor cannot
  answer in somebody else's name
- **Anonymous Participant Registration** — Allow anyone with the public link to
  self-register as a participant without authentication. The interface presents this and
  participant locking as mutually exclusive; the API does not enforce that, so a script
  setting both gets both
- **Self-hosted** — Your data stays on your infrastructure

### Authentication & Security

- **Email Verification** — Required before creating calendars
- **JWT Authentication** — RS256 asymmetric keys with refresh tokens, held in httpOnly
  cookies rather than in the response body
- **Passkeys (WebAuthn)** — Passwordless sign-in, including usernameless discoverable
  credentials
- **Two-factor Authentication** — TOTP with single-use backup codes, and a per-user
  attempt limit on verification
- **Magic Links** — Sign in from an emailed link, when SMTP is configured
- **Password Security** — Bcrypt hashing with strict password requirements
- **Rate Limiting** — Protection on public endpoints and API routes, backed by Redis when
  available and by an in-process limiter when it is not
- **Regenerable Tokens** — Public and ICS tokens can be regenerated if compromised
- **Security Headers** — HSTS, CSP, X-Frame-Options protection

### Deployment Options

WhenTo supports **two distinct deployment modes**:

| Mode             | Build Tag    | Quota Scope | Calendar Limit         |
| ---------------- | ------------ | ----------- | ---------------------- |
| **Cloud** (SaaS) | `cloud`      | Per-user    | 3 calendars per account |
| **Self-hosted**  | `selfhosted` | Server-wide | Unlimited              |

---

## 🌟 Why WhenTo?

### The Problem with Traditional Date Polls

Tools like Doodle or Framadate are great for finding a one-time date. But for recurring events:

- **Disposable Polls** — Every week, you need to create a new poll and resend the link to everyone
- **No Recurrence** — You can't say "I'm available every Tuesday evening" - you have to click on each date individually
- **No Synchronization** — Once the date is chosen, you have to manually add it to your calendar

### The WhenTo Solution

WhenTo rethinks collaborative planning for groups that meet regularly:

- **Permanent Calendars** — One shared calendar for all your sessions
- **iCal Sync** — Validated events automatically appear in Google Calendar, Outlook, Apple Calendar
- **Smart Recurrence** — Set your weekly availability once, with exceptions for special weeks
- **Privacy Guaranteed** — Self-hosted, open source, no tracking or ads

### Use Cases

- **RPGs & Board Games** — Organize weekly sessions without sending a new Doodle every week
- **Amateur Sports** — Manage team availability for recurring practices and games
- **Music Bands** — Schedule rehearsals when all members are available
- **Team Meetings** — Automatically find the ideal slot for your weekly meetings

---

## 🚀 Quick Start

### Prerequisites

- Docker & Docker Compose
- PostgreSQL 16+ and Redis 7+ (included in docker-compose)

### Installation

```bash
# Clone the repository
git clone https://github.com/When-To/whento.git
cd whento

# Copy and configure environment variables
cp .env.example .env
nano .env  # Update passwords and configuration

# Launch the application
docker compose up -d
```

The application is accessible at `http://localhost:8080`

### First Account

The **first registered user** automatically becomes an administrator.

---

## 📱 Usage

### 1. Create a Calendar

1. Log in to your account (verify your email first)
2. Click "New calendar"
3. Give it a name (e.g., "Weekly RPG Session")
4. Add participants (names only - no accounts required)
5. Set minimum threshold (e.g., 4 people)
6. Configure optional settings:
   - Allowed weekdays
   - Timezone
   - Holiday policy
   - Minimum event duration
   - Date range restrictions

### 2. Share the Link

Share the public link with your friends:

```
https://your-domain.com/c/abc123def456...
```

Each participant:

1. Opens the link
2. Selects their name from the list (or self-registers if anonymous registration is enabled on the calendar)
3. Indicates their availability (dates + optional time slots)
4. Can add recurring availability patterns

### 3. Subscribe to the Calendar

Once the threshold is reached on certain dates, add the subscription URL to your calendar app:

```
https://your-domain.com/api/v1/ics/feed/{token}.ics
```

| Application         | How to Add                                |
| ------------------- | ----------------------------------------- |
| **Google Calendar** | Other calendars → From URL                |
| **Apple Calendar**  | File → New Calendar Subscription          |
| **Outlook**         | Add calendar → From Internet              |
| **Thunderbird**     | New calendar → On the Network → iCalendar |

Events sync automatically!

---

## 💰 Pricing

WhenTo is free.

| Mode            | Calendars               | Price |
| --------------- | ----------------------- | ----- |
| **Cloud**       | 3 calendars per account | Free  |
| **Self-hosted** | Unlimited               | Free  |

There is no paid plan, no licence key and no payment integration. Every account on the
hosted service gets the same allowance; self-hosting is unrestricted.

---

## ⚙️ Configuration

### Environment Variables

#### Common (Both Modes)

```bash
# Application
APP_PORT=8080                     # Host port (Docker compose). Default: 8080
APP_ENV=production                # Default: development
APP_URL=https://your-domain.com   # Used to generate links in emails
LOG_LEVEL=info                    # Default: info (debug, info, warn, error)

# Database
# - For the docker-compose.yml shipped with this repo: set the DB_* variables
#   below (DB_NAME/DB_USER/DB_PASSWORD are REQUIRED — they are consumed by
#   both the postgres service and the app service). DATABASE_URL is ignored
#   in that mode.
# - For standalone / binary deployments: set DATABASE_URL directly (preferred);
#   DB_* are then only used as a fallback if DATABASE_URL is unset.
DB_NAME=whento
DB_USER=whento
DB_PASSWORD=yourpassword
# DB_HOST=localhost                 # Standalone/binary only (compose hardcodes "postgres")
# DB_PORT=5432                      # Standalone/binary only (compose hardcodes 5432, not exposed)
# DB_SSLMODE=disable                # Standalone/binary only (compose hardcodes "disable")
# DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable   # Standalone only

# Redis — same logic as the database
# - docker-compose: REDIS_PASSWORD is REQUIRED (used by both the redis service
#   and the app). REDIS_URL is ignored.
# - Standalone / binary: set REDIS_URL directly (preferred); REDIS_* are
#   fallback only.
REDIS_PASSWORD=yourpassword
# REDIS_HOST=localhost              # Standalone/binary only (compose hardcodes "redis")
# REDIS_PORT=6379                   # Standalone/binary only (compose hardcodes 6379, not exposed)
# REDIS_DB=0                        # Standalone/binary only
# REDIS_URL=redis://:password@host:6379                            # Standalone only

# JWT — keys are auto-generated on first run if they do not exist
JWT_PRIVATE_KEY_PATH=/app/keys/private.pem
JWT_PUBLIC_KEY_PATH=/app/keys/public.pem
JWT_ACCESS_EXPIRY=15m             # Default: 15m
JWT_REFRESH_EXPIRY=168h           # Default: 168h (7 days)
# JWT_ISSUER=whento               # Default: whento

# SMTP (required for email verification, password reset, and notifications)
SMTP_HOST=mail.example.com
SMTP_PORT=587                     # Default: 587. Use 465 for implicit TLS, 587 for STARTTLS
SMTP_USERNAME=user@example.com
SMTP_PASSWORD=yourpassword
SMTP_FROM=whento@example.com      # Default: contact@whento.be
SMTP_FROM_NAME=WhenTo             # Default: Contact WhenTo
# Note: TLS mode is determined automatically by the port — there is no
#       separate TLS toggle variable.

# Registration & email verification
EMAIL_VERIFICATION_ENABLED=true   # Default: false
# EMAIL_VERIFICATION_EXPIRY=24h
# PASSWORD_RESET_EXPIRY=1h
# MAGIC_LINK_EXPIRY=1h
ALLOWED_REGISTER=true             # Default: true
ALLOWED_EMAILS=*                  # Comma-separated patterns (e.g., *@company.com). Default: * (all)

# Rate Limiting
RATE_LIMIT_ENABLED=true           # Default: true

# Security
BCRYPT_COST=12                                # Default: 12
CORS_ORIGINS=https://your-domain.com          # Comma-separated allowed CORS origins (default: APP_URL)
TRUSTED_PROXIES=                              # Comma-separated trusted reverse proxy IPs/CIDRs (e.g., 127.0.0.1,10.0.0.0/8)
# DISABLE_ROBOTS=false                        # Disable robots.txt (default: false)

# WebAuthn / Passkeys (optional — defaults are derived from APP_URL)
# WEBAUTHN_RP_NAME=WhenTo
# WEBAUTHN_RP_ID=your-domain.com
# WEBAUTHN_RP_ORIGIN=https://your-domain.com
# WEBAUTHN_TIMEOUT=60s

# TOTP / 2FA (optional)
# TOTP_ISSUER=WhenTo
# TOTP_PERIOD=30
# TOTP_DIGITS=6
```

The two build variants read the same environment. There is nothing to configure that is
specific to one of them: the only difference is the calendar allowance, which is compiled
in rather than configured.

---

## 🔧 Architecture

### Project Structure

```
whento/
├── cmd/
│   ├── main.go              # Single entry point
│   ├── init_cloud.go        # Cloud-specific initialization (tag: cloud)
│   ├── init_selfhosted.go   # Self-hosted initialization (tag: selfhosted)
├── internal/                # Business modules, each {handlers,models,repository,service}
│   ├── auth/                # JWT RS256, users, sessions, magic links
│   ├── calendar/            # CRUD, participants
│   ├── availability/        # Availabilities, recurrences
│   ├── mfa/                 # TOTP and backup codes
│   ├── passkey/             # WebAuthn registration and sign-in
│   ├── notify/              # Threshold notifications, webhooks
│   ├── ics/                 # iCalendar feed generation
│   ├── seo/                 # robots.txt and sitemap.xml
│   ├── config/              # Environment parsing
│   ├── quota/               # Calendar limits (both modes)
│   └── testutil/            # Test helpers, including the database harness
├── pkg/                     # Shared packages, a separate Go module
│   ├── broadcast/           # Live-update notices, Redis-backed or process-local
│   ├── cache/               # Redis wrapper with a no-op fallback
│   ├── database/            # PostgreSQL + Redis
│   ├── datevalidation/      # Holidays, weekdays, opening hours
│   ├── email/               # SMTP delivery
│   ├── httputil/            # The {data, error} response envelope
│   ├── jwt/                 # RS256 token management
│   ├── logger/              # Structured logging with request ids
│   ├── middleware/          # Auth, rate limiting, CORS, security headers
│   ├── models/              # Shared entities
│   └── validator/           # Input validation
├── frontend/                # Vue 3 SPA
│   └── src/
│       ├── api/             # API client
│       ├── components/      # Vue components
│       ├── stores/          # Pinia stores
│       └── locales/         # i18n translations
└── migrations/              # SQL migrations
```

### Tech Stack

| Layer                   | Technology                                         |
| ----------------------- | -------------------------------------------------- |
| Backend                 | Go 1.26, Chi router, pgx/v5, go-redis/v9           |
| Frontend                | Vue 3, Vite 8, TypeScript, Tailwind CSS 4, Pinia 4 |
| Database                | PostgreSQL 16, Redis 7                             |
| Auth                    | JWT RS256 (asymmetric keys), bcrypt                |
| Licensing (Self-hosted) | Ed25519 cryptographic signatures                   |
| i18n                    | vue-i18n (FR/EN)                                   |
| iCalendar               | arran4/golang-ical                                 |

---

## 🐳 Deployment

### Docker Compose (Recommended)

The repository ships a ready-to-use [`docker-compose.yml`](docker-compose.yml). It defines the `app`, `postgres`, and `redis` services and builds `DATABASE_URL` / `REDIS_URL` for the app from the variables you set in `.env`.

```bash
cp .env.example .env
# Edit .env — at minimum set:
#   DB_PASSWORD, REDIS_PASSWORD, APP_URL, and SMTP_* if you want email
docker compose up -d
```

Minimum required variables in `.env` when using this compose file:

```bash
# Database — consumed by both the postgres service and the app
DB_PASSWORD=your_secure_password
# DB_NAME, DB_USER default to "whento" if unset

# Redis — consumed by both the redis service and the app
REDIS_PASSWORD=your_redis_password

# Application
APP_URL=https://your-domain.com

# SMTP (required for email verification and notifications)
SMTP_HOST=mail.example.com
SMTP_PORT=587
SMTP_USERNAME=user@example.com
SMTP_PASSWORD=yourpassword
SMTP_FROM=whento@example.com
```

> **Note:** with this compose file, `DATABASE_URL` and `REDIS_URL` are constructed automatically from `DB_*` / `REDIS_PASSWORD`. Setting them directly in `.env` has no effect here — they only apply to standalone / binary deployments.

> **⚠️ `docker compose down -v` deletes your data.** `postgres_data`, `redis_data` and
> `whento_keys` are local named volumes; `-v` removes all three, with no prompt and no
> undo. Stop the stack with `docker compose down`. Set up backups before the instance
> holds anything you would miss — [docs/backup-restore.md](docs/backup-restore.md).

### Operator documentation

The procedures that are too long for this file live in [`docs/`](docs/README.md):

| Guide                                                                | For                                                                              |
| -------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| [Backup and restore](docs/backup-restore.md)                         | Dumps, retention, restore, and the restore drill that proves the backup works.     |
| [Reverse proxy](docs/reverse-proxy.md)                               | Caddy/nginx examples, and setting `TRUSTED_PROXIES` so rate limiting works.        |
| [Migration rollback](docs/migration-rollback.md)                     | Migrations apply automatically at every start. What to do when one is the problem. |
| [Upgrading Postgres and Redis](docs/upgrade-postgres-redis.md)       | Why a Postgres major bump needs a dump and restore, step by step.                  |
| [Docker hardening](docs/hardening-docker.md)                         | What the compose file hardens, what it does not, and how to verify.                |
| [Logging and privacy](docs/logging-and-privacy.md)                   | What is logged, what is not, what Redis holds, and for how long.                   |

### JWT signing keys must be persisted

On first start the container generates an RSA-4096 key pair in `/app/keys` and signs
every access and refresh token with it. **That directory has to survive container
recreation.** If it does not, a new key pair is minted on the next start, every token
already issued stops verifying, and every signed-in user is logged out — on an image
update, on an `.env` change, on anything that makes Compose recreate the container.

The shipped `docker-compose.yml` handles this with a named volume:

```yaml
services:
  app:
    volumes:
      - whento_keys:/app/keys

volumes:
  whento_keys:
    driver: local
```

Consequences worth knowing:

- **Back up `whento_keys` alongside the database.** Restoring the database without the
  keys works, but logs everyone out. Both are covered by
  [docs/backup-restore.md](docs/backup-restore.md).
- **Deleting the volume is the way to rotate the keys** — a deliberate, global logout.
- **Calendar participant links are not affected.** They authorise by possession of the
  link itself, not by a signed token, so they keep working across a key rotation.
- Running the binary outside Docker: point `JWT_PRIVATE_KEY_PATH` /
  `JWT_PUBLIC_KEY_PATH` at a directory that persists across deployments, and keep the
  private key at mode `600`.

### Building from Source

```bash
# Self-hosted build (default)
make build
# or
go build -tags selfhosted -o bin/whento ./cmd/
```

### Reverse Proxy

WhenTo terminates no TLS of its own. Minimal Caddy configuration:

```caddyfile
your-domain.com {
    reverse_proxy whento-app:8080 {
        flush_interval -1  # live availability updates are a long-lived SSE stream
    }
}
```

Behind a proxy, set `TRUSTED_PROXIES` to the address the proxy connects from (for the
compose stack, the Docker bridge range, e.g. `172.16.0.0/12`). `X-Forwarded-For` and
`X-Real-IP` are ignored for every other source, by design: believing them
unconditionally would let any client choose its own rate-limit bucket. Until it is set,
per-IP rate limits count all proxied traffic as one client.

Complete Caddy and nginx configurations, how to find the right `TRUSTED_PROXIES` value,
and the SSE and header requirements: [docs/reverse-proxy.md](docs/reverse-proxy.md).

---

## 🛠️ Development

### Prerequisites

- Go 1.26
- Node.js 24
- Docker & Docker Compose

### Running in Development Mode

```bash
# Start databases
make dev-db

# Full-stack development (backend + frontend concurrently)
make dev-fullstack

# OR run separately:
make dev-backend    # Backend on :5173 (for frontend proxy)
make dev-frontend   # Frontend on :8080 (proxies /api to :5173)

# OR backend only
make dev            # Backend on :8080
```

### Testing

```bash
# Everything, for the default build (selfhosted)
make test
make test BUILD_TYPE=cloud

# With coverage
make test-coverage
```

Two things about running Go tests here catch people out.

**Always pass a build tag.** Whole packages are excluded without one, so a bare
`go test ./...` compiles a different program from the one that ships and can pass while
the real build is broken:

```bash
go test -tags selfhosted ./internal/auth/... -v
```

**`./...` does not reach `pkg/`.** It is a separate module wired in through a `replace`,
so it needs its own invocation — which is why `make test` runs both:

```bash
cd pkg && go test -tags selfhosted ./...
```

Some repository tests need a real PostgreSQL. They read `DATABASE_URL` and **skip
cleanly when it is unset**, so `make test` stays green on a machine without a database;
CI supplies one. Point it at a throwaway database rather than your development one — the
tests create and delete their own rows, but they run against whatever you give them:

```bash
make dev-db                       # postgres + redis via docker-compose.dev.yml
DATABASE_URL=... make test
```

Frontend and browser tests live under `frontend/`:

```bash
cd frontend
npm run test                      # unit tests (Vitest)
npm run test:coverage
npx playwright test               # component and rendering tests, no server needed
npm run test:e2e:backend          # drives a real server; see the note below
```

`test:e2e:backend` expects `make dev-fullstack` running, and the **server must be started
with `RATE_LIMIT_ENABLED=false`**. The limiter is per IP, and a suite driving a real
calendar from one address is exactly the traffic it exists to refuse — leaving it on
turns every test into a 429.

### Migrations

```bash
# Apply migrations
make migrate-up

# Rollback last migration
make migrate-down

# Check migration status
make migrate-status
```

These are development commands. In production the container entrypoint runs
`migrate up` on every start; reverting one there is a different procedure, with
different risks — [docs/migration-rollback.md](docs/migration-rollback.md).

### Frontend Commands

```bash
cd frontend

npm run dev         # Vite dev server
npm run build       # Production build
npm run type-check  # TypeScript checking
npm run lint        # ESLint
npm run format      # Prettier (same as `make format-frontend`)
```

### Formatting

```bash
make hooks         # Enable the pre-commit hook (once per clone)
make format        # Format Go (goimports) + frontend (prettier)
make format-check  # Check formatting without modifying
```

The `pre-commit` hook formats staged files automatically — see [CONTRIBUTING.md](.github/CONTRIBUTING.md).

---

## 📖 API Documentation

### Authentication Routes (`/api/v1/auth`)

- `POST /register` — Register new user (email verification required)
- `POST /login` — Login with credentials
- `POST /refresh` — Refresh access token
- `POST /logout` — Logout (invalidate refresh token)
- `GET /me` — Get current user profile
- `PATCH /me` — Update profile (display name, locale, timezone)
- `PATCH /me/password` — Change password
- `POST /forgot-password`, `POST /reset-password` — Password reset by email
- `POST /send-verification`, `GET /verify-email/{token}` — Email verification
- `POST /magic-link/request`, `GET /magic-link/verify/{token}` — Sign in from an emailed
  link. `GET /magic-link/available` reports whether SMTP is configured
- `POST /mfa/verify` — Complete a login that requires a second factor
- `POST /passkey/login/begin`, `POST /passkey/login/finish` — Passwordless sign-in

### Two-Factor Authentication (`/api/v1/mfa`)

- `POST /setup/begin`, `POST /setup/finish` — Enrol an authenticator app
- `GET /status` — Whether 2FA is enabled
- `POST /disable` — Turn 2FA off
- `POST /backup-codes/regenerate` — Issue new single-use backup codes

### Passkey Routes (`/api/v1/passkey`)

- `POST /register/begin`, `POST /register/finish` — Register a passkey
- `GET /list` — List this account's passkeys
- `PATCH /{id}/name` — Rename a passkey
- `DELETE /{id}` — Remove a passkey

### Calendar Routes (`/api/v1/calendars`)

- `POST /` — Create calendar (requires verified email)
- `GET /` — List my calendars
- `GET /{id}` — Get calendar details
- `PATCH /{id}` — Update calendar
- `DELETE /{id}` — Delete calendar
- `GET /public/{token}` — Public calendar view
- `POST /public/{token}/participants` — Self-register as a participant (anonymous, no auth required — only when `allow_anonymous_participants` is enabled)
- `POST /{id}/participants` — Add participant
- `PATCH /{id}/participants/{pid}` — Update participant
- `DELETE /{id}/participants/{pid}` — Delete participant
- `POST /{id}/regenerate-token` — Regenerate public/ICS token

### Availability Routes (`/api/v1/availabilities`)

- `GET/POST/PATCH/DELETE /calendar/{token}/participant/{pid}[/{date}]` — Manage availabilities
- `POST/GET/PATCH/DELETE .../recurrence[/{rid}]` — Manage recurring patterns
- `POST/DELETE .../recurrence/{rid}/exception[/{date}]` — Manage exceptions
- `GET /calendar/{token}/dates/{date}` — Get summary for specific date
- `GET /calendar/{token}/range` — Get summary for date range
- `GET /calendar/{token}/events` — Server-sent events announcing that the calendar
  changed. Each notice carries no payload: fetch the range summary again

### iCalendar Routes (`/api/v1/ics`)

- `GET /feed/{token}` — iCalendar subscription feed for one calendar
- `GET /unified/{token}` — One feed combining several of your calendars
- `GET /unified-feed`, `POST /unified-feed` — Read or create the unified feed
- `PATCH /unified-feed/calendars` — Choose which calendars it includes
- `POST /unified-feed/regenerate-token` — Rotate its URL, revoking the previous one

### Notification Routes (`/api/v1/calendars`)

- `GET /{id}/notify-config`, `PATCH /{id}/notify-config` — Notification settings, owner
  only. Webhook URLs are validated against SSRF before they are stored
- `POST /{token}/participants/{pid}/email` — Add an address to a participant
- `GET /participants/verify-email/{token}` — Verify a participant's address

### Quota Routes - Both Modes (`/api/v1/quota`)

- `GET /limits` — Calendar limits and current usage

### Admin Routes (`/api/v1/admin`)

- `GET /users` — List all users
- `PATCH /users/{id}/role` — Update user role
- `DELETE /users/{id}` — Delete user
- `GET /users/{id}/calendars` — View user's calendars
- `POST /users/{id}/disable-2fa` — Clear a locked-out user's second factor

### Health

- `GET /api/health` — Liveness. Note the path: there is no `/api/v1/health`
- `GET /api/ready` — Readiness, including the database

---

## 🔐 Security Features

- **RS256 JWT** — Asymmetric keys (auto-generated at startup)
- **Bcrypt Password Hashing** — Cost factor 12
- **Email Verification** — Required before calendar creation
- **Rate Limiting** — Redis-backed with graceful fallback
  - Login: 5 req/min/IP
  - Register: 3 req/min/IP
  - Public endpoints: 60 req/min/IP
  - Anonymous participant registration: 10 req/min/IP
  - ICS feed: 30 req/min/IP
  - Authenticated: 100 req/min/user
- **Token Regeneration** — Separate public and ICS tokens can be regenerated
- **Trusted Proxies** — `X-Forwarded-For` and `X-Real-IP` are only believed when the
  connection comes from an address listed in `TRUSTED_PROXIES`. Without that, anyone
  could pick their own rate-limit bucket by setting a header
- **Webhook Validation** — Discord and Slack webhook URLs are checked before they are
  stored, so a notification cannot be pointed at a cloud metadata endpoint or an
  address inside your network
- **CORS Protection** — Configurable allowed origins. A wildcard is treated as
  same-origin only, since a wildcard combined with credentials would let any site make
  authenticated calls
- **Security Headers** — HSTS, CSP, X-Frame-Options
- **SQL Injection Protection** — Parameterized queries via pgx
- **Logs that carry no credentials** — the access log records the chi route pattern
  (`/calendar/{token}/participant/{pid}`), never the path, so calendar tokens and
  magic-link secrets never reach a log file. No IP address and no User-Agent are logged
  either, and rate-limit buckets are stored in Redis under a salted HMAC.
  [Full posture, with the known exceptions →](docs/logging-and-privacy.md)

### One thing to understand about participant links

Participant access is **capability-based**: possession of the calendar's public link,
plus a participant's id, is the authorisation. There is no per-participant secret, so
anyone holding a link can answer as that participant. That is deliberate — it is what
lets you invite people without asking them to create an account — but it means the link
should be shared the way you would share a door key.

If a link gets out, regenerate the public token: the old URL stops working immediately.

`lock_participants` narrows the exposure rather than removing it: the public view still
loads, but participant ids are masked out of it, so a visitor cannot discover somebody
else's id and answer in their name. Anyone holding a direct participant link can still
use it.

---

## 📊 Quota Enforcement

### Cloud Mode (Per User)

Every account may own **3 calendars**. There is no paid tier and no way to raise it.

### Self-Hosted Mode (Server-Wide)

**Unlimited.** No licence, no key, no limit.

**Over-quota lock**: a user who somehow holds more calendars than allowed keeps them,
but cannot create more and their ICS feeds stop rendering until usage drops back within
the allowance. Only reachable on the hosted service.

---

## 📜 License

**Business Source License 1.1**

WhenTo is licensed under the [Business Source License 1.1](LICENSE):

- ✅ **Free for self-hosted use** — Deploy internally for your organization
- ✅ **Free to modify and redistribute** — Under the same BSL terms
- ❌ **Commercial license required** — For offering as a hosted service (SaaS) to third parties
- 🔄 **Converts to Apache 2.0** — On 2035-12-03 (10 years from first publication)

### What this means:

| Use Case                                   | Free?  | License Required?                         |
| ------------------------------------------ | ------ | ----------------------------------------- |
| Self-host for personal use                 | ✅ Yes | No                                        |
| Self-host for your organization (internal) | ✅ Yes | No                                        |
| Offer as SaaS to customers                 | ❌ No  | Yes (Commercial license)                  |
| Fork and modify for yourself               | ✅ Yes | No                                        |
| Resell as a product                        | ❌ No  | Yes (Commercial license)                  |

See [LICENSE](LICENSE) for full BSL terms, [LICENSE-COMMERCIAL](LICENSE-COMMERCIAL) for commercial licensing options.

**IMPORTANT**: After 2035-12-03, WhenTo automatically becomes Apache 2.0 licensed (fully permissive open source).

---

## 🤝 Contributing

Contributions are welcome!

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Please ensure:

- Code follows existing style conventions
- All tests pass (`make test`)
- New features include tests
- Documentation is updated

---

## 🙏 Acknowledgments

- [projectdiscovery/notify](https://github.com/projectdiscovery/notify) — Multi-provider notifications
- [arran4/golang-ical](https://github.com/arran4/golang-ical) — iCalendar generation
- [go-chi/chi](https://github.com/go-chi/chi) — Lightweight HTTP router
- [jackc/pgx](https://github.com/jackc/pgx) — PostgreSQL driver

---

## 📞 Support

- **Community Support** — GitHub Discussions
- **Bug Reports** — GitHub Issues
- **Email Support** — Available for Pro/Power subscribers (Cloud) or Pro/Enterprise license holders (Self-hosted)
- **Priority Support** — Available for Power subscribers (Cloud) or Enterprise license holders (Self-hosted)

---

<p align="center">
  <i>Made with ❤️ for the self-hosted community</i>
</p>
