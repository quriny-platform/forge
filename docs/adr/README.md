# Architecture Decision Records

One file per decision, numbered, immutable once merged. Supersede rather than
edit. Required for **every divergence from pgx's reference design**.

Naming: `NNNN-short-kebab-title.md`, e.g. `0002-single-registry-per-conn.md`.

## Template

```markdown
# NNNN. <Title>

Date: YYYY-MM-DD
Status: Proposed | Accepted | Superseded by [NNNN](NNNN-....md)

## Context
What forces are at play? What does pgx (or database/sql) do here, and why is
that not the right fit for forge?

## Decision
What we do, stated in the active voice.

## Consequences
What becomes easier. What becomes harder. What we must remember later.
```

## Index

- [0001 — Record architecture decisions](0001-record-architecture-decisions.md) — Accepted
- [0002 — Security boundaries and non-goals](0002-security-boundaries.md) — Accepted
