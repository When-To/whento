# Developer onboarding

Getting from a fresh clone to a change that passes CI. If you want to know *how
the system is put together* rather than how to run it, read
[architecture.md](architecture.md) first — this page assumes it.

---

## 1. The two things that will confuse you first

Read these before anything else. Both cause failures that look like something
else entirely.

### Build tags

Every build is `cloud` or `selfhosted`. Whole `.go` files are selected by tag, so:

```bash
go build ./...        # WRONG — excludes files, does not reflect the code that ships
go test ./...         # WRONG — same problem
make build            # selfhosted (the default)
make test             # go test -tags selfhosted, both modules
```

If you call Go directly, always pass `-tags selfhosted` or `-tags cloud`. Both
variants must compile; CI builds both. See
[ADR-0001](adr/0001-dual-build-tags.md).

### Two generated inputs the compiler needs

Go will not compile this repository from a bare checkout, and the error will not
say why in an obvious way:

- **`web/dist/`** — `//go:embed` is resolved at compile time.
  `make ensure-dist-placeholder` writes a stub; `make build` puts the real thing
  there.
- **`docs/swagger/`** — `cmd/main.go` imports the generated package.
  `make swagger` produces it (needs the `swag` CLI).

Both directories are git-ignored. If you see `pattern dist/*: no matching files`
or a missing `docs/swagger` import, this is why.

## 2. Setting up

### Dev container (recommended)

Open the repository in VS Code and reopen in the container. `post-create.sh`
installs the pinned tools, `npm install`s the frontend, enables the git hooks,
applies migrations and generates the JWT keys. Ports 8080, 5432 and 6379 are
forwarded.

### Manually

Requirements:

| Tool | Version | Why pinned |
| --- | --- | --- |
| Go | 1.26 | `go.mod`, CI, every Dockerfile |
| Node | 24 (LTS) | CI, Dockerfiles, Dependabot ignores majors past it |
| Docker + Compose | — | Postgres and Redis for development |
| `goimports` | **v0.48.0** | Output differs between releases; a mismatch fails `make format-check-go` |
| `golangci-lint` | v2.12.2 | Findings differ between releases |
| `swag` | v1.16.6 | Generates `docs/swagger/` |
| `migrate` | v4.19.1 | Must match what the Dockerfiles install |

The pinned versions are duplicated in `Makefile`, `.githooks/pre-commit`,
`.devcontainer/post-create.sh` and the workflows. If you bump one, bump all of
them.

```bash
go install golang.org/x/tools/cmd/goimports@v0.48.0
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
go install github.com/swaggo/swag/cmd/swag@v1.16.6
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1
```

Then:

```bash
cp .env.example .env          # edit DATABASE_URL / REDIS_URL if not using dev-db
make hooks                    # once per clone — see below
make dev-db                   # postgres + redis via docker-compose.dev.yml
./scripts/generate-keys.sh    # JWT keypair into keys/ (git-ignored)
make migrate-up               # needs DATABASE_URL from .env
(cd frontend && npm install)
make dev-fullstack
```

`make` auto-loads `.env` (`-include .env` plus `export`), which is where the
`migrate-*` targets get `DATABASE_URL`.

### `make hooks` is opt-in, once per clone

It sets `core.hooksPath` to the versioned `.githooks/`, which is *local* git
config and therefore cannot be committed. The `pre-commit` hook formats staged
files and re-stages them, so commits land formatted. Files that are only
partially staged (`git add -p`) are skipped with a warning.

**Do not rely on it.** `git commit --no-verify` walks past it, and a clone that
never ran `make hooks` has no hook at all. CI is the real gate — `make
format-check` runs there.

## 3. Running it

| Command | What it does |
| --- | --- |
| `make dev-db` | Postgres + Redis only |
| `make dev` | Backend on **:8080**, serving the embedded (possibly placeholder) frontend |
| `make dev-fullstack` | Frontend on **:8080** proxying `/api` to the backend on **:5173** |
| `make dev-backend` | Backend on :5173, for use with a separately started frontend |
| `make dev-frontend` | Vite on :8080 |

**The ports in `dev-fullstack` are inverted on purpose**: you always browse
:8080, whichever mode you are in, and Vite proxies the API through. Ctrl+C stops
both halves.

Redis is optional in every mode. Without it the cache is a no-op, the rate
limiter falls back to an in-memory backend, and live updates stay within the one
process — all deliberate, none fatal.

Swagger UI is at `http://localhost:8080/swagger/`.

## 4. Making a change

### Adding a backend domain

Three edits, in this order:

1. `internal/<domain>/{handlers,models,repository,service}` — the service declares
   the repository interface it needs; the repository satisfies it.
2. A constructor block in `cmd/wire.go` (`buildHandlers`), and a field on the
   `handlers` struct.
3. `cmd/routes_<domain>.go`, plus one `register<Domain>Routes` line in
   `newRouter` (`cmd/router.go`).

Rate limits go through the `routeLimiter` helper in `cmd/ratelimit.go` —
`l.on(r, perIP(60, time.Minute)).Get(…)` for a single route, `l.use(r, …)` for a
group. Never hand-write an `if cfg.RateLimitEnabled` branch; that is exactly what
the helper replaced.

### Changing the API surface

The chain is annotations → Swagger → TypeScript:

```bash
make swagger      # regenerate docs/swagger/ from the // @... annotations
make types        # regenerate frontend/src/types/api.generated.ts
make types-check   # non-mutating: fails if the committed types are stale
```

`docs/swagger/` is git-ignored; `frontend/src/types/api.generated.ts` **is
committed**, and CI runs `make types-check`. Commit the regenerated types with
the change that causes them.

### Adding a migration

```
migrations/{common,cloud,selfhosted}/NNN_snake_case_name.{up,down}.sql
```

- The numbering space is **shared across all three directories**. A number may be
  reused between `cloud/` and `selfhosted/` only, since one build gets one of
  them (`005` and `013` are both reused that way).
- Highest common migration today is `014`; **the next one is `015`**.
- Always write both `.up.sql` and `.down.sql`.
- Never run `migrate` against `migrations/` directly — `scripts/build-migrations.sh`
  merges the directories into a temporary `migrations-build/` that the `migrate-*`
  targets delete afterwards.

### Adding user-facing text

Every string goes through vue-i18n: `frontend/src/locales/{en,fr}.json`. A new
locale also needs the backend email templates under
`internal/auth/{handlers,service}/templates/locales/` and
`internal/notify/**/templates/locales/`; the procedure is documented in
`frontend/src/i18n.ts`.

## 5. Conventions a reviewer will hold you to

### Go

- **`goimports -w -local github.com/whento`.** Plain `gofmt` is not enough and
  `make format-check-go` will fail.
- Every `.go` file starts with the three-line licence header. When a `//go:build`
  line is present it goes on line 5:

  ```go
  // WhenTo - Collaborative event calendar for self-hosted environments
  // Copyright (C) 2025 WhenTo Contributors
  // SPDX-License-Identifier: BSL-1.1
  ```

- Tests are colocated (`foo_test.go`, same package), **table-driven** with
  `t.Run`, and use **hand-written mocks** — no testify, no mock library. Handler
  tests use `httptest` plus `internal/testutil`.
- Classify database errors with `pkg/dberr` (SQLSTATE), never by message text.
- Log fields are checked by an AST guard: a field named `email`, `token`, `ip`,
  `path`, `name`, `participant_id`… fails the build. Use `logger.Fingerprint`.
  See [ADR-0005](adr/0005-no-personal-data-in-telemetry.md).

### Frontend

- `<script setup lang="ts">` everywhere; TypeScript `strict` plus
  `noUnusedLocals` / `noUnusedParameters`; alias `@/*` → `src/*`.
- Prettier: single quotes, semicolons, print width 100, `arrowParens: avoid`.
  Note its scope — `frontend/{src,dev,e2e,e2e-backend,scripts}` plus the
  frontend's root config files. **Markdown is not formatted by anything**, so
  documentation edits cannot break `make format-check`.
- Pinia stores are setup style: `defineStore('name', () => { … })`.
- Tailwind v4 tokens live in `frontend/src/style.css` (`@theme`).
  `frontend/tailwind.config.js` is a leftover v3 file — prefer the CSS.
- Licence header as a block comment at the top of every `.ts` / `.vue` file.
- All HTTP through `src/api/client.ts`. Never axios from a component.

## 6. Before you push

```bash
make format-check     # goimports + prettier, non-mutating
make lint-go          # golangci-lint: 2 modules × 2 build tags
make test             # go test, both modules
make types-check      # generated frontend types match the annotations
go build -tags selfhosted -o /dev/null ./cmd/
go build -tags cloud      -o /dev/null ./cmd/
(cd frontend && npm run type-check && npm run lint && npm run test:coverage)
```

The `/check` command in `.claude/commands/` runs the equivalent of the above in
one go.

Two things about the test suite worth knowing before you are surprised by them:

- **Repository tests skip when `DATABASE_URL` is unset.** A laptop without
  Postgres runs `make test` green while never executing them. CI is where they
  actually run, and CI migrates first. If you touched a repository, set
  `DATABASE_URL` locally and re-run.
- **Coverage is a ratchet.** CI enforces floors for the root module and `pkg/`
  separately, plus statement/branch/function/line thresholds in
  `frontend/vitest.config.ts`. They are set just under what the suite reaches
  today. Raise them when you add tests; never lower them to make a build pass.

## 7. Opening the pull request

- Branch from `main`.
- **The pull request title must be a Conventional Commit** — `feat:`, `fix:`,
  `docs:`, `chore(deps):`, `refactor:`, … CI checks it, because merges are
  squashed and that title becomes the commit subject on `main` and the input to
  the changelog. See
  [CONTRIBUTING → Commit messages](../.github/CONTRIBUTING.md#commit-messages).
- Explain the *why* concisely. This codebase's comments and commit messages
  document reasoning rather than restating the code; match that.
- Update `CHANGELOG.md` under `## [Unreleased]` if the change is user-visible.
- `CODEOWNERS` routes review; workflows, migrations, `pkg/jwt`, `pkg/middleware`,
  `internal/auth`, `internal/mfa` and `internal/passkey` are all flagged as
  sensitive.

## 8. Where things are

| Question | File |
| --- | --- |
| How is it all wired? | [architecture.md](architecture.md) |
| Why is it like that? | [adr/](adr/README.md) |
| How do I run it in production? | [root README](../README.md), then the runbooks in [docs/README.md](README.md) |
| What gets logged? | [logging-and-privacy.md](logging-and-privacy.md) |
| How do I cut a release? | [CONTRIBUTING → Release process](../.github/CONTRIBUTING.md#release-process) |
