# 0004 — Ship one binary with the frontend embedded

**Status:** Accepted
**Date:** recorded 2026-08 (the choice predates the records)

## Context

WhenTo's primary audience self-hosts. That means the person deploying it is
frequently the same person who will be woken up by it, is not a full-time
operator, and is running it on a NAS, a small VPS or a home server next to
whatever else they run.

The conventional split — an API container, a static-asset container, and nginx in
front to route between them — is three moving parts, a routing configuration that
can be wrong, and two artefacts whose versions can drift. Every one of those is a
support question, and every support question lands on a maintainer.

There is a second, sharper reason. The frontend is a single-page application, so
**every unmatched path must return `index.html`** for deep links to work, while
`/api/*` must not. Getting that wrong produces a 200 with a page of HTML where a
404 belonged — a failure that looks like success to any client checking the
status code and not the content type. When that rule lives in a reverse-proxy
config the operator writes, it is wrong on a meaningful fraction of installs.

## Decision

Compile the built frontend into the Go binary with `//go:embed`, and serve it from
the same process and the same port as the API.

[`web/embed.go`](../../web/embed.go):

```go
//go:embed dist/*
//go:embed dist/assets/*
var frontendFS embed.FS
```

`SPAHandler` is mounted last, on `/*`, by `registerFallbacks` in
[`cmd/router.go`](../../cmd/router.go). The fallback rules are code, not
configuration:

- `/api/*` and any otherwise-unmatched route answer **404 in the JSON envelope**;
- everything else serves `index.html`, with SEO meta tags injected per route (and
  the route-specific set only for the `cloud` variant).

The build chain is `make build`: Swagger generation → `npm run build:$(BUILD_TYPE)`
→ copy `frontend/dist` to `web/dist` → `go build`. Version, build date and VCS ref
are stamped in with `-ldflags` onto the symbols in `cmd/buildinfo.go`.

## Consequences

**Good**

- Deployment is one artefact: `docker run`, or download a binary and run it. The
  README's quick start is genuinely short.
- The frontend and the API it talks to are the same version, always. There is no
  configuration under which they can disagree.
- The SPA-fallback rule is written once, in Go, and covered by tests
  (`cmd/routes_test.go` walks the assembled route table without needing the
  embedded frontend at all).
- Native binaries are built for five platform/architecture pairs from the same
  source, and each carries a working web UI.
- Nothing is served from the filesystem at runtime, so there is no asset directory
  to mis-permission or path-traverse.

**Bad**

- **`web/dist/` must exist for the Go build to succeed.** `//go:embed` is resolved
  at compile time, so a bare checkout does not build. `make ensure-dist-placeholder`
  writes a stub, and every CI job that compiles Go either downloads the real
  frontend artifact or calls that target. This is a real and recurring papercut for
  newcomers.
- The Go build depends on a Node build, which makes the full build slower and the
  toolchain requirements wider than "Go".
- Assets are served by Go's `http.FileServer`, not by nginx. Fine at
  self-hosting scale; a CDN in front is the answer at any other scale.
- The binary carries the whole frontend, and changing one CSS line means shipping
  a new binary.
- `docs/swagger/` is imported by `cmd/main.go`, so the *generated* Swagger package
  must also exist before Go will compile — a second instance of the same
  "generated code is a build input" tax.

## Alternatives considered

- **Separate containers behind a reverse proxy.** The industry default, rejected
  on the operator-burden and version-drift grounds above. Note that operators who
  *want* a reverse proxy still get one; it just terminates TLS and forwards to a
  single upstream, which is a configuration that is hard to get wrong. See
  [reverse-proxy.md](../reverse-proxy.md).
- **Serving `dist/` from disk with a configurable path.** Keeps the single
  process but reintroduces a filesystem layout to get right, and makes the binary
  non-self-contained for no benefit at this scale.
- **Frontend on a static host / CDN, API separate.** Best at scale, worst for the
  self-hosting audience, and it makes CORS and cookie configuration the
  operator's problem.

## When to revisit

If the hosted variant's traffic makes Go-served assets the bottleneck, the answer
is a CDN in front rather than un-embedding. Reversing this would only be right if
self-hosting stopped being the primary audience.
