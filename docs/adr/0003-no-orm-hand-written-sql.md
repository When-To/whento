# 0003 — No ORM: hand-written SQL over pgx

**Status:** Accepted
**Date:** recorded 2026-08 (the choice predates the records)

## Context

The backend is a scheduling application over PostgreSQL. Its data access is not
uniform: most of it is ordinary CRUD, but the parts that matter — computing which
dates reach the participation threshold, expanding recurrence rules with their
exceptions across a range, aggregating several calendars into one ICS feed — are
set-oriented queries that a query builder tends to make longer rather than
shorter.

Go's ORM options (GORM, ent, Bun) each bring a schema-definition mechanism, a
migration story and a query DSL. The project already has golang-migrate and a
three-directory migration layout it needs for the `common`/`cloud`/`selfhosted`
split; an ORM's own migration tooling would be a second, competing source of
truth about the schema.

## Decision

No ORM. Hand-written SQL executed through `pgx/v5`, with one repository type per
aggregate under `internal/<domain>/repository/`.

The supporting conventions:

- **The service declares the interface, the repository satisfies it.**
  `type CalendarRepository interface` lives in `internal/calendar/service`, not in
  `repository/`. The concrete type satisfies it structurally. This is what makes
  services testable with hand-written mocks and no mocking library.
- **Parameterised queries only.** `$1`-style placeholders; never string
  concatenation into SQL.
- **Errors are classified by SQLSTATE**, through
  [`pkg/dberr`](../../pkg/dberr/dberr.go), using `errors.As` on `*pgconn.PgError`.
  Never by message text: the text is localised by the server's `lc_messages`,
  reworded between major versions, and pgx wraps it, so an equality check against
  an English sentence is a coin toss.
- **Repositories are tested against a real PostgreSQL**, via
  `internal/testutil/dbtest`. Those tests skip when `DATABASE_URL` is unset — so a
  laptop without Postgres still runs `make test` green — which means CI is the
  only place they actually execute, and CI migrates first.

## Consequences

**Good**

- The query that runs is the query in the file. Performance work is reading and
  editing SQL, not reverse-engineering a generator, and `EXPLAIN` output maps
  directly onto source.
- PostgreSQL features are available without asking whether the abstraction
  supports them: `ON CONFLICT`, CTEs, window functions, advisory locks (the quota
  lock in `internal/calendar/repository/quota_lock.go`), array and range types.
- One source of truth for the schema — the migrations — with no model definitions
  that can silently disagree with it.
- pgx is used directly, so its connection pooling, statement cache and
  `CopyFrom` are all reachable; pool statistics feed the metrics collector.
- Few dependencies. The root module's direct requirements are twelve packages.

**Bad**

- **Boilerplate.** Every column appears in the `SELECT` list, in the `Scan`, and
  in the struct. Adding a column is a mechanical edit in several places, and
  forgetting one is a compile error at best and a zero value at worst.
- **No compile-time link between SQL and Go.** A renamed column is caught at
  runtime, by a test, not by the type checker — which is precisely why repository
  tests run against a real database rather than a mock.
- Cross-cutting concerns must be re-implemented per repository: pagination,
  soft-delete semantics, `updated_at` maintenance.
- N+1 queries are possible and nothing warns about them; they have to be caught in
  review.

## Alternatives considered

- **GORM.** The most common choice, and the least appealing here: reflection-based,
  its generated SQL is hard to predict, and its `AutoMigrate` would compete with
  golang-migrate.
- **ent.** Strong compile-time guarantees, but its schema-as-Go-code model is
  another source of truth about the schema, and the graph traversal API is a poor
  fit for the aggregate queries that are the interesting part of this domain.
- **sqlc.** The closest to a real contender: hand-written SQL, generated
  type-safe Go, which would remove most of the boilerplate and the missing
  compile-time link at once. It was not adopted, and the cost of adopting it now
  is that it is all-or-nothing per query file plus a code-generation step in the
  build. It remains the alternative worth taking seriously.
- **squirrel / goqu (query builders).** Solves the concatenation risk that
  parameterised queries already solve, and makes complex queries harder to read
  rather than easier.

## When to revisit

If repository boilerplate starts dominating the diff of ordinary changes, or if a
column rename causes a production incident, **sqlc** is the migration to evaluate
— it preserves everything in the "Good" list above. An ORM is not: reversing this
decision toward GORM or ent would mean rewriting the queries that are the reason
for the decision.
