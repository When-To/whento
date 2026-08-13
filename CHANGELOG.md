# Changelog

All notable changes to WhenTo, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) as described in
[CONTRIBUTING → Release process](.github/CONTRIBUTING.md#release-process).

## How reliable is this file, and from when

This file was written after the fact, in August 2026, by reading the git history
back from `main`. Its accuracy is therefore not uniform:

- **From `v1.3.5` (2026-02-09) onwards** the history uses
  [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) with
  increasing consistency, and every entry below is derived directly from a commit
  subject. Treat these as accurate.
- **`v1.3.3` and `v1.3.4`** are the transition: `chore(deps)` appears, the rest is
  free-form.
- **`v1.0.0` through `v1.3.2`** predate the convention entirely. Those subjects are
  sentence-case prose, and the entries below are a hand-made summary of them. They
  are best-effort: use them to orient yourself, not as an audit trail. The commit
  range for each release is given so you can read the source instead.

Two further caveats, both consequences of the release process this file exists to
replace:

- **`v1.0.0` is not an ancestor of `main`.** The line that survived starts at
  `v1.0.1`. The `v1.0.0` entry below describes the initial release as tagged.
- **`v1.6.2` is not an ancestor of `main` either.** It was tagged on a branch whose
  commits were later replayed onto `main` under different hashes. The work it
  contains did ship, and is listed under `v1.6.2` below, but
  `git log v1.6.2..v1.6.3` does not describe it — the range spans the divergence
  and returns most of the history. This is the kind of thing an automated changelog
  would have made visible on the day rather than eight months later.

Entries only cover what reached `main`. Dependency bumps are folded into a single
line per release rather than listed individually.

---

## [Unreleased]

The work since `v1.6.3` is largely a technical audit and its remediation: coverage
went from 29% to a CI-enforced floor, the workflows were hardened and pinned, and
several defects the new tests surfaced were fixed.

### Added

- Live availability updates over server-sent events, so a participant's grid
  refreshes when somebody else writes to the same calendar (#78).
- Prometheus metrics on a listener of their own, off unless `METRICS_ENABLED` says
  otherwise, labelled by method, chi route pattern and status — never by path,
  token or identity.
- A database test harness (`internal/testutil/dbtest`) and browser tests that drive
  the write paths against a real backend (#65, #72).
- Operator documentation under `docs/`: backup and restore, Postgres/Redis
  upgrades, migration rollback, reverse proxy and `TRUSTED_PROXIES`, logging and
  privacy, Docker hardening.

### Changed

- The first day of the week is resolved from the calendar's timezone rather than
  from the viewer's locale (#77).
- The calendar views were rebuilt on one shared, tested week model (#58).
- `cmd/` was split into `wire.go` (dependency injection), `router.go` and one
  `routes_<domain>.go` per business domain; `main.go` now holds only `main()`,
  `run()` and the Swagger annotations.
- Frontend API types are generated from the Swagger spec into
  `frontend/src/types/api.generated.ts` and committed, so a backend response that
  changes shape shows up as a diff in the pull request that causes it.
- The hand-rolled dependency-update workflow was replaced by Dependabot, covering
  Go (both modules), npm, GitHub Actions, Dockerfiles, compose files and
  devcontainer features (#82).
- Every third-party action is pinned to a full commit SHA; every job has a
  `timeout-minutes`; workflows declare least-privilege permissions and
  `concurrency` groups.
- Toolchains aligned: Go 1.26 everywhere, Node 24, `goimports` v0.48.0,
  `golangci-lint` v2.12.2 (#51, #52).

### Fixed

- The access token is refreshed at most once at a time, instead of once per
  concurrent 401 (#73).
- An unmatched `/api` path answers 404 JSON instead of serving the SPA with a
  200 (#67).
- Six production defects surfaced by the technical audit, and three more that the
  test suite had until then only documented (#69).
- The participant e2e suite no longer fails when the run crosses a month
  boundary (#75).
- Three calendar reading and selection defects in the frontend (#63).
- `make dev-fullstack` now stops the backend when the frontend exits (#66).

### Security

- `govulncheck` (both build tags, both modules), CodeQL, `dependency-review` and
  Trivy image scanning before push; SBOM and signed build provenance on release
  artefacts.
- Personal data removed from log fields, with an AST guard
  (`pkg/logger/logfields_test.go`) that fails the build on a log field named after
  an address, a credential, a name or an IP.
- SPDX headers on every source file (#68).

---

## [v1.6.3] — 2026-06-18

_Range: `v1.6.1..v1.6.3` minus what shipped as v1.6.2._

### Changed

- The dependency-update workflow reports the total number of npm vulnerabilities
  it found and fixed, rather than only that it ran.

### Security

- `npm audit` runs as part of the dependency update workflow, and the run fails
  visibly when vulnerabilities remain.
- Dependency updates (#44).

## [v1.6.2] — 2026-05-13

### Added

- Rate limiting keeps working when Redis is unavailable: an in-memory fallback
  behind a circuit breaker, instead of failing open (#37).

### Changed

- `APP_ENV` defaults to `production` in `.env.example`.
- The per-variant `.env` and compose templates were removed; there is one of each.
- Environment-variable hygiene, with URL-shaped variables validated at startup so
  a malformed `DATABASE_URL` fails fast (#35).
- The devcontainer wires `DATABASE_URL`/`REDIS_URL`, applies migrations through
  `make migrate-up`, and pins its features.
- The licence public key is compiled in; `LICENSE_PUBLIC_KEY` is gone.
- README: cloud-only sections dropped from the self-hosted documentation.

### Fixed

- `.env.example` used a different port variable from `docker-compose.yml`.
- The container waits for the database with `pg_isready` rather than a fixed
  sleep; base images bumped (#36).

## [v1.6.1] — 2026-04-19

### Added

- A list view for the participant calendar, alongside the week and month
  views (#33).

### Changed

- Shared helpers extracted from the ICS feed handlers.
- Dependency updates (#32).

## [v1.6.0] — 2026-04-12

### Added

- A unified ICS feed aggregating every calendar a user owns into a single
  subscription URL (#29).

### Changed

- Dependency update pull requests now verify that the project still builds (#30).
- Dependency updates (#28, #31).

## [v1.5.1] — 2026-04-09

### Added

- Licence sales included in accounting, with a paginated licence list view (#25).
  _(Cloud-side; the licensing code has since been removed — see v1.6.2.)_

### Fixed

- `npm audit` was split into separate detect and fix phases so they run in the
  right order (#26).

### Changed

- Dependency updates (#27).

## [v1.5.0] — 2026-04-09

### Changed

- OCI metadata labels on the container image: source, documentation, vendor,
  licence (#24).
- Base images and tooling versions upgraded (#23).
- `baseUrl` removed from `tsconfig.json` for TypeScript 6.0 compatibility.
- Dependency updates (#21).

### Security

- Security audit remediations (#22).

## [v1.4.2] — 2026-03-12

### Fixed

- Calendar descriptions keep their line breaks, and are shown in every calendar
  view rather than only one (#20).

### Changed

- Dependency updates (#19).

## [v1.4.1] — 2026-03-04

### Added

- Anonymous participant registration can be enabled from the calendar creation
  form (#18).
- The event threshold may exceed the participant count when anonymous
  registration is enabled — the point being that more people may yet join (#17).

## [v1.4.0] — 2026-03-02

### Added

- Anonymous participant registration from the public calendar view: a visitor
  holding the link can add themselves without an account (#16).

### Changed

- README documents the SMTP and email environment variables.

## [v1.3.9] — 2026-03-02

### Fixed

- Three reported issues (#12, #13, #14) resolved together (#15).
- The self-hosted release body no longer carries the tag message verbatim.
  _(A first attempt at the problem that this changelog and the annotated-tag rule
  in CONTRIBUTING finally address.)_

### Changed

- Dependency updates (#11).

## [v1.3.8] — 2026-02-28

### Fixed

- The owner's email address is pre-populated only on the participant that
  actually matches it (#10).

### Changed

- Dependency updates (#9).

## [v1.3.7] — 2026-02-24

### Changed

- The dependency update workflow was refactored and given a git identity of its
  own, so its commits stop being attributed to a person.
- Dependency updates (#8).

## [v1.3.6] — 2026-02-09

### Fixed

- The privacy page described usage data collection that does not happen.

## [v1.3.5] — 2026-02-09

### Fixed

- `noindex` removed from the legal pages, which are meant to be findable.

### Changed

- Dependency updates (#7).

## [v1.3.4] — 2026-01-16

### Changed

- Dependency updates (#6).

## [v1.3.3] — 2026-01-14

### Added

- An automated dependency update workflow (#3), later replaced by Dependabot.
- `workflow_dispatch` on the CI workflow.

### Fixed

- The Node.js version comparison stripped the `^` prefix before comparing.

### Changed

- Frontend dependencies cleaned up. Dependency updates (#5).

## [v1.3.2] — 2026-01-07

### Fixed

- The SMTP `From` address is read from the environment (#2).

## [v1.3.1] — 2026-01-06

### Added

- Participant selection, which highlights the dates a chosen subset of
  participants has in common.

### Fixed

- Duplicate API calls on the initial load of the participant view.

## [v1.3.0] — 2026-01-05

### Fixed

- Data is reloaded when switching between week and month views.
- The participant view layout in month mode.
- `make dev-backend` and `make dev-fullstack` work without a built frontend.

### Changed

- The Go build cache moved to a Docker volume instead of a bind mount.
- Prettier formatting applied across the frontend.

## [v1.2.1] — 2025-12-17

### Fixed

- A `v-memo` ESLint error in the calendar grid.

## [v1.2.0] — 2025-12-17

### Added

- A compact/classic view-mode toggle for the calendar grids.

## [v1.1.6] — 2025-12-15

### Fixed

- Time controls stayed out of sync across several weekly grids on one page.

## [v1.1.5] — 2025-12-15

### Changed

- Mobile UX work across the calendar views and controls.

## [v1.1.4] — 2025-12-15

### Changed

- Prettier introduced; API calls in the participant view reduced.

## [v1.1.3] — 2025-12-13

### Added

- A responsive hamburger menu for mobile navigation.

## [v1.1.2] — 2025-12-12

### Fixed

- vue-i18n failed to compile email placeholders containing `@`.

## [v1.1.1] — 2025-12-12

### Added

- Missing i18n translations for the notification features.

### Changed

- Swagger documentation is generated once per CI run and shared between jobs as
  an artifact.

## [v1.1.0] — 2025-12-12

### Added

- Multi-channel notifications for threshold alerts.
- Improved touch gestures for selecting availability on mobile.

### Fixed

- Swagger annotations on the notification endpoints.
- TypeScript type inference for participant availabilities.

## [v1.0.2] — 2025-12-09

### Added

- `.env` file support for binary deployments, so the binary is configured the
  same way the container is.

## [v1.0.1] — 2025-12-09

### Added

- Multi-platform binary builds in the release workflow (Linux, Windows, macOS;
  amd64 and arm64).

### Changed

- `robots.txt` rules for the login and register pages.

## [v1.0.0] — 2025-12-08

Initial release: collaborative scheduling calendars with participant
availability, threshold-based event validation and iCalendar subscription feeds,
shipped as a single binary with the frontend embedded. GitHub workflows and
community templates landed with it.

_Note: this tag is not an ancestor of `main` — see the caveats at the top._

[unreleased]: https://github.com/When-To/whento/compare/v1.6.3...HEAD
[v1.6.3]: https://github.com/When-To/whento/releases/tag/v1.6.3
[v1.6.2]: https://github.com/When-To/whento/releases/tag/v1.6.2
[v1.6.1]: https://github.com/When-To/whento/releases/tag/v1.6.1
[v1.6.0]: https://github.com/When-To/whento/releases/tag/v1.6.0
[v1.5.1]: https://github.com/When-To/whento/releases/tag/v1.5.1
[v1.5.0]: https://github.com/When-To/whento/releases/tag/v1.5.0
[v1.4.2]: https://github.com/When-To/whento/releases/tag/v1.4.2
[v1.4.1]: https://github.com/When-To/whento/releases/tag/v1.4.1
[v1.4.0]: https://github.com/When-To/whento/releases/tag/v1.4.0
[v1.3.9]: https://github.com/When-To/whento/releases/tag/v1.3.9
[v1.3.8]: https://github.com/When-To/whento/releases/tag/v1.3.8
[v1.3.7]: https://github.com/When-To/whento/releases/tag/v1.3.7
[v1.3.6]: https://github.com/When-To/whento/releases/tag/v1.3.6
[v1.3.5]: https://github.com/When-To/whento/releases/tag/v1.3.5
[v1.3.4]: https://github.com/When-To/whento/releases/tag/v1.3.4
[v1.3.3]: https://github.com/When-To/whento/releases/tag/v1.3.3
[v1.3.2]: https://github.com/When-To/whento/releases/tag/v1.3.2
[v1.3.1]: https://github.com/When-To/whento/releases/tag/v1.3.1
[v1.3.0]: https://github.com/When-To/whento/releases/tag/v1.3.0
[v1.2.1]: https://github.com/When-To/whento/releases/tag/v1.2.1
[v1.2.0]: https://github.com/When-To/whento/releases/tag/v1.2.0
[v1.1.6]: https://github.com/When-To/whento/releases/tag/v1.1.6
[v1.1.5]: https://github.com/When-To/whento/releases/tag/v1.1.5
[v1.1.4]: https://github.com/When-To/whento/releases/tag/v1.1.4
[v1.1.3]: https://github.com/When-To/whento/releases/tag/v1.1.3
[v1.1.2]: https://github.com/When-To/whento/releases/tag/v1.1.2
[v1.1.1]: https://github.com/When-To/whento/releases/tag/v1.1.1
[v1.1.0]: https://github.com/When-To/whento/releases/tag/v1.1.0
[v1.0.2]: https://github.com/When-To/whento/releases/tag/v1.0.2
[v1.0.1]: https://github.com/When-To/whento/releases/tag/v1.0.1
[v1.0.0]: https://github.com/When-To/whento/releases/tag/v1.0.0
