# Security Policy

forge sits on the trust boundary of every application that imports it. It holds
credentials, negotiates TLS, constructs SQL, and parses bytes from a server it
cannot vouch for. Security reports are treated accordingly.

## Reporting a vulnerability

Report privately through GitHub's
[private vulnerability reporting](https://github.com/quriny-platform/forge/security/advisories/new).

**Do not open a public issue for a security defect.**

If GitHub advisories are unavailable, email muhammadrohadi03@gmail.com with
`[forge security]` in the subject.

Include: affected version or commit, a minimal reproducer, the impact you
believe it has, and whether it is already public.

### What to expect

| Stage | Target |
|---|---|
| Acknowledgement | 7 days |
| Initial assessment | 14 days |
| Fix or mitigation plan | agreed case by case |

forge is maintained part-time. These are honest targets, not an SLA. A fix
ships before details are published; credit is given unless you ask otherwise.

## Supported versions

| Version | Supported |
|---|---|
| `main` (pre-release) | ✅ |

forge has not reached v0.1.0. Until then only `main` receives fixes. A
supported-version table covering tagged releases arrives with the first release.

## Scope

In scope — defects in forge's own defences:

- SQL injection reachable through forge's parameterised-query or identifier
  quoting APIs
- Credential, DSN or query-argument values reaching logs, traces or error
  strings
- Memory-safety or denial-of-service defects in the wire protocol decoder or
  binary codecs when fed hostile server bytes
- TLS verification that can be silently weakened or bypassed
- Authentication defects: SCRAM implementation, timing-sensitive comparisons
- Supply-chain defects: release artefacts, CI workflow permissions, dependency
  integrity

Out of scope — documented non-goals. forge does not authorise queries, sanitise
application-authored SQL strings, encrypt data at rest, manage keys, or
rate-limit callers. SQL assembled by string concatenation in application code is
the application's defect, not forge's.

The four trust boundaries and the full reasoning are in
[ADR 0002](docs/adr/0002-security-boundaries.md); the threat model, OWASP Top 10
mapping and ASVS control table are in
[docs/SECURITY-PRACTICES.md](docs/SECURITY-PRACTICES.md).

## Security practices

- Parameterised queries are the only value path; identifiers pass through a
  single quoting choke point.
- Every byte received from a server is untrusted: lengths validated before
  allocation, hard caps on message and collection sizes, fuzzed decoders, no
  panics in library code.
- Redaction lives in the type system, so a careless `%v` cannot leak a secret.
- Stdlib-only runtime dependencies. Adding a runtime dependency requires its own
  ADR.
- CI pins every GitHub Action by commit SHA, runs `gosec` and `govulncheck`, and
  grants `contents: read` by default.
