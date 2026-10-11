# forge

A PostgreSQL driver and toolkit for Go, built on the wire protocol directly.

> **Status: alpha.** Under active development, no release yet. The API will
> break without notice until v0.1.0. Not ready for production use.

## Install

```bash
go get github.com/quriny-platform/forge
```

Requires Go 1.26+ and PostgreSQL 14+.

## Usage

> Target API. Not yet implemented — see the status table below.

```go
ctx := context.Background()

db, err := pool.New(ctx, "postgres://user:pass@localhost:5432/mydb")
if err != nil {
    log.Fatal(err)
}
defer db.Close()

rows, err := db.Query(ctx, "SELECT id, name FROM users WHERE active = $1", true)
if err != nil {
    log.Fatal(err)
}

users, err := scan.CollectRows(rows, scan.RowToStructByName[User])
if err != nil {
    log.Fatal(err)
}
```

## Status

| # | Milestone | State |
|---|---|---|
| M00 | Repo foundation & CI | 🔨 in progress |
| M01 | Core interfaces & errors | ⬜ |
| M02 | Wire protocol v3 | ⬜ |
| M03 | Connection + simple query | ⬜ |
| M04 | SCRAM-SHA-256 auth | ⬜ |
| M05 | TLS & sslmode | ⬜ |
| M06 | Text codecs & registry | ⬜ |
| M07 | Extended query protocol | ⬜ |
| M08 | Binary codecs | ⬜ |
| M09 | Cancellation & deadlines | ⬜ |
| M10 | Driver adapter | ⬜ |
| M11 | Connection pool | ⬜ |
| M12 | Row scanner | ⬜ |
| M13 | Query builder | ⬜ |
| M14 | EntityStore migration | ⬜ |
| M15 | COPY protocol | ⬜ |
| M16 | CLI | ⬜ |
| M17 | Release & open source | ⬜ |

## Architecture

```
                      your application
                             │
        ┌──────────┬─────────┼──────────┐
        │          │         │          │
      pool/      scan/      qb/     postgres/
        │          │         │          │
        └─────┬────┘    (no deps)    pgconn
              │                      ┌──┴───┐
         forge (root)             pgwire  pgtype
       engine-agnostic             wire   OID ↔ Go
        Conn/Rows/Tx             protocol  codecs
```

Arrows point in the import direction. `pool/`, `scan/` and `qb/` never import
`postgres/*` — they work against the root interfaces, so a second engine needs
no changes above `forge`. `forge`, `pgwire` and `pgtype` are stdlib-only.

## Non-goals

forge does not authorise queries, sanitise application-authored SQL strings,
encrypt data at rest, manage keys, or rate-limit callers. It gives you
parameterised queries and identifier quoting; everything above that boundary
is the application's job. See [ADR 0002](docs/adr/0002-security-boundaries.md).

## Development

```bash
go test ./...                      # unit tests, no database required
go test -race ./...
gofmt -l -s -w . && golangci-lint run ./...

docker compose up -d               # PostgreSQL 17 on :5433
go test -tags=integration ./...
```

## Licence

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).

Chosen over MIT for its explicit patent grant (§3). A driver sits in the
dependency tree of every application that imports it; the patent position
should not be left implicit.
