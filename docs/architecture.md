# Architecture

How WhenTo is put together, for someone about to change it. The
[root README](../README.md) is the user- and operator-facing documentation; this
page is the contributor-facing one. The decisions that shaped it — and the reasons
they are not up for casual revision — are recorded as
[ADRs](adr/README.md).

Everything below is a statement about the code as it stands, with file references,
so it can be re-checked rather than believed.

---

## 1. What ships

One executable. `bin/whento` contains the HTTP server, the SQL, the migration
files (in the container image), the generated Swagger UI and the compiled Vue
frontend. It needs PostgreSQL. Redis is optional and every feature that uses it
degrades rather than fails.

The frontend is embedded at compile time by
[`web/embed.go`](../web/embed.go):

```go
//go:embed dist/*
//go:embed dist/assets/*
var frontendFS embed.FS
```

This is why `web/dist/` must exist for the Go build to succeed at all —
`make ensure-dist-placeholder` writes a stub when you have not built the real
frontend. See [ADR-0004](adr/0004-single-binary-embedded-frontend.md).

## 2. Two build variants, always both

Every build is either `cloud` or `selfhosted`, selected by a Go build tag and, on
the frontend, by `VITE_BUILD_TYPE`. `BUILD_TYPE ?= selfhosted` in the Makefile.

```
go build ./...     # wrong: whole files are excluded, this is not the code that ships
make build         # selfhosted
make build BUILD_TYPE=cloud
```

There are no build-tag-exclusive *packages*. What differs is the calendar
allowance and the SEO behaviour, expressed as symmetric file pairs that must
expose the same symbols:

| Pair | Symbols |
| --- | --- |
| [`cmd/init_cloud.go`](../cmd/init_cloud.go) / [`cmd/init_selfhosted.go`](../cmd/init_selfhosted.go) | `InitServices`, `RegisterQuotaRoutes`, `buildType` |
| [`internal/quota/cloud.go`](../internal/quota/cloud.go) / [`internal/quota/selfhosted.go`](../internal/quota/selfhosted.go) | `CalendarCounter`, the quota service constructor and its methods |

CI builds, lints and vulnerability-scans both. Adding a cloud-only feature without
its self-hosted counterpart breaks the build. See
[ADR-0001](adr/0001-dual-build-tags.md).

## 3. Repository layout

```
cmd/                     package main: process lifecycle, DI, routing
internal/<domain>/       business domains, not shared outside this module
  handlers/                HTTP: decode, call service, encode
  models/                  domain types and their JSON shapes
  repository/              SQL against pgx
  service/                 business rules; declares the repository interfaces it needs
internal/config/         environment parsing and startup validation
internal/testutil/       test helpers, incl. dbtest (a real-Postgres harness)
pkg/                     a second Go module: reusable, no build tags, no domain knowledge
web/                     //go:embed of the built frontend + SEO meta injection
frontend/                Vue 3 + Vite application
migrations/              golang-migrate SQL, split common/cloud/selfhosted
scripts/                 migration assembly, key generation, container entrypoint
build/                   the Dockerfiles CI actually builds
docs/                    this documentation; docs/swagger/ is generated
```

`go.work` declares **two modules**: `github.com/whento/whento` (root) and
`github.com/whento/pkg` (`pkg/`, wired via a `replace`). This matters more often
than it looks: `./...` never crosses a module boundary, so anything that walks
packages — tests, lint, coverage, `govulncheck` — needs a second invocation from
inside `pkg/`. That is why `make test` is `test-root` plus `test-pkg`, and why
`make lint-go` runs four times (two modules × two build tags).

The domains, as they stand: `auth`, `availability`, `calendar`, `ics`, `mfa`,
`notify`, `passkey`, `quota`, `seo`. Only `quota` and `seo` deviate from the
four-directory shape; both are small enough to be flat files.

## 4. Startup, in order

All of it is in `cmd/`, and it is split by *question answered* rather than by
size:

| File | Answers |
| --- | --- |
| [`main.go`](../cmd/main.go) | `main()`, `run()`, the Swagger annotation block, the metrics listener |
| [`wire.go`](../cmd/wire.go) | "what is this built from" — `deps`, `handlers`, `buildHandlers` |
| [`router.go`](../cmd/router.go) | "what is it reachable at" — global middleware, `newRouter`, the fallbacks |
| [`routes_<domain>.go`](../cmd/) | one per domain: paths, per-route auth, per-route rate limits |
| [`ratelimit.go`](../cmd/ratelimit.go) | `routeLimiter`, `perIP` / `perUser` / `perPathIP` |
| [`services.go`](../cmd/services.go), `init_*.go` | the build-variant seam |
| [`buildinfo.go`](../cmd/buildinfo.go) | `Version`, `BuildDate`, `VCSRef`, set by `-ldflags` |

`run()` is the only place that owns something needing to be closed, and each
`defer` sits next to the thing it releases. `main()` does nothing but set the exit
status — `os.Exit` skips deferred calls, so a single `os.Exit` inside the startup
sequence would leak the pool, the Redis client and the broadcast broker.

The sequence:

1. `config.Load()`, then `cfg.Validate()` — malformed URL-shaped variables
   (`DATABASE_URL`, `REDIS_URL`, `APP_URL`) fail fast. `cfg.Warnings()` covers the
   incoherences that leave one feature dead but the instance serviceable; those
   are logged, not fatal.
2. Logger (`log/slog`, JSON to stdout), build info, Swagger version overridden
   from the linker so `/swagger` describes the binary actually serving it.
3. PostgreSQL pool; pool statistics registered with the metrics collector.
4. Redis — **optional**. On failure the client is `nil` and everything downstream
   takes its degraded path: `cache.NewRedisCache(nil)` is a no-op cache, the rate
   limiter falls back to an in-memory backend, `broadcast.NewRedisBroker(nil, …)`
   becomes a process-local hub, and the readiness probe reports Redis as absent
   rather than down.
5. JWT manager, cache key salt, email service, quota services (`InitServices`,
   build-tag specific), rate limiter, broadcast broker.
6. `buildHandlers(d)` — the whole of the dependency injection.
7. `newRouter(d, h, spaHandler)`.
8. `http.Server` with a `BaseContext` that shutdown can cancel, and a metrics
   listener on a **separate port** when `METRICS_ENABLED` is set.

Shutdown: on SIGINT/SIGTERM, `srv.Shutdown` gets a 10 s budget; two seconds in,
the base context is cancelled so the SSE streams — which never end on their own —
are told to leave without aborting ordinary in-flight requests.

## 5. Dependency injection

Manual, centralized, no framework. `buildHandlers` in
[`cmd/wire.go`](../cmd/wire.go) constructs repositories, then services, then
handlers, in that order, and returns one `handlers` struct with a field per
handler.

The order is not free. Three edges cross module boundaries and are commented in
place:

- `userRepo`, the MFA repository and the passkey repository are built with the
  auth module and read by the MFA, passkey and auth handlers;
- the calendar repositories are built with the calendar module and read by the
  notification service;
- the availability **service** is constructed after the notification service,
  because it notifies through it — its repositories are still declared with the
  rest of its module.

**Adding a domain is three edits**: a constructor block in `cmd/wire.go`, a new
`cmd/routes_<domain>.go`, and one line in `newRouter`.

Note the direction of the interfaces: a service declares the repository interface
it needs (`type CalendarRepository interface` lives in
`internal/calendar/service`, not in `repository/`). The concrete repository
satisfies it structurally. That is what makes the services testable with
hand-written mocks and no mocking library.

## 6. A request, end to end

Global middleware, in registration order, from `newRouter`:

```
RequestID → Logger → Metrics → Recoverer → SecurityHeaders → LimitRequestSize(1MB) → CORS
```

Two orderings there are load-bearing rather than cosmetic:

- `Metrics` sits **outside** `Recoverer`, so a panic is counted as the 500 the
  client received rather than not counted at all.
- chi's `RealIP` is **deliberately absent and must stay absent**. It rewrites
  `r.RemoteAddr` from `X-Forwarded-For` for every request, before anything can ask
  whether the connection came from a trusted proxy — which would make
  `TRUSTED_PROXIES` decoration and every per-IP limit one client-chosen header
  away from being bypassed. Proxy headers are honoured in exactly one place,
  `middleware.IPKeyFunc`, and only for connections originating from a configured
  proxy. See [reverse-proxy.md](reverse-proxy.md).

### 6a. An owner reading their calendars

`GET /api/v1/calendars` →

1. global middleware above;
2. `middleware.Auth(jwtManager, cacheStore)` — validates the Ed25519-signed JWT
   and puts the user id and role in the request context;
3. `routeLimiter` bucket `perUser(100, time.Minute)`, keyed on the authenticated
   user id (which only means anything *below* `Auth` — above it the key is empty
   and the limiter passes everything through);
4. `CalendarHandler.ListMyCalendars` decodes, calls `CalendarService`, and encodes;
5. `CalendarService` applies the rules and calls the repository through the
   interface it declared;
6. the repository runs hand-written SQL on the pgx pool; PostgreSQL errors are
   classified by SQLSTATE through [`pkg/dberr`](../pkg/dberr/dberr.go), never by
   message text;
7. `httputil.JSON` writes the standard envelope: `{"success":…, "data":…}`, or
   `{"success":false, "error":{…}}` on failure.

### 6b. A participant answering from a shared link

`POST /api/v1/availabilities/calendar/{token}/participant/{pid}` →

There is **no `Auth` middleware on this path, by design**. The calendar's public
token plus the participant's UUID *are* the authorisation; there is no
per-participant secret to verify. Anyone holding the link can act as that
participant. See [ADR-0002](adr/0002-capability-based-participant-access.md).

The route carries `perIP(60, time.Minute)` and a
`PublishChanges(broker)` middleware that emits one notice per successful write —
placed on the group rather than inside the nine service methods, so that a tenth
write route cannot silently stop notifying anyone.

Browsers watching the same calendar hold an `EventSource` on
`GET /api/v1/availabilities/calendar/{token}/events`, which has a bucket of its
own (`perPathIP(300, time.Minute)`): a long-lived connection the browser
re-establishes after every hiccup would otherwise burn the participant API's
budget. The notices carry **no data**, only the fact that something changed; the
listener refetches through the normal read path, so a notice can never leak a
field the recipient is not allowed to see, and there is one read model rather
than two.

### 6c. Anything unmatched

`registerFallbacks` runs last, because `r.NotFound` only propagates to
sub-routers already mounted. An unmatched `/api` path answers 404 in the JSON
envelope — serving the SPA for it (200, a page of HTML) had hidden a broken seed
script and a health check in this repository's own CI pointing at a route that
never existed. Everything else falls through to the embedded SPA.

## 7. `pkg/` — the second module

Reusable code with no domain knowledge and **no build tags**. If it needs to know
what a calendar is, it belongs in `internal/`.

| Package | What it is |
| --- | --- |
| `broadcast` | Live-update fan-out. Process-local hub, or Redis pub/sub across instances. |
| `cache` | Redis-backed cache; `NewRedisCache(nil)` is a working no-op. Keys are stored as salted digests, so Redis never holds a token, a participant id or an address. |
| `database` | pgx pool and Redis client construction, pool statistics, readiness pingers. |
| `datevalidation` | Date and holiday helpers (e.g. `IsHolidayEve`). |
| `dberr` | Classifies PostgreSQL errors by SQLSTATE via `errors.As` on `*pgconn.PgError` — never by message text, which is localised and reworded between major versions. |
| `email` | SMTP sending; reports `IsConfigured()` so features degrade instead of erroring. |
| `httputil` | The `{success, data, error}` response envelope and the error codes. |
| `jwt` | Ed25519 access/refresh tokens. The signing method is pinned, with a test guarding against algorithm confusion. |
| `logger` | `log/slog` setup, `Fingerprint` for correlating without identifying, and the AST guard described below. |
| `metrics` | Prometheus collectors and the exposition handler. Labels are method, chi route *pattern*, status, build variant — nothing else. |
| `middleware` | `RequestID`, `Logger`, `Metrics`, `Recoverer`, `SecurityHeaders`, `LimitRequestSize`, `CORS`, `Auth`, `RequireRole`, the rate limiter and its key functions, `SetTrustedProxies`. |
| `models` | Shared entity scaffolding, enums, list/pagination types. |
| `validator` | Request validation and the email matcher. |

## 8. Data

**No ORM.** Hand-written SQL over `pgx/v5`, one repository per aggregate. See
[ADR-0003](adr/0003-no-orm-hand-written-sql.md).

Migrations are golang-migrate, in three source directories —
`migrations/{common,cloud,selfhosted}` — merged at build time by
[`scripts/build-migrations.sh`](../scripts/build-migrations.sh) into a temporary
`migrations-build/` that every `make migrate-*` target deletes afterwards. Never
run `migrate` directly against `migrations/`.

The numbering space is **shared across all three directories**, with one
exception: a number may be reused between `cloud/` and `selfhosted/`, since only
one of them is ever copied into a given build. `005` and `013` are both reused
that way. The highest common migration today is `014`; the next one is `015`.

Migrations run automatically at container start. What to do when one of them is
the problem is [migration-rollback.md](migration-rollback.md).

## 9. Frontend

Vue 3 with `<script setup lang="ts">`, Pinia (setup style), vue-router 5,
vue-i18n, Tailwind v4, Vite. TypeScript `strict` plus `noUnusedLocals` and
`noUnusedParameters`.

```
frontend/src/{api,components,composables,config,locales,router,stores,types,utils,views}
```

Two rules a reviewer will hold you to:

- **All HTTP goes through [`src/api/client.ts`](../frontend/src/api/client.ts)** —
  a singleton `apiClient` with `baseURL: /api/v1`, automatic refresh on 401 (at
  most one refresh in flight at a time), which already unwraps `response.data.data`.
  Never call axios from a component.
- **All user-facing strings go through vue-i18n**
  (`src/locales/{en,fr}.json`). Adding a locale means the frontend files *and* the
  backend email templates under `internal/auth/{handlers,service}/templates/locales/`
  and `internal/notify/**/templates/locales/`.

`src/types/api.generated.ts` is generated and **committed**. The chain is: Go
`// @…` annotations → `docs/swagger/swagger.json` (git-ignored) → Swagger 2.0
converted to OpenAPI 3 in memory → `api.generated.ts`. Committing the output is
the point: a backend response that changes shape shows up as a reviewable diff in
the pull request that causes it. Regenerate with `make types`; `make types-check`
fails if it is stale, and CI runs it.

The variant guard is `isCloud` / `isSelfHosted` from
`src/composables/useBuildType.ts` — see `src/router/index.ts` for the pattern.

## 10. What the tests enforce, beyond correctness

Several tests in this repository are *guards*: they exist to make a rule
unbreakable by the next person, not to test a function.

| Guard | Enforces |
| --- | --- |
| [`pkg/logger/logfields_test.go`](../pkg/logger/logfields_test.go) | Parses every non-test `.go` file under `internal/`, `pkg/` and `cmd/` and fails on any slog field named after personal data — `email`, `token`, `ip`, `name`, `path`, `participant_id`, `topic`, … See [ADR-0005](adr/0005-no-personal-data-in-telemetry.md). |
| `pkg/middleware` metric/log tests | The route *pattern* is recorded, never the request path — in this API the path carries the credential. |
| `pkg/cache` key tests | A cache key never contains its input, and hashing is deterministic across instances. |
| `pkg/jwt` | The signing method is pinned (algorithm-confusion guard). |
| `cmd/routes_test.go` | Walks the assembled chi route table, without needing the embedded frontend. |
| `cmd/dockerfiles_test.go` | The Dockerfiles stay consistent with what the build expects. |

Coverage is a **ratchet**, not a target: CI enforces floors (root and `pkg/`
separately for Go, and statement/branch/function/line thresholds in
`frontend/vitest.config.ts`) set just under what the suite reaches today. Raise
them when tests land; never lower them.

## 11. Where to look next

- [adr/](adr/README.md) — why the load-bearing decisions are what they are.
- [development.md](development.md) — getting a working environment and your first
  change through CI.
- [logging-and-privacy.md](logging-and-privacy.md) — what is written, what is
  deliberately not, and what Redis holds.
- The operator runbooks listed in [docs/README.md](README.md).
