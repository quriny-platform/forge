# 0002. Security boundaries and non-goals

Date: 2026-09-06
Status: Accepted

## Context

forge is a library on the trust boundary of every application that imports it.
It holds credentials, negotiates TLS, constructs SQL, and parses bytes from a
server it cannot vouch for. A defect here is a defect in every consumer
simultaneously, and consumers cannot compensate for a boundary they do not know
exists.

Without a written boundary, two failure modes follow. Users assume protections
forge does not provide — "the driver escapes my inputs, so concatenation is
fine" — and contributors add convenience APIs that quietly widen the attack
surface, because nothing says which side of the line they are on.

OWASP's Top 10 2021, ASVS, and Proactive Controls are the vocabulary the wider
industry uses for this. Adopting them costs nothing and makes forge's posture
legible to a security reviewer who has never seen the code.

## Decision

forge names four trust boundaries and defends each explicitly:

- **TB1 — application → forge API.** SQL text arrives from developers;
  values and identifiers frequently originate with end users. Defence:
  parameterised queries as the only value path, a single identifier-quoting
  choke point, and an API in which values cannot syntactically reach the SQL
  half of `(sql, args)`.
- **TB2 — forge → network → server.** Every byte received is untrusted.
  Defence: length validation before allocation, hard caps on message and
  collection sizes, fuzzed decoders, no panics in library code.
- **TB3 — forge → logs, traces and errors.** Egress, not ingress. Defence:
  redaction at the type level, so a careless `%v` cannot leak a secret.
- **TB4 — forge → dependencies and CI.** Defence: a stdlib-only runtime
  dependency tree, SHA-pinned actions, `govulncheck` in CI, signed release
  checksums, published SBOM.

forge explicitly does **not**: authorise queries, sanitise application-authored
SQL strings, encrypt data at rest, manage keys, or rate-limit callers.

Every milestone carries a `## Security (OWASP)` section whose controls are part
of its definition of done. The full mapping lives in
`docs/SECURITY-PRACTICES.md`.

## Consequences

- Any new exported API must be classified against TB1–TB4 in its PR, and a
  convenience helper that interpolates values into SQL cannot be merged
  regardless of how ergonomic it is.
- A stdlib-only runtime dependency tree becomes a security commitment, not a
  preference. Adding a runtime dependency requires its own ADR.
- Redaction lives in the type system (`Password` with a masking `String()`,
  message types rendering `[redacted]`) rather than at call sites, which costs
  some ergonomics and removes a whole class of leak.
- The non-goals must appear in the README. Under-claiming is safe;
  over-claiming stops users from adding the controls forge lacks.
- Security work is spread across every milestone rather than deferred to a
  hardening pass, which is slower per milestone and far cheaper overall.
