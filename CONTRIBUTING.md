# Contributing to forge

forge is in alpha. The API breaks without notice until v0.1.0, and the build
order is fixed — see [docs/ROADMAP.md](docs/ROADMAP.md).

## Running the tests

```bash
go test ./...                      # unit tests, no database required
go test -race ./...
gofmt -l -s -w . && golangci-lint run ./...
```

Unit tests run against fakes and a scripted mock backend, so a clean checkout
is productive without provisioning PostgreSQL.

Integration tests are a separate, slower tier:

```bash
docker compose up -d               # PostgreSQL 17 on :5433
go test -tags=integration ./...
docker compose down -v
```

Port 5433 is deliberate — it will not collide with a local PostgreSQL install.

Integration tests read `FORGE_TEST_DSN` and skip when it is unset:

```
postgres://postgres:forge_test@localhost:5433/forge_test?sslmode=disable
```

`sslmode=disable` is correct **only** for this throwaway loopback container.
Never copy that DSN into real configuration; forge defaults to verified TLS
and does not set `InsecureSkipVerify`.

## Test-first, always

Every change arrives red → green → refactor, in that order:

1. Write the test. Run it. Watch it fail *for the right reason* — a compile
   error counts as red.
2. Write the smallest code that passes.
3. Refactor. The design settles here, not in the green step.

No exported function ships untested. See
[docs/ROADMAP.md](docs/ROADMAP.md) § "The TDD loop, concretely".

## Commits and branches

[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).
Types: `test:` `feat:` `fix:` `refactor:` `docs:` `chore:` `perf:`.
Scope is the package: `pgwire`, `pgconn`, `pgtype`, `pool`, `scan`, `qb`, `cli`.

```
test(pgwire): startup message round-trips
feat(pgwire): encode/decode StartupMessage
refactor(pgwire): extract writeBuf helper
```

One milestone = one branch = one PR, squash-merged. Commit on every green; a
milestone produces 6–15 commits, not one.

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

Lower layers never import upper layers. `pool/` and `scan/` work against the
root interfaces so a second engine needs no changes above `forge`.
`TestImportDirection` enforces this in CI via `go list -deps`.

Read this before writing an import, not after a red build.

## Architecture decisions

Every divergence from pgx or from `database/sql/driver` conventions requires an
ADR in [docs/adr/](docs/adr/), in the same PR as the change. ADRs are immutable
after merge — a changed mind produces a new ADR that supersedes the old one.
Reviewers reject divergences that arrive without one.

Update the index in [docs/adr/README.md](docs/adr/README.md) in the same commit.

## Security

New exported APIs must be classified against the trust boundaries TB1–TB4 in
[ADR 0002](docs/adr/0002-security-boundaries.md). A convenience helper that
interpolates values into SQL cannot be merged regardless of how ergonomic it is.

Each milestone carries a `## Security (OWASP)` section that is part of its
definition of done. The full threat model is in
[docs/SECURITY-PRACTICES.md](docs/SECURITY-PRACTICES.md).

Report vulnerabilities privately — see [SECURITY.md](SECURITY.md), not the
issue tracker.

## Pull requests

`main` is protected: pull requests are required, the `test`, `race` and `lint`
checks must pass, and force-pushes are rejected.

Before opening a PR, work the Definition of Done in
[docs/ROADMAP.md](docs/ROADMAP.md) § "Definition of done (every milestone)".

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
