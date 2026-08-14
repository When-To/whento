# Contributing to WhenTo

Thank you for your interest in contributing to WhenTo!

## How to Contribute

### Reporting Bugs

1. Check existing [issues](https://github.com/When-To/whento/issues) to avoid duplicates
2. Use the bug report template
3. Include reproduction steps, expected vs actual behavior

### Suggesting Features

1. Open a [discussion](https://github.com/When-To/whento/discussions) first
2. Explain the use case and why it would be valuable
3. If approved, create an issue with the feature request template

### Pull Requests

1. **Fork** the repository
2. **Create a branch** from `main`: `git checkout -b feature/your-feature`
3. **Enable the git hooks** (once per clone): `make hooks`
4. **Make your changes** following the code style guidelines
5. **Test your changes**: `make test`
6. **Commit** with clear messages
7. **Push** to your fork
8. **Open a Pull Request** against `main`

### Git Hooks

Run `make hooks` once after cloning (done automatically in the devcontainer). It
points `core.hooksPath` at the versioned `.githooks/` directory.

The `pre-commit` hook formats the files staged for the commit and re-stages them,
so commits always land formatted:

- Go files → `goimports -w -local github.com/whento`
- Frontend files → `prettier --write`

Files that are only partially staged (`git add -p`) are skipped with a warning,
to avoid pulling unstaged work into the commit. Use `git commit --no-verify` to
bypass the hook.

### Code Style

#### Go

- Follow standard Go conventions
- Format with `make format-go` (goimports with `-local github.com/whento`), not plain `gofmt`
- Add tests for new functionality
- Use meaningful variable names

#### Frontend (Vue/TypeScript)

- Follow existing component patterns
- Use TypeScript strict mode
- Format with `make format-frontend` (prettier) and run `npm run lint` before committing

`make format` runs both; `make format-check` verifies without modifying.

### Commit Messages

WhenTo follows [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):

```text
feat: add calendar sharing via email
fix(ics): correct timezone handling in ICS export
docs: update deployment instructions
refactor: simplify availability calculation
chore(deps): update backend and frontend dependencies
```

The grammar is `type(optional-scope): subject`, with an optional `!` before the
colon for a breaking change. Accepted types:

`feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`,
`chore`, `revert`

Explain the **why**, concisely. The subject line is the part everyone reads;
the body is where the reasoning goes.

#### This is enforced on the pull request title, not on your commits

The **Validate PR Title** job in CI checks
`pull_request.title` against the grammar above.

Two reasons it is checked there rather than commit by commit:

- Pull requests are **squash-merged**, so the pull request title — not your
  individual commit subjects — becomes the commit message on `main`, and is
  therefore what the changelog and the release notes are built from.
- The `pre-commit` hook is **opt-in per clone** (`make hooks`) and is bypassed by
  `git commit --no-verify`, so a local hook could never be the gate.

Commits on your branch can say whatever helps you. Get the title right.

## Development Setup

The short version:

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/whento.git
cd whento

make hooks           # once per clone: format staged files on commit
make dev-db          # Start PostgreSQL + Redis
make dev-fullstack   # Frontend on :8080, backend on :5173

make test
```

Two things that will otherwise bite you:

- **Never run `go build ./...` or `go test ./...`.** Every build is `cloud` or
  `selfhosted`, whole files are selected by build tag, and a bare invocation
  silently compiles neither variant honestly. Use the `make` targets, or pass
  `-tags selfhosted` / `-tags cloud`.
- **A bare checkout does not compile.** `web/dist/` (`//go:embed`) and
  `docs/swagger/` (imported by `cmd/main.go`) are generated and git-ignored.
  `make ensure-dist-placeholder` and `make swagger` produce them.

The long version — pinned tool versions, how to add a domain, a migration or a
locale, and the full pre-push checklist — is in
[docs/development.md](/docs/development.md). The design behind it is in
[docs/architecture.md](/docs/architecture.md) and the
[ADRs](/docs/adr/README.md).

## Release process

Releases are cut by hand by the maintainer. Anyone can read this; only someone
with push access to `main` can perform it.

### 1. Choose the version

Semantic versioning, as applied here:

| Bump | When |
| --- | --- |
| **major** (`2.0.0`) | A breaking change for a self-hosted operator: a removed or renamed environment variable, an API response that changes shape, a migration that cannot be rolled back, a required manual step during upgrade. |
| **minor** (`1.7.0`) | Any `feat:` — a new user-visible capability, a new endpoint, a new configuration option with a safe default. |
| **patch** (`1.6.4`) | `fix:`, `perf:`, `chore(deps):`, `docs:`, `ci:`, `refactor:`, `test:` — nothing an operator has to do anything about. |

A dependency bump that changes a base image major version (Postgres, Redis) is
**not** a patch: it needs a release note and, in the Postgres case, a documented
dump-and-restore. See
[docs/upgrade-postgres-redis.md](/docs/upgrade-postgres-redis.md).

### 2. Update the changelog

Move the `## [Unreleased]` entries into a new version section in
[CHANGELOG.md](/CHANGELOG.md), dated, and add the comparison link at the
bottom. The draft comes straight out of the history:

```bash
git log --no-merges --format='- %s' v1.6.3..HEAD
```

which is readable precisely because the titles are Conventional Commits.

### 3. Tag — and the tag **must be annotated**

```bash
git tag -a v1.6.4 -m "v1.6.4

Short summary of what is in this release.
"
git push origin v1.6.4
```

**`git tag -a`, never `git tag v1.6.4`.** This is not style. A lightweight tag is
just a ref pointing at a commit and carries no message of its own, so:

- `git tag -l -n999` prints the **commit** message instead. That is how the
  `v1.6.3` release body came to be prefixed with sixteen lines of "update
  dependencies" — the release notes described a dependency bump rather than the
  release.
- `git describe`, which stamps `main.Version` into locally built binaries, is
  less useful, and the tag carries no tagger, date or signature of its own.

`release-selfhosted.yml` now detects this: it checks the tag's object type with
`git cat-file -t` and, for anything that is not a real tag object, falls back to
the notes GitHub generates from the commit range. So a lightweight tag no longer
produces a *wrong* release body — but it produces one with no human summary,
which is a worse release either way.

For the record: **22 of the 31 tags in this repository's history are
lightweight**, and `v1.6.2` was tagged on a branch that is not an ancestor of
`main`, which is why `CHANGELOG.md` carries caveats about both.

### 4. Let the workflow do the rest

Pushing a `v*.*.*` tag triggers
[`release-selfhosted.yml`](workflows/release-selfhosted.yml), which:

- builds and scans the multi-arch image (Trivy, before push), publishes it to
  GHCR with an in-registry SBOM and a signed provenance attestation;
- cross-compiles binaries for linux/amd64, linux/arm64, windows/amd64,
  darwin/amd64 and darwin/arm64;
- generates an SPDX SBOM from the linux/amd64 binary, checksums everything, and
  attests the archives;
- creates the GitHub release with the annotated tag message on top of the
  generated notes.

Then check the release page: the body should open with your tag message, and
`checksums.txt` plus the `.spdx.json` should be attached.

### Why this is not automated

`release-please` and equivalents were considered and deliberately not adopted;
the reasoning is in
[docs/adr/0006-manual-releases.md](/docs/adr/0006-manual-releases.md). The
short version: this repository has one maintainer, a release cadence measured in
weeks, and the actual failure was tag hygiene — which the annotated-tag rule
above and the CI title check fix directly, without adding a bot with write access
to `main`. Revisit it when there is more than one person cutting releases.

## Questions?

- Open a [Discussion](https://github.com/When-To/whento/discussions)
- Check the [README](/README.md) for documentation

## License

By contributing, you agree that your contributions will be licensed under the BSL-1.1 license.
