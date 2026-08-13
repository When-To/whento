# Security Policy

## Supported Versions

WhenTo is released as `vMAJOR.MINOR.PATCH` tags, each publishing a self-hosted
container image and native binaries. Only one line is maintained at a time.

| Version                | Supported                | What that means                                                                 |
| ---------------------- | ------------------------ | -------------------------------------------------------------------------------- |
| **1.6.x** (current)    | ✅ Full support           | Bug fixes and security fixes, released as new patch versions.                     |
| **1.5.x**              | ⚠️ Security fixes only    | Until 90 days after the release of 1.6.0, or until 1.7.0 ships, whichever is later. |
| **≤ 1.4.x**            | ❌ Unsupported            | No fixes. Upgrade.                                                                |

In practice: **run the latest release.** Fixes land on the current minor; older
minors receive a backport only for vulnerabilities rated High or Critical, and
only inside the window above.

The `latest` container tag always points at the newest release. It is convenient
and it is not a support statement — pin a version tag or a digest in production
and upgrade deliberately (see [docs/hardening-docker.md](../docs/hardening-docker.md)).

There is no LTS line. If you need one, that is a commercial conversation:
see [`LICENSE-COMMERCIAL`](../LICENSE-COMMERCIAL).

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues,
discussions, or pull requests.**

Two private channels, in order of preference:

1. **GitHub private vulnerability reporting** — on
   <https://github.com/When-To/whento>, open the **Security** tab and choose
   *Report a vulnerability*. This creates a private advisory visible only to the
   maintainers, keeps the whole exchange in one place, and lets us credit you and
   request a CVE from the same thread.
2. **Email** — <security@whento.be>.

If you need to send the report encrypted, ask for the current key at that address
first, with no vulnerability details in the initial message; no long-lived PGP key
is published for this project, so a key fingerprint you found elsewhere is not
ours.

### What to include

- What the issue is, and which component (`pkg/jwt`, participant links, rate
  limiting, an endpoint, the container image…).
- Steps to reproduce, ideally against a local `docker compose up` stack.
- The version or commit you tested. The health endpoint deliberately does not
  reveal it; the binary logs `WhenTo build info` with the version, build type,
  build date and commit once at startup (`docker compose logs app | head`).
- Impact as you see it: what an attacker gets, and what they need to start with.
- A suggested fix, if you have one. Optional and always welcome.

### What happens next

| Stage                | Target                                                                                    |
| -------------------- | ------------------------------------------------------------------------------------------ |
| Acknowledgement      | **48 hours** (business days; longer over holidays — this is a small project, not a vendor SOC). |
| Triage and severity  | 5 business days. You get our assessment, including if we disagree that it is a vulnerability. |
| Fix — Critical       | Patch release targeted within **7 days** of confirmation.                                  |
| Fix — High           | Within **30 days**.                                                                        |
| Fix — Medium / Low   | Within **90 days**, or in the next scheduled release.                                      |
| Public disclosure    | Once a fix is released, or **90 days** after your report, whichever comes first.            |

We use CVSS v3.1 as a starting point for severity and will tell you the vector we
used. If we cannot meet a target we will say so before it passes, with a reason
and a new date.

We support coordinated disclosure and will not ask you to stay silent
indefinitely. If a report is still unfixed at 90 days, publishing is your call
and we will not treat it as hostile. If a fix needs longer for a defensible
reason, we will ask — not insist.

You will be credited in the release notes and in the GitHub advisory unless you
prefer to remain anonymous. There is no paid bug bounty.

### Scope

**In scope:** the application in this repository, the published self-hosted
container images and binaries, the release and build workflows, and the default
configuration shipped in `docker-compose.yml` and `.env.example`.

**Out of scope:** the hosted service at whento.be (report those to the same
address, but note they are a different deployment); vulnerabilities that require
an already-compromised host or an attacker who already holds a valid calendar
link; findings from an automated scanner with no demonstrated impact; missing
hardening headers on a deployment you configured yourself; and the deliberate
design decision below.

### Not a vulnerability: participant links are the credential

Participant access is **capability-based by design**. Possession of a calendar's
public token plus a participant UUID *is* the authorisation — there is no
per-participant secret to verify, so anyone holding the link can act as that
participant. That is the product working as intended, and it is documented in the
[README](../README.md#one-thing-to-understand-about-participant-links).

What *is* a vulnerability: any way to obtain a token you were not given — by
enumeration, by a timing side channel, through a log or an error message, through
a referrer header, or through an endpoint that returns another calendar's token.

## Security Best Practices for Self-Hosted Deployments

1. **Keep WhenTo updated.** Run the current minor; watch releases on GitHub.
2. **Terminate TLS at a reverse proxy** — and set `TRUSTED_PROXIES` to match, or
   every per-IP rate limit collapses into a single shared bucket.
   [docs/reverse-proxy.md](../docs/reverse-proxy.md) has working Caddy and nginx
   configurations and explains exactly what to set.
3. **Harden the container stack.** `no-new-privileges`, dropped capabilities and
   resource limits ship in `docker-compose.yml`;
   [docs/hardening-docker.md](../docs/hardening-docker.md) explains what is
   applied, what is not, and how to verify it.
4. **Secure the database.** Strong `DB_PASSWORD`, no published port (the shipped
   compose file publishes none), and TLS if Postgres is not on the same host.
5. **Protect the JWT keys.** `whento_keys` holds the RSA private key; back it up
   with the database and keep it mode `0600`. Deleting the volume logs every user
   out — which is also how you rotate the keys deliberately.
6. **Enable rate limiting** (`RATE_LIMIT_ENABLED=true`, the default). Running
   several instances against one Redis additionally requires the same
   `RATE_LIMIT_KEY_SALT` everywhere, or each instance keeps its own buckets and
   N instances grant N times the allowance.
7. **Restrict registration** with `ALLOWED_REGISTER` and `ALLOWED_EMAILS` if the
   instance is not meant to be open.
8. **Back it up, and restore it.** [docs/backup-restore.md](../docs/backup-restore.md)
   — including the drill. An untested backup is not a backup.
9. **Know what your logs contain** before you ship them anywhere:
   [docs/logging-and-privacy.md](../docs/logging-and-privacy.md).

## What WhenTo logs, and what it does not

Stated here because it is a security property, not only a privacy one: log
files travel, and in WhenTo the URL path *is* a credential.

- The HTTP access log records the chi **route pattern**
  (`/calendar/{token}/participant/{pid}`) and never the request path, so calendar
  tokens, participant UUIDs and magic-link secrets do not reach the logs.
- **No IP address and no User-Agent are logged**, anywhere in the request path.
- Rate-limit buckets are stored in Redis under a **salted HMAC-SHA256** digest, so
  Redis holds no IP address, calendar token or user id for rate limiting.
- Known exceptions — recipient email addresses in mail-sending logs, and the
  calendar token in the server-sent-events handler — are listed explicitly in
  [docs/logging-and-privacy.md](../docs/logging-and-privacy.md#5-known-gaps),
  along with what Redis holds and for how long. A privacy claim with an unstated
  exception is worse than no claim.

## Security-sensitive code

Changes to these areas get extra scrutiny in review, and changing a token format,
a key derivation or a signature scheme is a compatibility event that must be
called out in the release notes:

- [`pkg/jwt`](../pkg/jwt) — access and refresh tokens
- [`pkg/middleware`](../pkg/middleware) — rate limiting, trusted proxies, security headers, CORS
- [`internal/mfa`](../internal/mfa), [`internal/passkey`](../internal/passkey) — TOTP and WebAuthn
- [`internal/auth`](../internal/auth) — sessions, magic links, password reset

Automated coverage: CodeQL, `govulncheck` (both build variants, both modules) and
dependency review run in [`.github/workflows/security.yml`](workflows/security.yml);
container images are scanned in the release workflow, just before they are
pushed. Dependency updates arrive through Dependabot;
GitHub's Dependabot **security** updates are a repository setting and must be
enabled there separately.
