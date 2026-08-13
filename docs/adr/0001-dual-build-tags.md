# 0001 — Two build variants selected by Go build tags

**Status:** Accepted
**Date:** recorded 2026-08 (the split predates the records; it is visible from the
first release)

## Context

WhenTo is distributed two ways from one repository:

- **self-hosted** — what the public releases and the GHCR image are, unrestricted;
- **cloud** — the hosted service, which caps how many calendars an account may
  own and serves richer SEO metadata for its public marketing pages.

The differences are small and unlikely to grow: today they are the calendar
allowance ([`internal/quota`](../../internal/quota/)) and the SEO behaviour
([`internal/seo`](../../internal/seo/), [`web/embed.go`](../../web/embed.go)).
Historically there were more — the cloud variant carried Stripe subscriptions and
the self-hosted one carried Ed25519 licence verification. Both have since been
removed, which is exactly why the mechanism needs a written justification: it now
looks disproportionate to what it currently separates.

The constraint that has not gone away is that **the cloud image must never be
published to a public registry**. Whatever separates the variants has to make the
wrong artefact hard to build by accident, not merely discouraged.

## Decision

Select the variant with a Go build tag, `cloud` or `selfhosted`, chosen at
compile time. `BUILD_TYPE ?= selfhosted` in the Makefile; the frontend equivalent
is `VITE_BUILD_TYPE`, read through `isCloud` / `isSelfHosted` in
`frontend/src/composables/useBuildType.ts`.

Three rules make it workable:

1. **No build-tag-exclusive packages.** The difference is expressed as symmetric
   *file pairs* inside otherwise shared packages, each pair exposing the same
   symbols:
   - `cmd/init_cloud.go` / `cmd/init_selfhosted.go` — `InitServices`,
     `RegisterQuotaRoutes`, `buildType`;
   - `internal/quota/cloud.go` / `internal/quota/selfhosted.go`.
2. **Both variants must compile, and CI proves it** — `go build`, `golangci-lint`
   and `govulncheck` all run twice, under each tag.
3. **A bare `go build ./...` is wrong** and is treated as such in every document
   and every Makefile target. It silently excludes whole files, so it neither
   builds nor tests the code that ships.

## Consequences

**Good**

- The variant is decided at link time. A self-hosted binary does not contain the
  cloud code paths at all — not disabled, absent — so a runtime configuration
  mistake cannot turn one into the other.
- The symmetric-pair rule keeps the divergence visible and small: adding a
  cloud-only feature without its self-hosted counterpart does not compile.
- The Makefile can refuse to do the dangerous thing. `make docker-build
  BUILD_TYPE=cloud` errors out rather than silently producing a self-hosted image
  labelled "cloud", which is what it used to do.

**Bad**

- Every tool that walks packages needs the tag passed explicitly, and forgetting
  it produces a *green* result over the wrong code — the worst failure mode
  available. This is why `make lint-go` runs four times (two modules × two tags)
  and why `.golangci.yml` deliberately sets no `run.build-tags`.
- Editor tooling shows one variant's files as excluded, which reads as an error.
- CI cost roughly doubles for the build, lint and vulnerability-scan jobs.
- Test coverage is measured under `selfhosted` only; the cloud-only files are
  compiled and linted but their branches are not exercised.

## Alternatives considered

- **A runtime flag** (`MODE=cloud`). Rejected: it ships the cloud code inside the
  public binary and makes "never publish the cloud image" a policy about
  configuration rather than a property of the artefact.
- **Two repositories, or a fork.** Rejected: the shared surface is ~99% of the
  code, and the divergence would be paid on every single change.
- **An interface with two implementations, both compiled in, selected by
  config.** This is what the code effectively does *within* a variant
  (`quota.QuotaService`); making it runtime-selected only moves the problem back
  to the previous alternative.

## When to revisit

If the remaining divergence shrinks to just the calendar allowance and a boolean
for SEO, both would be expressible as configuration — with the caveat above about
what that means for the cloud artefact. Conversely, if the two variants start
diverging in ways that need whole packages rather than file pairs, rule 1 has
failed and the decision should be re-argued rather than quietly bent.
