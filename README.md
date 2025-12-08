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
- **Multi-language** — Interface available in French and English
- **Timezone Support** — Each calendar can have its own timezone
- **Holiday Policies** — Configure how public holidays are handled (ignore/allow/block)
- **Participant Locking** — Option to disable public view and require direct participant links
- **Self-hosted** — Your data stays on your infrastructure

### Authentication & Security

- **Email Verification** — Required before creating calendars
- **JWT Authentication** — RS256 asymmetric keys with refresh tokens
- **Password Security** — Bcrypt hashing with strict password requirements
- **Rate Limiting** — Protection on public endpoints and API routes
- **Regenerable Tokens** — Public and ICS tokens can be regenerated if compromised
- **Security Headers** — HSTS, CSP, X-Frame-Options protection

### Deployment Options

WhenTo supports **two distinct deployment modes**:

| Mode             | Build Tag    | Quota Scope | Billing System                 |
| ---------------- | ------------ | ----------- | ------------------------------ |
| **Cloud** (SaaS) | `cloud`      | Per-user    | Stripe subscriptions           |
| **Self-hosted**  | `selfhosted` | Server-wide | Ed25519 cryptographic licenses |

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
2. Selects their name from the list
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

## 💰 Pricing & Licensing

### Cloud Subscriptions (Per User)

| Plan      | Calendars     | Price               | Features                                   |
| --------- | ------------- | ------------------- | ------------------------------------------ |
| **Free**  | 3 calendars   | Free                | Unlimited participants, iCal subscriptions |
| **Pro**   | 100 calendars | 25€/year (+ VAT)    | Email support                              |
| **Power** | Unlimited     | 100€/year (+ VAT)   | Priority support                           |

All Cloud plans include:

- Unlimited participants per calendar
- iCal subscription feeds
- Recurring availabilities
- Email notifications

### Self-Hosted Licenses (Per Server)

| Tier           | Calendars     | Price                   | Support                            |
| -------------- | ------------- | ----------------------- | ---------------------------------- |
| **Community**  | 30 calendars  | Free                    | Community support                  |
| **Pro**        | 300 calendars | 100€ one-time (+ VAT)   | 1 year included, 60€/year renewal  |
| **Enterprise** | Unlimited     | 250€ one-time (+ VAT)   | 2 years included, 60€/year renewal |

All Self-hosted licenses are **perpetual** (lifetime) with optional support renewal.

#### License Features

- **Ed25519 Cryptographic Validation** — Offline verification, no phone-home
- **Auto-activation** — Can be set via environment variable
- **Manual Activation** — Admin UI for license management
- **License Shop** — Integrated e-commerce for purchasing licenses

---

## ⚙️ Configuration

### Environment Variables

#### Common (Both Modes)

```bash
# Application
APP_ENV=production
APP_URL=https://your-domain.com
PORT=8080
LOG_LEVEL=info

# Database
DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable

# Redis
REDIS_URL=redis://:password@host:6379

# JWT (auto-generated on first run)
JWT_PRIVATE_KEY_PATH=/app/keys/private.pem
JWT_PUBLIC_KEY_PATH=/app/keys/public.pem
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=168h

# Registration
ALLOWED_REGISTER=true
ALLOWED_EMAILS=  # Comma-separated patterns (e.g., *@company.com)

# Rate Limiting
RATE_LIMIT_ENABLED=true

# Security
BCRYPT_COST=12
```

---

## 🔧 Architecture

### Project Structure

```
whento/
├── cmd/
│   ├── main.go              # Single entry point
│   ├── init_cloud.go        # Cloud-specific initialization (tag: cloud)
│   ├── init_selfhosted.go   # Self-hosted initialization (tag: selfhosted)
│   └── licensegen/          # License generator CLI tool
├── internal/                # Business modules
│   ├── auth/                # JWT RS256, users, sessions
│   ├── calendar/            # CRUD, participants
│   ├── availability/        # Availabilities, recurrences
│   ├── ics/                 # iCalendar feed generation
│   ├── subscription/        # Cloud-only (tag: cloud)
│   ├── licensing/           # Self-hosted only (tag: selfhosted)
│   └── quota/               # Quota enforcement (both modes)
├── pkg/                     # Shared packages
│   ├── cache/               # Redis wrapper
│   ├── database/            # PostgreSQL + Redis
│   ├── jwt/                 # RS256 token management
│   ├── middleware/          # Auth, rate limiting, CORS
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
| Backend                 | Go 1.25+, Chi router, pgx/v5, go-redis/v9          |
| Frontend                | Vue 3, Vite 7, TypeScript, Tailwind CSS 4, Pinia 3 |
| Database                | PostgreSQL 16, Redis 7                             |
| Auth                    | JWT RS256 (asymmetric keys), bcrypt                |
| Licensing (Self-hosted) | Ed25519 cryptographic signatures                   |
| i18n                    | vue-i18n (FR/EN)                                   |
| iCalendar               | arran4/golang-ical                                 |

---

## 🐳 Deployment

### Docker Compose (Recommended)

```yaml
version: "3.8"

services:
  whento:
    image: ghcr.io/When-To/whento:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://whento:password@postgres:5432/whento
      - REDIS_URL=redis://:password@redis:6379
      - APP_URL=https://your-domain.com
    depends_on:
      - postgres
      - redis
    volumes:
      - jwt_keys:/app/keys

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: whento
      POSTGRES_USER: whento
      POSTGRES_PASSWORD: password
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    command: redis-server --requirepass password
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
  jwt_keys:
```

### Building from Source

```bash
# Self-hosted build (default)
make build
# or
go build -tags selfhosted -o bin/whento ./cmd/main.go
```

### Reverse Proxy

Example with Caddy:

```caddyfile
your-domain.com {
    reverse_proxy localhost:8080
}
```

---

## 🛠️ Development

### Prerequisites

- Go 1.25+
- Node.js 20+
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
# Run all tests
make test

# Test specific module
go test ./internal/auth/... -v

# Test with coverage
make test-coverage

# Build-specific tests
go test -tags cloud ./...
go test -tags selfhosted ./...
```

### Migrations

```bash
# Apply migrations
make migrate-up

# Rollback last migration
make migrate-down

# Check migration status
make migrate-status
```

### Frontend Commands

```bash
cd frontend

npm run dev         # Vite dev server
npm run build       # Production build
npm run type-check  # TypeScript checking
npm run lint        # ESLint
```

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

### Calendar Routes (`/api/v1/calendars`)

- `POST /` — Create calendar (requires verified email)
- `GET /` — List my calendars
- `GET /{id}` — Get calendar details
- `PATCH /{id}` — Update calendar
- `DELETE /{id}` — Delete calendar
- `GET /public/{token}` — Public calendar view
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

### iCalendar Routes (`/api/v1/ics`)

- `GET /feed/{ics_token}` — iCalendar subscription feed

### Billing Routes - Cloud Only (`/api/v1/billing`)

- `POST /checkout` — Create Stripe checkout session
- `POST /portal` — Create Stripe customer portal session
- `POST /webhook` — Stripe webhook handler

### License Routes - Self-hosted Only (`/api/v1/license`)

- `POST /activate` — Activate license with JSON key
- `GET /status` — Get license status and quota
- `DELETE /deactivate` — Deactivate license (admin only)

### Quota Routes - Both Modes (`/api/v1/quota`)

- `GET /status` — Get quota status (user or server-wide)

### Admin Routes (`/api/v1/admin`)

- `GET /users` — List all users
- `PATCH /users/{id}/role` — Update user role
- `DELETE /users/{id}` — Delete user
- `GET /users/{id}/calendars` — View user's calendars

---

## 🔐 Security Features

- **RS256 JWT** — Asymmetric keys (auto-generated at startup)
- **Bcrypt Password Hashing** — Cost factor 12
- **Email Verification** — Required before calendar creation
- **Rate Limiting** — Redis-backed with graceful fallback
  - Login: 5 req/min/IP
  - Register: 3 req/min/IP
  - Public endpoints: 60 req/min/IP
  - ICS feed: 30 req/min/IP
  - Authenticated: 100 req/min/user
- **Token Regeneration** — Separate public and ICS tokens can be regenerated
- **CORS Protection** — Configurable allowed origins
- **Security Headers** — HSTS, CSP, X-Frame-Options
- **SQL Injection Protection** — Parameterized queries via pgx

---

## 📊 Quota Enforcement

### Cloud Mode (Per User)

- **Free**: 3 calendars
- **Pro**: 100 calendars
- **Power**: Unlimited calendars

**Subscription Expiry Lock**: If a subscription expires while over quota (e.g., 50 calendars on Free tier), the user cannot create new calendars or access ICS feeds until usage drops to ≤3.

### Self-Hosted Mode (Server-Wide)

- **Community**: 30 calendars
- **Pro**: 300 calendars
- **Enterprise**: Unlimited calendars

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
| Self-host for your organization (internal) | ✅ Yes | No (or Pro/Enterprise for more calendars) |
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
