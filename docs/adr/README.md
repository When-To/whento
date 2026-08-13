# Architecture decision records

A handful of decisions in this codebase are load-bearing: code all over the
repository is shaped by them, they are expensive to reverse, and none of them is
obvious from reading a single file. Until now they lived in commit messages, in
`CLAUDE.md`, and in the heads of people who were there. These records are where
they live instead.

An ADR is not documentation of *how* something works — that is
[architecture.md](../architecture.md). It is the record of a decision: what forced
it, what was chosen, what that costs, and what would have to be true to revisit
it. Read them before proposing a change that contradicts one; a pull request that
does so will be asked for the argument these pages already answer.

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-dual-build-tags.md) | Two build variants selected by Go build tags | Accepted |
| [0002](0002-capability-based-participant-access.md) | Participant access is capability-based: the link *is* the authorisation | Accepted |
| [0003](0003-no-orm-hand-written-sql.md) | No ORM — hand-written SQL over pgx | Accepted |
| [0004](0004-single-binary-embedded-frontend.md) | Ship one binary with the frontend embedded | Accepted |
| [0005](0005-no-personal-data-in-telemetry.md) | No personal data in logs or metrics, enforced by an AST guard | Accepted |

## Writing a new one

Copy the shape of an existing record: **Context** (what forced the decision),
**Decision**, **Consequences** (both directions), **Alternatives considered**, and
**When to revisit**. Number it sequentially, do not renumber existing records, and
add a row above.

Superseding rather than editing is the rule: if a decision changes, write a new
record, set the old one's status to `Superseded by NNNN`, and leave its text
alone. The value of these files is that they say what was true at the time.
