# 0003. Rows value access and untyped query arguments

Date: 2026-10-25
Status: Accepted

## Context

`database/sql/driver.Rows` exposes `Columns() []string` and
`Next(dest []driver.Value) error`. A consumer cannot obtain a row's values
without supplying a destination slice, and cannot see column type metadata
beyond the name. `driver.Conn` takes arguments as `[]driver.NamedValue`, whose
`Value` is restricted to a small set of types (int64, float64, bool, []byte,
string, time.Time), so every other Go type must be converted by the caller or
by `database/sql` before the driver sees it.

forge's `scan` package (M12) must support `RowToMap` and struct scanning
without a second round trip, which needs both the decoded values of the
current row and the column metadata the server sent with it. pgx solves this
with `Rows.Values()` and `Rows.FieldDescriptions()`.

## Decision

`forge.Rows` exposes:

- `Values() ([]any, error)` — the current row decoded to Go values.
- `FieldDescriptions() []FieldDescription` — name, table OID, attribute
  number, type OID, type size, type modifier and format code per column.

`Next() bool` reports errors through `Err()`, the same contract as
`database/sql.Rows` and `bufio.Scanner`; that is a borrowed convention, not a
divergence.

`forge.Conn`, `forge.Stmt` and `forge.Tx` take arguments as `args ...any`.
Mapping Go values to wire formats is the job of `pgtype`'s codec registry
(M06, M08), so the core interface does not restrict argument types.

The SQL text and its arguments stay separate at every entry point. There is no
variant that accepts SQL formatted from values.

## Consequences

- `scan` can implement `RowToMap` and struct scanning on top of the `forge`
  interfaces alone, never importing an engine package.
- `Values()` is an addition over `driver.Rows`, so a `database/sql` shim would
  need to adapt it. Mapping `forge.Conn` onto `driver.Connector` is possible
  but not free.
- `...any` pushes type validation to runtime: an unsupported argument type is
  reported when the query is encoded, not at compile time.
- The behaviour of `Values()` before the first `Next()` or after `Close()` is
  part of the `Rows` contract and must be tested by every implementation.
