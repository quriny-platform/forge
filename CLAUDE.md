This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

forge is a PostgreSQL driver and toolkit for Go (`github.com/quriny-platform/forge`). It provides engine-agnostic core abstractions modeled on `database/sql/driver`, with PostgreSQL as the first implementation. Requires Go 1.26+ and targets PostgreSQL 14+.

## Build & Test Commands

```
# Run all tests
go test ./...

# Run a specific test
go test -run TestFunctionName ./...

# Run tests for a specific package
go test ./postgres/pgconn/...

# Run tests with race detector
go test -race ./...

# Fuzz a target (protocol parser, codecs, identifier quoting)
go test -fuzz=FuzzName ./path

# Integration tests against local PostgreSQL
docker compose up -d
go test -tags=integration ./...

# Format
gofmt -l -s -w .

# Lint
golangci-lint run ./...
```

## Architecture

Layered architecture, bottom-up:

- `forge` (root package) — engine-agnostic core interfaces: `Driver`, `Conn`, `Rows`, `Result`, `Stmt`, `Tx`. Modeled on `database/sql/driver`; stdlib-only.
- `postgres/pgwire/` — PostgreSQL wire protocol v3 encoder/decoder. `FrontendMessage`/`BackendMessage` types for every protocol message. Stdlib-only.
- `postgres/pgtype/` — Type system mapping between Go and PostgreSQL types (OID ↔ Go, text + binary). `Codec`, `Registry`. Stdlib-only.
- `postgres/pgconn/` — Connection layer. Handles authentication (SCRAM-SHA-256, md5 awareness), TLS/sslmode, startup, simple + extended query lifecycle, cancellation, COPY. Imports `pgwire` + `pgtype`.
- `postgres/` — Implements root `forge.Driver`/`Conn`/`Rows`/`Tx` as an adapter over `pgconn`.
- `pool/` — Concurrency-safe connection pool (channel/semaphore based). Engine-agnostic: depends only on `forge.Conn`.
- `scan/` — Row scanner: reflection + generics into structs and `map[string]any`. Engine-agnostic: depends only on `forge`.
- `qb/` — Query builder with per-engine `Dialect` (placeholders, identifier quoting). Depends on nothing.
- `tracelog/` — Logging adapter implementing the tracer interfaces (`QueryTracer`, etc.) on `pgconn.Config`.
- `multitracer/` — Composes multiple tracers into one.

Supporting:

- `internal/stmtcache/` — Prepared statement cache with LRU eviction, used by `pgconn`.
- `internal/sanitize/` — SQL query + args redaction for logging (no raw arg values ever logged).
- `internal/testutil/` — scripted mock backend (unit-test-speed protocol conversations), docker-postgres harness (integration tier), and cross-connection-kind test helpers (≈ pgxtest).
- `docs/adr/` — architecture decision records, one per divergence from pgx's design.

## Key Design Conventions

- **Import direction is strict** — lower layers never import upper layers. `forge` root and `pgwire`/`pgtype` are stdlib-only; `pgconn` may import `pgwire`+`pgtype`; `pool`/`scan` depend only on `forge`, never on `postgres/*`; `qb` is pure.
- **TDD** — tests written before implementation on every milestone (red → green → refactor). Unit tests run against fakes/mock backend; docker-postgres integration tests are a separate, slower tier.
- **Context-based** — all blocking operations take `context.Context` and honor cancellation/deadlines.
- **Security non-negotiables** — no credentials in logs or error strings; `crypto/subtle` constant-time comparisons in auth; no `InsecureSkipVerify` default; toolkit APIs are parameterized-query-only, with identifier quoting for dynamic SQL; fuzz tests on protocol parser, binary codecs, and identifier quoting.
- **One branch per milestone** — `feat/core-interfaces`, `feat/pgwire-protocol`, `feat/pgconn-simple-query`, `feat/auth-scram`, `feat/tls-sslmode`, `feat/pgtype-text`, `feat/extended-query`, `feat/pgtype-binary`, `feat/cancel-context`, `feat/postgres-driver-adapter`, `feat/pool`, `feat/scan`, `feat/query-builder`, `feat/quriny-entitystore`, `feat/copy-protocol`.
- **ADRs** — every divergence from pgx's reference design recorded in `docs/adr/`.
- **Formatting** — run `gofmt -l -s -w .` after changes.
- **Linters** — `govet` plus `golangci-lint` defaults.

## Eventual Consumer

Quriny's `EntityStore` (`quriny-platform/platform/backend/internal/store/postgres.go`) currently uses `pgxpool.Pool.Query`, `pgx.CollectRows`/`RowToMap`, `CollectExactlyOneRow`, `pgx.ErrNoRows`, `map[string]any` records, UUID `[16]byte`→string. `pool` and `scan` mirror these names/semantics for a mechanical swap (milestone `feat/quriny-entitystore`).
