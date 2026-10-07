# M01 — Core interfaces & errors  `feat/core-interfaces`

**Covers:** interface design · sentinel errors & wrapping · `errors.Is`/`As` ·
table-driven tests · compile-time interface assertions

**Links**
- [`database/sql/driver`](https://pkg.go.dev/database/sql/driver) — read every interface before you start
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- [Error handling and Go](https://go.dev/blog/error-handling-and-go)
- [Learn Go with Tests — Structs, methods & interfaces](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/structs-methods-and-interfaces)
- [Go Code Review Comments — interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)

**Package:** `forge` (repo root). **Imports allowed:** stdlib only.

> `errors_test.go` already exists in the repo and is red. Milestone starts there.

---

## Steps

- [ ] **1. RED is already written.** Run `go test ./...`. It fails to compile:
  `undefined: forge.ErrNoRows`. Read `errors_test.go` and note what it demands —
  distinct sentinels that survive `%w` wrapping.

- [ ] **2. GREEN — `errors.go`.** Declare package `forge` and three sentinels
  with `errors.New`. Naming and messages:
  - `ErrNoRows` — `"forge: no rows in result set"`
  - `ErrTooManyRows` — `"forge: too many rows in result set"`
  - `ErrClosed` — `"forge: connection is closed"`
  Each gets a doc comment explaining *when a caller sees it*, not what it says.
  Why `errors.New` and not a struct: sentinels are identity, not data;
  `errors.Is` compares pointers.
  *Commit:* `feat: add core sentinel errors`

- [ ] **3. RED — structured error.** New test `TestQueryError` in `errors_test.go`:
  a `*forge.QueryError` must (a) render `forge: query "SELECT 1": <cause>`,
  (b) `errors.Is(err, forge.ErrNoRows)` when it wraps that sentinel,
  (c) be findable with `errors.As(err, &qe)` from three wrapping layers deep,
  (d) **never** include argument values in `Error()` — assert
  `!strings.Contains(err.Error(), "secret")` after constructing it with an
  argument `"secret"`.

- [ ] **4. GREEN — `QueryError`.** Struct fields: `SQL string`, `Args []any`
  (kept for programmatic inspection, never printed), `Err error`. Methods on
  pointer receiver: `Error() string` (uses `%q` on SQL, delegates to `e.Err`) and
  `Unwrap() error`. The redaction rule is the point of this milestone: the type
  *holds* the args, `Error()` *never* renders them.
  *Commit:* `feat: add QueryError with arg redaction`

- [ ] **5. RED — the interface contracts.** New file `forge_test.go`. Write
  compile-time assertions against a fake implemented in the test file:
  ```go
  var (
      _ forge.Driver = (*fakeDriver)(nil)
      _ forge.Conn   = (*fakeConn)(nil)
      // ... Rows, Result, Stmt, Tx
  )
  ```
  Then a behavioural test `TestRowsIteration` driving the fake: `Next()` returns
  true twice then false, `Err()` returns nil, `Close()` is idempotent (call it
  twice, expect no panic and no error). Writing the fake first is how you
  discover whether the interface is pleasant to implement.

- [ ] **6. GREEN — `forge.go`.** Define, all with `context.Context` as first
  parameter where blocking:
  - `Driver`: `Open(ctx context.Context, dsn string) (Conn, error)`
  - `Conn`: `Ping(ctx) error`; `Exec(ctx, sql string, args ...any) (Result, error)`;
    `Query(ctx, sql string, args ...any) (Rows, error)`;
    `Prepare(ctx, name, sql string) (Stmt, error)`;
    `Begin(ctx, opts *TxOptions) (Tx, error)`; `Close(ctx) error`
  - `Rows`: `Next() bool`; `Scan(dest ...any) error`; `Values() ([]any, error)`;
    `FieldDescriptions() []FieldDescription`; `Err() error`; `Close() error`
  - `Result`: `RowsAffected() int64`; `Command() string`
  - `Stmt`: `Query`/`Exec` (ctx + args), `Close(ctx) error`
  - `Tx`: `Commit(ctx) error`; `Rollback(ctx) error`; plus the query methods
  - `FieldDescription` struct: `Name string`, `TableOID uint32`,
    `AttributeNumber uint16`, `DataTypeOID uint32`, `DataTypeSize int16`,
    `TypeModifier int32`, `Format int16`
  - `TxOptions` struct: `IsoLevel IsoLevel`, `AccessMode AccessMode`,
    `DeferrableMode DeferrableMode` — each a `string`-based named type with
    exported constants (`Serializable`, `RepeatableRead`, `ReadCommitted`,
    `ReadUncommitted`; `ReadWrite`, `ReadOnly`; `Deferrable`, `NotDeferrable`).
  Design notes to think through before typing: `Rows.Next()` returns `bool` not
  `(bool, error)` because errors are collected and surfaced by `Err()` — same
  contract as `database/sql` and `bufio.Scanner`; that is a deliberate borrow, so
  no ADR needed. `Values()` is forge's addition over `driver.Rows` — it *is* a
  divergence, so it needs one.
  *Commit:* `feat: add core Driver/Conn/Rows/Result/Stmt/Tx interfaces`

- [ ] **7. RED — the layer rule, as a test.** `internal/architecture_test.go`:
  run `go list -deps -f '{{.ImportPath}}' github.com/quriny-platform/forge` via
  `os/exec`, assert every dependency is either stdlib (no dot in the first path
  segment) or inside the module. Skip with `testing.Short()`.

- [ ] **8. GREEN.** Nothing to implement — the root package is already
  stdlib-only. The test's job is to *stay* green as later milestones land. Extend
  it with a table: package under test → allowed import prefixes, seeded with the
  layer table from [ROADMAP.md](../ROADMAP.md).
  *Commit:* `test: enforce import direction with go list`

- [ ] **9. REFACTOR.** Move the sentinels' doc comments into a package-level
  `doc.go` overview describing what `forge` is and how the layers relate. Verify
  every exported identifier has a comment starting with its own name.
  *Commit:* `docs: add package overview`

- [ ] **10. ADR.** `docs/adr/0003-rows-values-and-field-descriptions.md` —
  why `Rows` exposes `Values()` and `FieldDescriptions()` (needed by `scan` at
  M12 for `RowToMap` without a second round trip), and why forge's `Conn` takes
  `...any` args rather than `driver.NamedValue`.
  *Commit:* `docs: ADR 0003 rows value access`

## Security (OWASP)

**Risks: [A04 Insecure Design](https://owasp.org/Top10/A04_2021-Insecure_Design/) · [A09 Logging Failures](https://owasp.org/Top10/A09_2021-Security_Logging_and_Monitoring_Failures/)**

The API shape decided here determines whether injection is *expressible* later.
This is the cheapest possible moment to get it right.

- [ ] **Design rule, written into `doc.go`:** no exported function in `forge`
  ever takes a pre-formatted SQL string built from values. Every query entry
  point is `(sql string, args ...any)`. There is no `Queryf`. There never will be.
- [ ] **`QueryError` holds args but never prints them.** Already in step 4 —
  treat it as the security control it is. Add the negative test explicitly:
  `TestQueryErrorRedactsArgs` constructs the error with `"hunter2"` and asserts
  `!strings.Contains(err.Error(), "hunter2")`.
- [ ] Give `QueryError` a `Redacted() string` used by loggers, and document that
  `Args` is for programmatic inspection only. See
  [Error Handling](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html).
- [ ] Sentinel errors must not leak schema detail. `ErrNoRows` says "no rows",
  not "no rows in table users where id=42".
- [ ] **Read and confirm ADR [0002 — Security boundaries](../adr/0002-security-boundaries.md)**
  against the API you just designed. It records the trust boundaries (TB1–TB4)
  and non-goals from [SECURITY-PRACTICES.md](../SECURITY-PRACTICES.md); ASVS V1
  treats the threat model as a deliverable. If your interfaces contradict it,
  one of the two is wrong — resolve it now, before anything implements them.

**Verify:** `FuzzQueryErrorRedaction` — for arbitrary arg strings, none appears
in `Error()`.

## Done when

- `go test ./...` green; fakes satisfy every interface at compile time.
- `errors_test.go` untouched and passing.
- No interface method blocks without a `context.Context`.
- ADR 0002 merged and indexed.

## Stretch

- Add `TestQueryErrorNoSecrets` as a fuzz target: any arg string must be absent
  from `Error()`.
- Sketch (in the ADR, not code) how `forge.Conn` would map onto
  `database/sql/driver.Connector` if you later ship a `database/sql` shim.
