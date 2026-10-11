# M10 — Driver adapter & first real database  `feat/postgres-driver-adapter`

**Covers:** the adapter pattern · build tags · unit vs integration tiers ·
cross-implementation test suites · tracing hooks

**Links**
- [`database/sql/driver`](https://pkg.go.dev/database/sql/driver)
- [Go build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints)
- [Learn Go with Tests — Dependency injection](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/dependency-injection)
- [Docker: postgres image](https://hub.docker.com/_/postgres)

**Packages:** `postgres/`, `internal/testutil`, `tracelog/`, `multitracer/`.

> This is the milestone where `SELECT 1` runs against a real server through the
> public `forge` interfaces. Everything before it was scaffolding.

---

## Steps

- [ ] **1. RED — the conformance suite, first.** `internal/testutil/conformance.go`
  exports a suite that any `forge.Conn` implementation must pass:
  ```go
  func RunConnTests(t *testing.T, connect func(t *testing.T) forge.Conn)
  ```
  Cases: `Ping`; `Query` returning rows; `Exec` returning `RowsAffected`;
  `Query` with no rows → `Next()` false, `Err()` nil; scanning into wrong-arity
  dest → error; `Rows.Close` before exhaustion leaves the conn reusable;
  transaction commit persists / rollback discards; `ErrNoRows` from a
  single-row helper; a query with a syntax error returns a SQLSTATE-bearing
  error; using a closed conn returns `forge.ErrClosed`.
  Write it before the adapter so the adapter is built to pass it. It is also the
  test suite for the *fake* implementation, which keeps the fake honest.

- [ ] **2. GREEN — `postgres/postgres.go`.**
  ```go
  type Driver struct{}
  func (Driver) Open(ctx context.Context, dsn string) (forge.Conn, error)

  type Conn struct{ pg *pgconn.PgConn; typeMap *pgtype.Map }
  type Rows struct{ /* wraps *pgconn.ResultReader */ }
  type Result struct{ tag pgconn.CommandTag }
  type Tx struct{ c *Conn; closed bool }
  ```
  Compile-time assertions at the top of the file:
  `var _ forge.Driver = Driver{}` and so on for every type.
  The adapter's real work is translation:
  - `pgconn.CommandTag` → `forge.Result` (`RowsAffected`, `Command`)
  - `pgwire.FieldDescription` → `forge.FieldDescription` (copy, do not alias)
  - `*pgconn.PgError` → returned as-is, but wrapped in `*forge.QueryError` with
    the SQL so callers get context; `errors.As` must still reach the `PgError`.
  - no rows on a single-row read → `forge.ErrNoRows`
  - `...any` args → encoded through `pgtype.Map` into `[][]byte` + format codes
  *Commits:* `feat(postgres): adapter conn and driver`, `feat(postgres): rows and result adapters`

- [ ] **3. RED — argument encoding.** Table test through the adapter with the
  mock backend: `int64`, `string`, `nil`, `[]byte`, `time.Time`, `[]int32`,
  `*int32(nil)`, a `driver.Valuer`, an unsupported type (a channel) → clear
  error naming the argument index and Go type. Argument errors must not reach the
  server; they are a client-side failure.

- [ ] **4. GREEN — `Conn.Query`/`Exec`.** Resolve each arg to (OID, format,
  bytes) via `pgtype.Map`, then call `pgconn.ExecParams`/`ExecPrepared`. Choose
  binary result formats where the codec supports it (from M08).
  *Commit:* `feat(postgres): encode query arguments through the type map`

- [ ] **5. RED/GREEN — `Rows.Scan`.** Delegate to `pgtype.Map.Scan` per column,
  after checking `len(dest) == len(fields)` and returning a message naming both
  counts. `Rows.Values()` decodes every column into `any` using the OID's default
  Go type — that mapping table (`int4`→`int32`, `text`→`string`, `uuid`→`string`,
  `numeric`→`pgtype.Numeric`, `jsonb`→`[]byte`) belongs in a doc comment because
  users will depend on it.
  *Commit:* `feat(postgres): scan and value decoding`

- [ ] **6. RED/GREEN — transactions.** `Tx` wraps the conn; `Commit`/`Rollback`
  are idempotent-ish: the second call returns `forge.ErrTxClosed`. Nested
  `Begin` on a `Tx` issues `SAVEPOINT`/`RELEASE`/`ROLLBACK TO` — implement it or
  return a clear "nested transactions unsupported" error and note the decision.
  *Commit:* `feat(postgres): transaction adapter`

- [ ] **7. GREEN — run the conformance suite twice.** Once against a fake
  `forge.Conn` built on the scripted backend, once (build-tagged) against real
  PostgreSQL. Same suite, two constructors. This is the `pgxtest` idea and it is
  worth the setup: a bug that only the real server catches is exactly the bug
  worth catching.
  *Commit:* `test: run conformance suite against mock and real postgres`

- [ ] **8. Integration harness.** `internal/testutil/docker.go`: read
  `FORGE_TEST_DSN` from the environment, skip with a helpful message when unset,
  and provide `WithTempTable(t, schema, fn)` that creates and drops a table per
  test so the suite is order-independent and parallel-safe.
  *Commit:* `test: add docker postgres harness`

- [ ] **9. RED/GREEN — tracing.** Define the tracer interfaces on `pgconn`:
  ```go
  type QueryTracer interface {
      TraceQueryStart(ctx context.Context, conn *PgConn, data TraceQueryStartData) context.Context
      TraceQueryEnd(ctx context.Context, conn *PgConn, data TraceQueryEndData)
  }
  ```
  plus `ConnectTracer`, `BatchTracer`, `CopyFromTracer`, `PrepareTracer`.
  Implement `tracelog.TraceLog` over an interface with one method
  (`Log(ctx, level, msg string, data map[string]any)`) so users can adapt
  `slog`, `zap`, or anything else. `multitracer.New(tracers...)` fans out.
  **The redaction test is the important one**: `internal/sanitize` must replace
  every argument value with a placeholder before logging, and a test must assert
  a secret argument never appears in the log output.
  *Commits:* `feat(pgconn): tracer interfaces`, `feat(tracelog): logging tracer`, `feat(multitracer): compose tracers`

- [ ] **10. README usage section.** Replace the M00 placeholder with the real
  15-line example, and *run it* as an `Example` in `example_test.go` so it cannot
  rot. Also add a runnable `ExampleConn_Query` for pkg.go.dev.
  *Commit:* `docs: real usage example, verified as a test`

## Security (OWASP)

**Risks: [A09 Logging Failures](https://owasp.org/Top10/A09_2021-Security_Logging_and_Monitoring_Failures/) · [A03 Injection](https://owasp.org/Top10/A03_2021-Injection/)**

Tracing is where a driver most often leaks. Reference:
[Logging cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html). ASVS V7.

- [ ] **`internal/sanitize` is a security component.** It redacts argument values
  *before* they reach any formatting verb, replacing each with a typed
  placeholder (`$1=<string len=7>`). Redacting after formatting is too late —
  the secret already exists as a string in memory and in the log buffer.
- [ ] **Default to redacted.** Full argument logging, if offered at all, is
  `Config.LogQueryArgs` — off by default, doc comment warning it will log PII
  and credentials.
- [ ] **SQL text may itself contain secrets** (a literal in a hand-written
  query). Truncate logged SQL to a documented length and note the risk.
- [ ] **Tracers are user code running on the hot path.** A panicking tracer must
  not take down the connection: recover, disable the tracer, surface the error.
  Test it with a deliberately panicking tracer.
- [ ] **Argument encoding errors are client-side.** An unsupported Go type must
  fail before anything reaches the server, with a message naming the argument
  index and type — never the value.
- [ ] **`Rows.Values()` type mapping is a contract.** Silent type coercion is how
  a `[]byte` becomes a `string` becomes an injection somewhere upstream. Document
  the OID→Go table and test it.
- [ ] **Integration tests must not use a superuser.** Create a least-privilege
  role for the test harness; a driver test suite that only passes as `postgres`
  is hiding permission bugs.

**Verify:** `TestTracerRedactsArguments` — run a query whose argument is
`"hunter2"`, capture every tracer callback and log line, assert absence.

## Done when

- `docker compose up -d && go test -tags=integration ./...` green.
- The conformance suite passes identically against mock and real server.
- `go vet` reports no `forge.Conn` implementation missing a method.
- A query with an argument containing `"hunter2"` never logs it.

## Stretch

- A `database/sql` shim: implement `driver.Connector` so `sql.OpenDB(forge…)`
  works, and run the stdlib's own `database/sql` tests against it.
- Add `pgconn` support for `LISTEN`/`NOTIFY` surfacing through the adapter.
