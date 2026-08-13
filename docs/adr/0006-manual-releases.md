# 0006 — Keep releases manual; enforce the commit convention in CI instead

**Status:** Accepted
**Date:** 2026-08

## Context

Until this record, WhenTo had no `CHANGELOG.md` and no automated release
tooling. Versions were chosen by hand, tags were pushed by hand, and the release
body came from the tag message.

That produced a concrete failure. **22 of the repository's 31 tags are
lightweight** (`git cat-file -t` reports `commit`, not `tag`), including
`v1.6.3`. A lightweight tag carries no message of its own, so `git tag -l -n999`
falls back to printing the *commit* message — which is how the `v1.6.3` release
body came to open with sixteen lines of "update dependencies". A second symptom:
`v1.6.2` was tagged on a branch that is not an ancestor of `main`, so
`git log v1.6.2..v1.6.3` spans the divergence and returns most of the history
rather than one release's worth.

Meanwhile the Conventional Commits convention was **documented but never
checked**. `.github/CONTRIBUTING.md` described it; nothing verified it. The only
formatting gate in the repository was `.githooks/pre-commit`, which is opt-in per
clone (`make hooks`) and bypassed by `git commit --no-verify`.

The obvious remedy is a release automation tool — `release-please` or equivalent
— which derives the version, the changelog and the tag from the commit history,
removing the human step that keeps going wrong.

Facts about this repository that bear on whether that is proportionate:

- **One maintainer.** `git shortlog` shows essentially a single human identity
  (across three addresses that `mailmap.txt` canonicalises) plus a bot.
- **Releases are episodic**, not continuous: 31 tags over nine months, in bursts.
- **Zero merge commits in the entire history.** Pull requests are squash-merged,
  so the pull request *title* becomes the commit subject on `main`.
- The repository has just been through a hardening pass: every third-party action
  pinned to a full commit SHA, least-privilege `permissions` on every workflow,
  `timeout-minutes` on every job. `CODEOWNERS` flags the workflows as sensitive
  precisely because they run with repository credentials and can publish images.
- The version number appears in more than one place: the `@version` annotation in
  `cmd/main.go` (currently `1.6.3`, and cosmetic — `run()` overrides
  `SwaggerInfo.Version` from the linker at startup), and `frontend/package.json`
  (`0.1.0`, private, and unrelated). Binaries take their version from
  `git describe` through `-ldflags`.

## Decision

**Split the problem, and only automate the half that is failing.**

1. **Enforce the commit convention in CI, now.** A `Validate PR Title` job in
   `ci.yml` checks `pull_request.title` against the Conventional Commits grammar.
   Because merges are squashed, that title *is* the commit message on `main`, so
   checking it is equivalent to checking the history — and it is the only check
   that cannot be bypassed by a contributor's local configuration.

   It is a plain `run:` step with a shell regex, not a third-party action. For
   fifteen lines of `case`/`grep`, adding a pinned external action to a workflow
   file that `CODEOWNERS` marks sensitive is a worse trade than writing it out.

2. **Keep cutting releases by hand, with a written procedure.**
   [CONTRIBUTING → Release process](../../.github/CONTRIBUTING.md#release-process)
   states who cuts a tag, the version-bump criteria, and — the part that actually
   broke — that **the tag must be annotated**, with the reason.

3. **Maintain `CHANGELOG.md` by hand**, drafted from the history rather than
   remembered:

   ```bash
   git log --no-merges --format='- %s' v1.6.3..HEAD
   ```

   That command is only readable because of point 1. The convention being
   enforceable is what makes the manual step cheap.

4. **Defend the release body against the failure regardless.**
   `release-selfhosted.yml` already detects the tag object type with
   `git cat-file -t` and falls back to GitHub's generated notes for anything that
   is not a real tag object. A lightweight tag can no longer produce a *wrong*
   release body — only a summary-less one.

`release-please` is **proposed, not adopted**. See "When to revisit" for the
trigger.

## Consequences

**Good**

- The gap that actually caused the incident — an unenforced convention and a
  malformed tag — is closed, in one CI job and one paragraph of documentation.
- No bot holds `contents: write` on `main`. Nothing new appears in the workflow
  supply chain, which matters more here than in most repositories: these
  workflows publish images, and one of them must never publish to a public
  registry.
- The maintainer keeps deciding what a release *is*. Version bumps in this
  project are judgement calls that a commit-type mapping gets wrong — a
  `chore(deps)` that moves a Postgres major version is not a patch, however it is
  labelled, because it obliges every operator to dump and restore.
- The changelog can say things the commit log cannot: that `v1.6.2` is not an
  ancestor of `main`, that entries before `v1.3.5` are reconstructed.

**Bad**

- **The changelog can still be forgotten**, because nothing enforces it. This is
  the honest cost of the decision. The mitigation is a line in the pull request
  checklist and a step in the release procedure, both of which are conventions,
  not gates.
- The version number is still typed by a human into a tag, so it can still be
  wrong or skipped.
- The `@version` annotation in `cmd/main.go` will keep drifting. It is harmless
  today (overridden at runtime) but it is a stale string that automation would
  have kept correct.
- Release notes quality depends on someone writing a tag message.

## Alternatives considered

### `release-please` (googleapis/release-please-action)

The strongest candidate, and genuinely well matched on two points: the history is
already Conventional Commits, and squash-merging means the titles it parses are
the real commit subjects.

What it would give: a standing "chore: release x.y.z" pull request that
accumulates the changelog, and, on merge, a tag and a GitHub Release whose body it
writes from the parsed commits — which sidesteps the annotated-tag problem
entirely, since the tag message stops being the source of the release body.

Why not now:

- **Trust cost.** It needs `contents: write` and `pull-requests: write`, and it
  writes to `main`. In a repository that just pinned every action to a SHA
  because "a moving major tag is a moving target: whoever controls it controls
  what runs with that token", adding a bot with commit rights to save one
  maintainer a `git tag -a` per fortnight is not an obviously good trade.
- **Configuration is not free.** Keeping the `@version` annotation and any other
  version-bearing file in step needs `extra-files` rules; the two-module Go
  workspace and the private `frontend/package.json` both need deciding about.
  That is an afternoon of setup and an ongoing thing to maintain, against a
  manual step measured in minutes per release.
- **It automates the half that was working.** Nobody has complained that the
  version numbers were wrong. The tags were.
- **It removes judgement where judgement is wanted** — see the Postgres example
  above.
- Its output would be *worse* for the historical part of `CHANGELOG.md`, which
  needs the human caveats about `v1.0.0` and `v1.6.2`.

### `commitlint` via husky or a CI step

The named alternative for point 1. Rejected in favour of the shell regex because
it means adding `@commitlint/cli` and `@commitlint/config-conventional` to a
`package.json` — and the only `package.json` in scope is the frontend's, where
release tooling has no business being. A root-level Node project purely to lint a
string would be more machinery than the string.

The husky variant is worse still: it is a git hook, and this repository already
knows that hooks are opt-in per clone.

### `amannn/action-semantic-pull-request`

The standard action for exactly point 1, and a reasonable choice. Rejected on the
same supply-chain reasoning: it does what fifteen lines of shell do, in a file
`CODEOWNERS` marks sensitive.

### `git-cliff` / `conventional-changelog` for the changelog only

Generates the changelog without taking write access or owning the version. A
sensible middle option, and the one to reach for first if the manual changelog
starts being forgotten. Not adopted now because a generated changelog is a list
of commit subjects, and the file is more useful when it is written for a reader —
compare the `## [Unreleased]` section, which groups thirty-three commits into six
statements.

### Do nothing

Rejected. The convention being documented and unchecked is what let the situation
develop.

## When to revisit

Adopt `release-please` when **either** of these becomes true:

- **more than one person cuts releases** — at which point the procedure stops
  being one person's habit and starts being coordination; or
- **the changelog is out of date at release time twice in a row** — which is the
  measurable form of "the manual step is not being done".

The CI title check adopted here is the precondition for that migration, not an
alternative to it: `release-please` is only as good as the commit subjects it
parses. Adopting it later will be a small step *because* of this decision, not in
spite of it.
