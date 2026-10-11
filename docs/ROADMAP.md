# forge — Development Roadmap

A PostgreSQL driver and toolkit for Go, built from zero, test-first, one
milestone per branch. Each milestone specifies the failing test, the API shape
to implement, and the reference material behind the design.

Companion documents: [REFERENCES.md](./REFERENCES.md) (reference library) ·
[SECURITY-PRACTICES.md](./SECURITY-PRACTICES.md) (threat model + OWASP mapping) ·
[adr/](./adr/) (decision records).

**Read [SECURITY-PRACTICES.md](./SECURITY-PRACTICES.md) before M00.** forge sits
on the injection and crypto boundary of every application that imports it; every
milestone below carries a `## Security (OWASP)` section that is part of its
definition of done, not an appendix.

---

## How to use this roadmap

1. **One milestone = one branch = one PR.** Never start M(n+1) before M(n) is
   merged into `main`.
2. **Red → green → refactor, always in that order.** The RED step tells you the
   test file, test name, and exact assertions. Write it. Watch it fail *for the
   right reason* (compile error counts as red). Only then read the GREEN step.
3. **Commit on every green.** A milestone should produce 6–15 commits, not one.
4. **Never skip the REFACTOR step.** The design settles there, not in GREEN.
5. GREEN steps state the API shape, not an implementation. Where they stop, the
   linked reference material carries the rest.

## The TDD loop, concretely

```bash
go test ./...              # red — expected failure
# write the smallest code that could work
go test ./...              # green
gofmt -l -s -w . && golangci-lint run ./...
go test -race ./...
git add -A && git commit
```

## Definition of done (every milestone)

- [ ] All new behaviour arrived test-first; no untested exported function.
- [ ] `go test ./...` green; `go test -race ./...` green.
- [ ] `gofmt -l -s -w .` produces no output.
- [ ] `golangci-lint run ./...` clean.
- [ ] Exported identifiers have doc comments starting with the identifier name.
- [ ] No credentials, DSN passwords, or query argument values in any log line or
      error string (grep the diff for `%v` on config structs).
- [ ] Import direction respected — see the layer table below.
- [ ] ADR written if anything diverges from pgx or `database/sql/driver`.
- [ ] PR opened, CI green, squash-merged, branch deleted.

**Security gates** (from [SECURITY-PRACTICES.md](./SECURITY-PRACTICES.md) §5):

- [ ] The milestone's `## Security (OWASP)` section is worked through, not skimmed.
- [ ] At least one *negative* test exists — something an attacker would try,
      asserted to fail safely.
- [ ] No secret takes a formatting verb: `grep -nE '%[vsq].*(assword|ecret|oken|DSN|args)'`
      over the diff returns nothing.
- [ ] `gosec` clean, or every `#nosec` carries a one-line justification.
- [ ] Any new parsing of untrusted bytes has a fuzz target.
- [ ] `govulncheck ./...` clean.

## Layer rules (never violated)

```
qb/            → (nothing)
forge (root)   → stdlib only
pgwire, pgtype → stdlib only
pgconn         → pgwire + pgtype
postgres/      → pgconn + forge
pool/, scan/   → forge only          ← never postgres/*
tracelog/, multitracer/ → pgconn interfaces
```

A test proves this in M1 (`TestImportDirection`, via `go list -deps`).

## Git workflow

```bash
git switch main && git pull --ff-only
git switch -c feat/<milestone>

# red
git commit -m "test(pgwire): startup message round-trips"
# green
git commit -m "feat(pgwire): encode/decode StartupMessage"
# refactor
git commit -m "refactor(pgwire): extract writeBuf helper"

git push -u origin feat/<milestone>
gh pr create --fill --base main
# after CI green:
gh pr merge --squash --delete-branch
```

Commit types: `test:` `feat:` `fix:` `refactor:` `docs:` `chore:` `perf:`
(see [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)).
Scope = package: `pgwire`, `pgconn`, `pgtype`, `pool`, `scan`, `qb`, `cli`.

---

## Milestones

| # | Milestone | Branch | Covers | Primary risk |
|---|---|---|---|---|
| [M00](roadmap/M00-repo-foundation.md) | Repo foundation & CI | `chore/repo-foundation` | Module layout, CI, linting, OSS hygiene | A06 supply chain |
| [M01](roadmap/M01-core-interfaces.md) | Core interfaces & errors | `feat/core-interfaces` | Interface design, sentinel errors, `errors.Is/As` | A04, A09 |
| [M02](roadmap/M02-pgwire-protocol.md) | Wire protocol v3 | `feat/pgwire-protocol` | Binary framing, `encoding/binary`, fuzzing | A08, DoS |
| [M03](roadmap/M03-pgconn-simple-query.md) | Connection + simple query | `feat/pgconn-simple-query` | `net.Conn`, state machines, mock backends | A09, A10 |
| [M04](roadmap/M04-auth-scram.md) | SCRAM-SHA-256 auth | `feat/auth-scram` | HMAC, PBKDF2, constant-time compare | A02, A07 |
| [M05](roadmap/M05-tls-sslmode.md) | TLS & sslmode | `feat/tls-sslmode` | TLS handshake, cert verification, config parsing | A02, A05 |
| [M06](roadmap/M06-pgtype-text.md) | Text codecs & registry | `feat/pgtype-text` | Type systems, generics, NULL modelling | A03 |
| [M07](roadmap/M07-extended-query.md) | Extended query protocol | `feat/extended-query` | Prepared statements, LRU caching | A03, A01 |
| [M08](roadmap/M08-pgtype-binary.md) | Binary codecs | `feat/pgtype-binary` | Wire encodings, time zones, arrays | A08, DoS |
| [M09](roadmap/M09-cancel-context.md) | Cancellation & deadlines | `feat/cancel-context` | `context`, goroutine lifecycle, leak tests | DoS |
| [M10](roadmap/M10-postgres-driver-adapter.md) | Driver adapter | `feat/postgres-driver-adapter` | Adapter pattern, build tags, integration tier | A09 |
| [M11](roadmap/M11-pool.md) | Connection pool | `feat/pool` | Semaphores, `sync`, benchmarks, race testing | DoS, A01 |
| [M12](roadmap/M12-scan.md) | Row scanner | `feat/scan` | Reflection + generics, struct tags | A04 |
| [M13](roadmap/M13-query-builder.md) | Query builder | `feat/query-builder` | Fluent APIs, injection surfaces, dialects | **A03 injection** |
| [M14](roadmap/M14-quriny-entitystore.md) | Quriny EntityStore swap | `feat/quriny-entitystore` | API compatibility, real-world migration | A03, A09 |
| [M15](roadmap/M15-copy-protocol.md) | COPY protocol | `feat/copy-protocol` | Streaming IO, backpressure, pprof | A03, DoS |
| [M16](roadmap/M16-cli.md) | `forge` CLI | `feat/cli` | CLI ergonomics, testable `main`, signals | A09, A05 |
| [M17](roadmap/M17-release-oss.md) | Release & open source | `chore/release-v0.1.0` | Semver, changelog, pkg.go.dev, releases | A06, A08 |

M02, M06, M07, M08 and M11 carry the most design risk. Do not compress them.

## Phases

- **Phase 1 — foundations** (M00–M03): talk to a real PostgreSQL and run
  `SELECT 1` end to end.
- **Phase 2 — production connection** (M04–M09): auth, TLS, types, prepared
  statements, cancellation. The driver becomes deployable here.
- **Phase 3 — toolkit** (M10–M13): the ergonomic layer users actually touch.
- **Phase 4 — proof & polish** (M14–M17): real consumer, COPY, CLI, release.

## Security spine

Three milestones are load-bearing for security and must not be rushed:
**M04** (authentication), **M05** (transport), **M13** (injection). Two more are
the memory-safety boundary against a hostile server: **M02** and **M08**.
The full threat model, OWASP Top 10 mapping, ASVS control table, coding rules,
and per-milestone security index live in
[SECURITY-PRACTICES.md](./SECURITY-PRACTICES.md).
