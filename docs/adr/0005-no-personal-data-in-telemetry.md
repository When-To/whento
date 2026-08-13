# 0005 — No personal data in logs or metrics, enforced by an AST guard

**Status:** Accepted
**Date:** 2026-08

## Context

Two properties of this application make ordinary logging hygiene insufficient.

**The people in the data never agreed to anything.** A calendar is answered by
whoever was sent the link. They are not users, did not accept terms, and often did
not give a name. Whatever an instance writes about them, it writes about
third parties.

**The request path is a credential.** Because participant access is
capability-based ([ADR-0002](0002-capability-based-participant-access.md)), the
calendar token lives in the URL. A conventional access log — one line per request
with method, path and client IP — is therefore a log of *credentials next to the
addresses of the people using them*. The same holds for a metrics label
`path="/api/v1/availabilities/calendar/abc123…"`: it would put the token into the
monitoring system, into the scrape history, and into every dashboard.

The audit that produced this record found leaks in exactly the places unit tests
do not reach: a notification loop that queries three repositories, and
fire-and-forget goroutines. Driving each of them from a test would mean inventing
a fixture for code never built to have one, and the fixture would then be the
thing under test.

## Decision

**Nothing that identifies a person, a browser or a credential is written to logs
or exposed as a metric label.** In place of identities, correlation tags:
`logger.Fingerprint` produces a stable, non-reversible handle so two log lines can
be tied together without either naming anyone.

Specifically:

- **Logs** — structured JSON to stdout via `log/slog`. No email addresses, no
  names, no IP addresses, no User-Agent, no tokens, no request paths. The route
  *pattern* is logged instead of the path.
- **Metrics** — off unless `METRICS_ENABLED`, and on a listener of its own rather
  than on the application router, because the exposition names every route and
  reports pool saturation and Go build details: useful to an operator, free
  reconnaissance to anyone else. Labels are HTTP method, chi route **pattern**,
  status code, and build variant. The only free-form label values in the process
  are the two in `metrics.SetBuildInfo`, both fixed at link time.
- **No telemetry of any kind leaves the instance.** No analytics, no crash
  reporting, no phone-home, no update check, no vendor SDK. The only outbound
  connections are PostgreSQL, Redis and the configured SMTP server. Metrics are
  *pulled* by the operator's monitoring, never pushed.
- **Redis holds digests, not values.** Cache keys and rate-limit buckets are
  stored under a salted hash (`cache.SetKeySalt`,
  `middleware.SetRateLimitKeySalt`), so Redis never contains a calendar token, an
  ICS token, a participant id, an email address or an IP.

### The enforcement

A rule that depends on everyone remembering it is not a rule.
[`pkg/logger/logfields_test.go`](../../pkg/logger/logfields_test.go) parses every
non-test `.go` file under `internal/`, `pkg/` and `cmd/` with `go/parser`, walks
the AST for slog call sites (`Info`/`Warn`/`Error`/`Debug` and their `…Context`
variants, plus the typed `slog.String`/`Int`/… constructors), and fails the build
on any **field name** drawn from a forbidden list: `email`, `to`, `recipient`,
`token`, `public_token`, `ics_token`, `access_token`, `refresh_token`,
`magic_link`, `topic`, `challenge_id`, `participant_id`, `pid`, `name`,
`display_name`, `chat_id`, `ip`, `remote_addr`, `client_ip`, `user_agent`,
`password`, `path`, `url`, `query`, and others.

Three design choices in that guard are deliberate:

- **It checks the name, not the value.** A field called `email` holding something
  harmless is still a field the next person will fill with an address.
- **It covers code nobody wrote a privacy test for**, including a log line added
  tomorrow in a package that has no tests at all.
- **Exceptions are explicit and few.** `reset_url` (when SMTP is unconfigured the
  log *is* how a password reset is delivered — warn level, documented in
  [logging-and-privacy.md §5](../logging-and-privacy.md)); `webhook_url`
  (truncated to twenty characters, which is scheme and host and none of the secret
  path); and one file-scoped lift, `cmd/main.go:path`, for the build-time
  Prometheus exposition path. That last one is why `startMetricsServer` must stay
  in `cmd/main.go` — moving it moves it out from under the exception and fails the
  test.

## Consequences

**Good**

- The privacy claim in [logging-and-privacy.md](../logging-and-privacy.md) is
  checkable rather than aspirational, and stays true without anyone policing
  reviews for it.
- An operator can ship these logs to a third-party aggregator without shipping
  their users' data with them.
- A leaked log file or a compromised monitoring stack does not hand over calendar
  tokens.
- Metrics can be enabled without turning the exposition into an inventory of who
  uses the instance.

**Bad**

- **Debugging is harder, and this is the real cost.** "Which calendar was that?"
  is not answerable from the logs. The fingerprint correlates lines with each
  other but does not resolve back to a row; answering it means a database query
  with something learned elsewhere.
- The guard is name-based, so it is **defeatable** — `slog.String("recipient_addr", …)`
  passes. It raises the floor; it is not a proof.
- It only understands literal field names. A key built at runtime is invisible to
  it.
- The forbidden list has to grow as the vocabulary does, and it grew once already:
  `topic` and `challenge_id` read as harmless until you know that in this codebase
  a pub/sub topic *is* the calendar token and a challenge id is the handle to a
  live WebAuthn ceremony.
- Legitimate fields with unlucky names need an exception entry, which is friction
  by design.

## Alternatives considered

- **A redacting slog handler** that scrubs values on the way out. Rejected as the
  primary control: it needs a rule per shape of secret, it costs work on every log
  line, and it fails open — an unrecognised secret passes through. It would be a
  reasonable *addition*, not a replacement.
- **Review discipline alone.** This is what was in place, and it is what the audit
  found had not held.
- **A linter plugin** (a custom `golangci-lint` analyzer). Equivalent in power and
  strictly more machinery; a test in `pkg/logger` runs everywhere `go test` runs,
  including on a contributor's laptop before they push.
- **Logging identifiers but retaining logs briefly.** Rejected: retention is the
  operator's decision and the code cannot depend on it.

## When to revisit

Add to `forbiddenFields` whenever a new field name turns out to name something
sensitive in this domain — that is maintenance, not revision. Revisit the decision
itself only if debugging without identifiers becomes a genuine obstacle to
supporting operators; the answer then is a redacting handler *plus* the guard, and
an explicit statement in `docs/logging-and-privacy.md` of what changed.
