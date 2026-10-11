# forge — Reference Library

Stable references cited from [ROADMAP.md](./ROADMAP.md). *Core* items back the
design decisions in the milestone that cites them; *Deeper* items cover the
**Stretch** tasks.

---

## 1. Test-Driven Development in Go

**Core**
- [Learn Go with Tests](https://quii.gitbook.io/learn-go-with-tests) — the whole book; the roadmap deep-links chapters per milestone.
- [Learn Go with Tests — Install Go / TDD cycle](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/install-go)
- [Learn Go with Tests — Mocking](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/mocking)
- [Learn Go with Tests — Reflection](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/reflection)
- [Learn Go with Tests — Sync](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/sync)
- [Learn Go with Tests — Context](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/context)
- [Learn Go with Tests — Generics](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/generics)
- [`testing` package docs](https://pkg.go.dev/testing)

**Deeper**
- [Go Fuzzing tutorial](https://go.dev/doc/tutorial/fuzz)
- [Go Fuzzing reference](https://go.dev/security/fuzz/)
- [Subtests and sub-benchmarks](https://go.dev/blog/subtests)
- [Table-driven tests (Go wiki)](https://go.dev/wiki/TableDrivenTests)
- [`testing/synctest` — testing concurrent code](https://pkg.go.dev/testing/synctest)

## 2. Go language & idiom

**Core**
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Error handling and Go](https://go.dev/blog/error-handling-and-go)
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors) — `%w`, `errors.Is`, `errors.As`
- [Go Concurrency Patterns: Context](https://go.dev/blog/context)
- [Tutorial: Generics](https://go.dev/doc/tutorial/generics)

**Deeper**
- [The Go Memory Model](https://go.dev/ref/mem)
- [Data Race Detector](https://go.dev/doc/articles/race_detector)
- [`sync` package](https://pkg.go.dev/sync) · [`sync/atomic`](https://pkg.go.dev/sync/atomic)
- [Profiling Go Programs](https://go.dev/blog/pprof)
- [`go doc` comment conventions](https://go.dev/doc/comment)

## 3. Database & driver design

**Core**
- [`database/sql/driver`](https://pkg.go.dev/database/sql/driver) — the interface set `forge` root mirrors
- [`database/sql`](https://pkg.go.dev/database/sql)
- [Go database/sql tutorial](https://go.dev/doc/database/index)

**Reference implementations — read, never copy**
- [jackc/pgx](https://github.com/jackc/pgx) · [jackc/pgproto3](https://github.com/jackc/pgx/tree/master/pgproto3) · [jackc/pgtype internals](https://github.com/jackc/pgx/tree/master/pgtype)
- [lib/pq](https://github.com/lib/pq)

## 4. PostgreSQL internals

**Core**
- [Frontend/Backend Protocol — overview](https://www.postgresql.org/docs/current/protocol.html)
- [Message Formats](https://www.postgresql.org/docs/current/protocol-message-formats.html) — byte layout of every message
- [Message Flow](https://www.postgresql.org/docs/current/protocol-flow.html) — startup, simple query, extended query, COPY, cancellation
- [Error and Notice Message Fields](https://www.postgresql.org/docs/current/protocol-error-fields.html)
- [SASL Authentication](https://www.postgresql.org/docs/current/sasl-authentication.html)
- [Binary format / `pg_type`](https://www.postgresql.org/docs/current/catalog-pg-type.html)

**Deeper**
- [`send`/`recv` functions in the source tree](https://github.com/postgres/postgres/tree/master/src/backend/utils/adt)
- [libpq connection strings & sslmode](https://www.postgresql.org/docs/current/libpq-connect.html)
- [COPY](https://www.postgresql.org/docs/current/sql-copy.html)
- [Prepared statements & the extended protocol](https://www.postgresql.org/docs/current/sql-prepare.html)

## 5. Security & cryptography

- [RFC 5802 — SCRAM](https://datatracker.ietf.org/doc/html/rfc5802) (test vectors in §5)
- [RFC 7677 — SCRAM-SHA-256](https://datatracker.ietf.org/doc/html/rfc7677)
- [RFC 4013 — SASLprep](https://datatracker.ietf.org/doc/html/rfc4013)
- [`crypto/subtle`](https://pkg.go.dev/crypto/subtle) · [`golang.org/x/crypto/pbkdf2`](https://pkg.go.dev/golang.org/x/crypto/pbkdf2) (or Go 1.24+ [`crypto/pbkdf2`](https://pkg.go.dev/crypto/pbkdf2))
- [`crypto/tls`](https://pkg.go.dev/crypto/tls) · [`crypto/x509`](https://pkg.go.dev/crypto/x509)
- [govulncheck](https://go.dev/blog/vuln)

## 6. Security — OWASP

Read the first three before M00; the cheat sheets are cited per milestone. The
forge-specific mapping lives in [SECURITY-PRACTICES.md](./SECURITY-PRACTICES.md).

**Core**
- [OWASP Top 10 2021](https://owasp.org/Top10/)
- [OWASP Proactive Controls](https://owasp.org/www-project-proactive-controls/)
- [OWASP ASVS](https://owasp.org/www-project-application-security-verification-standard/)
- [Go Secure Coding Practices Guide](https://owasp.org/www-project-go-secure-coding-practices-guide/)

**Injection (M06, M07, M13, M15)**
- [SQL Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html)
- [Query Parameterization](https://cheatsheetseries.owasp.org/cheatsheets/Query_Parameterization_Cheat_Sheet.html)
- [Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Injection_Prevention_Cheat_Sheet.html)
- [SQL Injection (attack description)](https://owasp.org/www-community/attacks/SQL_Injection)

**Auth, crypto & transport (M04, M05)**
- [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [Cryptographic Storage](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
- [Transport Layer Security](https://cheatsheetseries.owasp.org/cheatsheets/Transport_Layer_Security_Cheat_Sheet.html)

**Logging, errors & secrets (M01, M03, M10, M16)**
- [Logging](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)
- [Error Handling](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html)
- [Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)

**Availability & input (M02, M08, M11, M15)**
- [Denial of Service](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html)
- [Input Validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)

**Design & supply chain (M00, M01, M17)**
- [Threat Modeling](https://cheatsheetseries.owasp.org/cheatsheets/Threat_Modeling_Cheat_Sheet.html)
- [Abuse Case](https://cheatsheetseries.owasp.org/cheatsheets/Abuse_Case_Cheat_Sheet.html)
- [Attack Surface Analysis](https://cheatsheetseries.owasp.org/cheatsheets/Attack_Surface_Analysis_Cheat_Sheet.html)
- [Top 10 CI/CD Security Risks](https://owasp.org/www-project-top-10-ci-cd-security-risks/)
- [Vulnerable Dependency Management](https://cheatsheetseries.owasp.org/cheatsheets/Vulnerable_Dependency_Management_Cheat_Sheet.html)
- [Software Component Verification Standard](https://owasp.org/www-project-software-component-verification-standard/) · [CycloneDX](https://owasp.org/www-project-cyclonedx/)
- [Web Security Testing Guide](https://owasp.org/www-project-web-security-testing-guide/)

## 7. CLI craft

- [Command Line Interface Guidelines](https://clig.dev/)
- [`flag` package](https://pkg.go.dev/flag)
- [`os/signal` + `signal.NotifyContext`](https://pkg.go.dev/os/signal#NotifyContext)
- [`rogpeppe/go-internal/testscript`](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) — script-driven CLI tests
- [Learn Go with Tests — Command line & project structure](https://quii.gitbook.io/learn-go-with-tests/build-an-application/command-line)

## 8. Open source practice

- [Go Modules Reference](https://go.dev/ref/mod) · [Module version numbering](https://go.dev/doc/modules/version-numbers)
- [Semantic Versioning 2.0.0](https://semver.org/)
- [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
- [Choose a License](https://choosealicense.com/) · [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/)
- [Publishing a Go module](https://go.dev/doc/modules/publishing) · [pkg.go.dev about](https://pkg.go.dev/about)
- [golangci-lint](https://golangci-lint.run/) · [GitHub Actions for Go](https://github.com/actions/setup-go)
- [Architecture Decision Records (Nygard)](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)
