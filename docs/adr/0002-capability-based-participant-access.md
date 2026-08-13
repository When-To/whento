# 0002 — Participant access is capability-based: the link *is* the authorisation

**Status:** Accepted
**Date:** recorded 2026-08 (the model predates the records)

## Context

A WhenTo calendar is answered by people who are, in the normal case, **not users
of the product**. They were sent a link by a friend, a band, a games group or a
sports club. They did not sign up, did not agree to anything, and are frequently
not even asked for a name — anonymous participation is a supported feature.

Requiring an account before someone can say "Tuesday works" would defeat the
purpose of the tool. The comparison points (Doodle, Framadate) do not require one
either, and a scheduling tool that does is a scheduling tool nobody's friends
will use.

Something still has to decide whether a given request may write a given
participant's availability.

## Decision

**Possession of the calendar's public token, plus the participant's UUID, is the
authorisation.** There is no per-participant secret, no participant session, and
no token to verify beyond the two identifiers already in the URL.

Concretely, in [`cmd/routes_availability.go`](../../cmd/routes_availability.go):

```
/api/v1/availabilities/calendar/{token}/participant/{pid}
```

carries no `middleware.Auth`. Every route in that file is public. The same
applies to the public half of
[`cmd/routes_calendar.go`](../../cmd/routes_calendar.go) —
`GET /api/v1/calendars/public/{token}` and the anonymous-registration and
participant-email routes.

The consequence is stated plainly rather than hedged: **anyone holding the link
can act as any participant of that calendar, by design.** The link is a
capability, and capabilities are bearer credentials.

What backs it:

- The public token is generated from `crypto/rand`
  ([`internal/calendar/service/calendar_service.go`](../../internal/calendar/service/calendar_service.go)),
  so it is not guessable.
- The ICS feed has a **separate** token, so handing someone a subscription URL
  does not hand them write access.
- The owner can revoke by rotating: `POST /api/v1/calendars/{id}/regenerate-token`
  invalidates every link previously shared.
- Every public route is rate-limited per IP, and the routes that spend a resource
  on a third party — sending mail to an address the caller picks — are limited far
  harder (5 or 3 per 15 minutes) than the read routes (60/minute).

Because the token is the credential, **the request path is a secret**. That is
not a side note; it propagates into the logging and metrics rules — see
[ADR-0005](0005-no-personal-data-in-telemetry.md) — where the route *pattern* is
recorded and the path never is.

## Consequences

**Good**

- A participant needs nothing: no account, no email, no password, no cookie. The
  product works for the people it is for.
- There is no participant credential to store, hash, rotate, leak or reset, and
  no participant session to fix or hijack.
- The authorisation check is trivial and therefore hard to get subtly wrong.
- It composes with anonymous participation, which has no identity to attach a
  secret to in the first place.

**Bad**

- **Anyone with the link can impersonate any participant on that calendar.**
  Editing someone else's availability, or deleting it, is within the model.
- The link leaks through the usual channels: browser history, chat previews,
  screenshots, referrer headers, and anything that logs URLs.
- Revocation is all-or-nothing. Rotating the token invalidates the link for
  everybody, and there is no way to remove one participant's access.
- No audit trail is meaningful: the system cannot distinguish "the participant
  wrote this" from "somebody with the link wrote it as them".
- The blast radius is bounded by what a calendar holds — names people chose to
  type, availability, and optionally an email address — but that is real personal
  data about people who never signed up. See
  [logging-and-privacy.md §8](../logging-and-privacy.md).

**Operationally this must be said out loud, not discovered.** The README and the
UI should treat a calendar link the way they would treat a shared password,
because that is what it is.

## Alternatives considered

- **Accounts for participants.** Rejected on product grounds, above.
- **A per-participant secret in the link** (`…/participant/{pid}?k={secret}`).
  This would stop cross-participant impersonation within a calendar and allow
  per-participant revocation. It is the most credible alternative and was not
  taken because it doubles the number of secrets to distribute — the organiser
  now has to send each person their own URL, rather than one link to a group
  chat, which is how these calendars are actually shared. Worth revisiting if
  per-participant revocation is ever asked for.
- **Signed, expiring participant tokens.** A `pkg/participanttoken` package once
  existed for something like this and was removed as unused
  (`chore: drop the unused participant token`). Expiry is the wrong shape for a
  calendar that is meant to persist across a season.
- **Email-verified participants.** Partially present already — a participant may
  add and verify an address — but it is opt-in and cannot be the authorisation
  without excluding anonymous participants.

## When to revisit

If someone asks to remove one participant's access without disturbing the others,
or if calendars start holding data whose exposure would matter more than
scheduling data does, the per-participant secret above is the design to reach
for. Any such change must keep anonymous participation working, or it is a
different product.
