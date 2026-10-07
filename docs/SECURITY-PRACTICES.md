# forge — Security Practices

Security spine for the [roadmap](./ROADMAP.md). Every milestone carries a
`## Security (OWASP)` section; this file holds the threat model those sections
refer back to, and the checklist that gates a release.

Standards used: [OWASP Top 10 2021](https://owasp.org/Top10/),
[OWASP ASVS](https://owasp.org/www-project-application-security-verification-standard/),
[OWASP Proactive Controls](https://owasp.org/www-project-proactive-controls/),
[OWASP Go Secure Coding Practices](https://owasp.org/www-project-go-secure-coding-practices-guide/).

---

## 1. What forge is, in security terms

forge sits between application code and a PostgreSQL server. It handles
credentials, negotiates TLS, builds SQL, and moves user data across a network.
It is a **library on the trust boundary of every application that imports it** —
a defect here is a defect in every consumer at once.

### Trust boundaries

| # | Boundary | Untrusted input | Primary risk |
|---|---|---|---|
| TB1 | Application → forge API | SQL strings (from developers), values and identifiers (often from end users) | Injection (A03) |
| TB2 | forge → network → server | Every byte the server sends | Memory safety, DoS, MITM (A02, A08) |
| TB3 | forge → logs / traces / errors | — (egress, not ingress) | Credential & PII disclosure (A09) |
| TB4 | forge → dependencies & CI | Modules, actions, release artefacts | Supply chain (A06, A08) |

### Explicit non-goals

Write these in the README so nobody assumes otherwise:

- forge does **not** authorise queries. Row-level and role-level access control
  belong to PostgreSQL and to the application.
- forge does **not** sanitise application-authored SQL. A caller who
  concatenates user input into a query string has already lost; forge's job is
  to make the safe path the easy one.
- forge does **not** encrypt data at rest, manage keys, or mask columns.
- forge does **not** rate-limit. DoS protection at the application layer is out
  of scope; internal allocation limits (TB2) are in scope.

### Abuse cases to keep in mind

Written as [abuse cases](https://cheatsheetseries.owasp.org/cheatsheets/Abuse_Case_Cheat_Sheet.html) —
each one becomes a test somewhere in the roadmap:

1. Attacker controls a table or column name reaching `qb` → tries to break out of
   the identifier quoting. *(M13)*
2. Attacker sits on the network path → downgrades or MITMs the connection.
   *(M05)*
3. Malicious or compromised server → sends a message claiming a 4 GB length, or
   an array header claiming 2³¹ elements. *(M02, M08)*
4. Attacker reads application logs → looks for passwords, DSNs, or query
   argument values. *(M03, M04, M10)*
5. Attacker floods the application → exhausts the connection pool and stalls
   every request. *(M11)*
6. Attacker publishes a typosquatted module or compromises a GitHub Action.
   *(M00, M17)*

---

## 2. OWASP Top 10 2021 → forge

| Risk | How it reaches a driver | Controls | Milestones |
|---|---|---|---|
| **A01 Broken Access Control** | Dynamic `SET ROLE` / `SET search_path` built by string concatenation; transaction isolation not what the caller asked for | Session-setting API takes quoted identifiers only; `TxOptions` rendered from typed constants, never free text | M07, M10 |
| **A02 Cryptographic Failures** | Plaintext connections, weak TLS, MD5 auth, non-constant-time MAC compare | TLS 1.2 minimum; `verify-full` documented as the recommendation; SCRAM-SHA-256; `crypto/subtle` for every secret comparison; `crypto/rand` for nonces | M04, M05 |
| **A03 Injection** | Identifiers interpolated into SQL; placeholder/arg desynchronisation; `IN ()` expansion | Parameterised queries only; `QuoteIdentifier` as the single quoting choke point, fuzzed; builders never accept raw fragments except through a name containing `Raw`/`Unsafe` | M06, M07, M13, M15 |
| **A04 Insecure Design** | An API that makes the unsafe thing convenient | `ToSQL() (string, []any, error)` — values cannot syntactically end up in the SQL half; no `Sprintf`-style query helper anywhere in the public API | M01, M13 |
| **A05 Security Misconfiguration** | `sslmode=disable` by accident; `InsecureSkipVerify` left on; config precedence surprises | Secure defaults; `InsecureSkipVerify` never user-settable, only implied by a named sslmode; documented precedence with tests | M03, M05 |
| **A06 Vulnerable & Outdated Components** | Transitive dependencies in a library everyone imports | Stdlib-only runtime dependencies as a hard rule; Dependabot; `govulncheck` in CI; SBOM at release | M00, M17 |
| **A07 Identification & Authentication Failures** | Silent fallback to a weaker auth method; credentials sent in the clear | Explicit auth method policy; cleartext refused without TLS or a Unix socket; unknown methods error rather than degrade | M04 |
| **A08 Software & Data Integrity Failures** | Attacker-controlled length fields driving allocation; unverified release artefacts | Every length validated before allocation, with a hard cap; server signature verified in SCRAM; signed checksums on releases | M02, M04, M08, M17 |
| **A09 Security Logging & Monitoring Failures** | Query args, DSNs, or passwords in logs and error strings | `internal/sanitize` redacts before formatting; `Config.String()` masks the password; `PasswordMessage.String()` renders `[redacted]`; a leak test per milestone that touches secrets | M03, M04, M10 |
| **A10 SSRF** | Connection targets influenced by server-supplied data (multi-host, redirects) | Hosts and fallbacks come only from caller-supplied config; nothing the server sends can change where forge connects | M03, M05 |

**Plus, not in the Top 10 but first-class here — resource exhaustion.**
Unbounded allocation from an attacker-controlled length is the most likely
memory-safety bug in this codebase. See
[Denial of Service cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html).
*(M02, M08, M11, M15)*

---

## 3. ASVS controls forge commits to

Subset of [ASVS](https://owasp.org/www-project-application-security-verification-standard/)
relevant to a driver. Each row names where it is verified.

| ASVS area | Requirement as it applies to forge | Verified in |
|---|---|---|
| V1 Architecture | Threat model documented; trust boundaries named | this file, ADR 0002 |
| V2 Authentication | Credentials never logged; strong auth (SCRAM) preferred; weak methods gated | M04 |
| V5 Validation & Encoding | All dynamic SQL parameterised; identifiers escaped at one choke point | M07, M13 |
| V6 Cryptography | Approved primitives only; `crypto/rand` for all nonces; constant-time comparison of secrets; no home-grown crypto | M04 |
| V7 Error Handling & Logging | Errors carry no secrets; logs redact argument values; failures are not silently swallowed | M01, M10 |
| V8 Data Protection | Secret material zeroed after use; not retained on long-lived structs | M04 |
| V9 Communications | TLS 1.2+, certificate validation available and recommended, downgrade refused when required | M05 |
| V12 Files & Resources | Cert/key file reads validated; no path built from server data | M05 |
| V14 Configuration | Secure defaults; dependency inventory minimal and scanned | M00, M05, M17 |

---

## 4. Coding rules (non-negotiable)

Enforced by review, and where possible by a linter or a test.

1. **No `fmt.Sprintf` into SQL.** Ever. `golangci-lint` `gosec` G201/G202 stay on.
2. **Values are placeholders, identifiers are quoted.** Nothing else is
   interpolated. An escape hatch must have `Raw` or `Unsafe` in its name.
3. **Secrets never take a formatting verb.** Not `%v`, not `%s`, not in an
   error, not in a `String()` method. Redact at the type level so it cannot be
   forgotten at the call site.
4. **Validate every length before you allocate.** A length field from the
   network is attacker input. Cap it, then allocate.
5. **`crypto/subtle.ConstantTimeCompare` for every secret or MAC comparison.**
   `bytes.Equal` on a signature is a timing oracle.
6. **`crypto/rand` only.** `math/rand` never touches a nonce, salt, or ID.
7. **Fail closed.** An unknown auth method, an unknown sslmode, an unparseable
   config → error. Never a silent fallback to a weaker mode.
8. **No new runtime dependencies** without an ADR. Stdlib-only is a security
   property, not an aesthetic one.
9. **Zero secret material after use** (`clear(saltedPassword)`), and keep it out
   of long-lived structs.
10. **Every security fix ships with the test that would have caught it**, named
    after the symptom.

---

## 5. Security gates in the Definition of Done

Added to every milestone:

- [ ] Threats introduced by this milestone are named in its `## Security (OWASP)` section.
- [ ] At least one *negative* test exists — something an attacker would try, asserted to fail safely.
- [ ] `grep -nE '%[vsq].*(assword|ecret|oken|DSN|args)' <diff>` returns nothing.
- [ ] `golangci-lint run` with `gosec` enabled is clean, or each `#nosec` carries a one-line justification.
- [ ] Any new parsing of untrusted bytes has a fuzz target.
- [ ] `govulncheck ./...` clean.

---

## 6. Per-milestone security index

| Milestone | Primary risk | Security test to write |
|---|---|---|
| [M00](roadmap/M00-repo-foundation.md) | A06 supply chain | CI runs `govulncheck`; Dependabot enabled; actions pinned by SHA |
| [M01](roadmap/M01-core-interfaces.md) | A04, A09 | `QueryError.Error()` never contains an argument value |
| [M02](roadmap/M02-pgwire-protocol.md) | A08, DoS | `FuzzBackendDecode`; oversized-length message rejected before allocation |
| [M03](roadmap/M03-pgconn-simple-query.md) | A09, A10 | `Config.String()` masks password; connection targets come only from config |
| [M04](roadmap/M04-auth-scram.md) | A02, A07 | RFC vectors; tampered server signature rejected; no password in any output |
| [M05](roadmap/M05-tls-sslmode.md) | A02, A05 | `verify-full` rejects a wrong hostname; `require` refuses a server that declines TLS |
| [M06](roadmap/M06-pgtype-text.md) | A03 | NULL never renders as the literal `NULL` string into SQL |
| [M07](roadmap/M07-extended-query.md) | A01, A03 | Args always travel as `Bind` parameters; `TxOptions` rendering is closed-set |
| [M08](roadmap/M08-pgtype-binary.md) | A08, DoS | Array header claiming 2³¹ elements errors without allocating |
| [M09](roadmap/M09-cancel-context.md) | DoS | Cancellation cannot be used to leak goroutines or wedge a connection |
| [M10](roadmap/M10-postgres-driver-adapter.md) | A09 | Tracer output for a query with a secret argument contains no secret |
| [M11](roadmap/M11-pool.md) | DoS | Pool never exceeds `MaxConns`; exhaustion returns an error, not a hang |
| [M12](roadmap/M12-scan.md) | A04 | Column/field mismatch errors rather than silently dropping data |
| [M13](roadmap/M13-query-builder.md) | A03 | Injection payload suite across every builder; `FuzzQuoteIdentifier` |
| [M14](roadmap/M14-quriny-entitystore.md) | A03, A09 | Migration preserves parameterisation; no new log surface |
| [M15](roadmap/M15-copy-protocol.md) | A03, DoS | COPY table/column names quoted; 1M-row copy stays in bounded memory |
| [M16](roadmap/M16-cli.md) | A09, A05 | DSN never echoed; password not visible in `ps`; `--help` shows no secrets |
| [M17](roadmap/M17-release-oss.md) | A06, A08 | `govulncheck` clean; SBOM published; release checksums signed |

---

## 7. Further reading

- [SQL Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html) · [Query Parameterization](https://cheatsheetseries.owasp.org/cheatsheets/Query_Parameterization_Cheat_Sheet.html)
- [Transport Layer Security](https://cheatsheetseries.owasp.org/cheatsheets/Transport_Layer_Security_Cheat_Sheet.html) · [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [Logging](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html) · [Error Handling](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html) · [Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)
- [Denial of Service](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html) · [Input Validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)
- [Threat Modeling](https://cheatsheetseries.owasp.org/cheatsheets/Threat_Modeling_Cheat_Sheet.html) · [Attack Surface Analysis](https://cheatsheetseries.owasp.org/cheatsheets/Attack_Surface_Analysis_Cheat_Sheet.html)
- [OWASP Top 10 CI/CD Security Risks](https://owasp.org/www-project-top-10-ci-cd-security-risks/) · [Vulnerable Dependency Management](https://cheatsheetseries.owasp.org/cheatsheets/Vulnerable_Dependency_Management_Cheat_Sheet.html)
- [Go Secure Coding Practices Guide](https://owasp.org/www-project-go-secure-coding-practices-guide/) · [govulncheck](https://go.dev/blog/vuln)
