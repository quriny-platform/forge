# 0001. Record architecture decisions

Date: 2026-09-06
Status: Accepted

## Context

`forge` is written as a from-scratch PostgreSQL driver while `pgx` already
exists and is the de-facto reference. Many design points will look arbitrary in
six months unless the reasoning survives — especially where forge deliberately
does *not* follow pgx (layer boundaries, error surface, pool semantics, codec
registry ownership).

Writing the reasoning down is a large part of the value: a decision that cannot
be explained in a page has not been made, only defaulted into.

## Decision

Use Architecture Decision Records as described by Michael Nygard, stored in
`docs/adr/` in the repo, one Markdown file per decision, numbered sequentially. An ADR is
required whenever forge diverges from pgx or from `database/sql/driver`
conventions. ADRs are immutable after merge; a changed mind produces a new ADR
that supersedes the old one.

## Consequences

- Every milestone PR that introduces a divergence must include its ADR; reviewers
  reject divergences that arrive without one.
- The ADR index in `docs/adr/README.md` must be updated in the same commit.
- Newcomers get an ordered narrative of *why* the code looks like it does,
  something the code itself cannot carry.
