# M07 — Extended query protocol  `feat/extended-query`

**Covers:** prepared statements · portals · pipelining · LRU caching ·
parameterised queries as the injection defence

**Links**
- [Message Flow — Extended Query](https://www.postgresql.org/docs/current/protocol-flow.html#PROTOCOL-FLOW-EXTENDED-QUERY)
- [PREPARE](https://www.postgresql.org/docs/current/sql-prepare.html)
- [`container/list`](https://pkg.go.dev/container/list) — for the LRU
- [Learn Go with Tests — Maps](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/maps)

**Packages:** `postgres/pgconn`, `internal/stmtcache`.

---

## The exchange

```
C: Parse   (statement name, SQL with $1…$n, optional param OIDs)
C: Describe('S', name)
C: Sync
S: ParseComplete, ParameterDescription, RowDescription|NoData, ReadyForQuery
--- statement now exists on the server ---
C: Bind    (portal name, statement name, param formats, param values, result formats)
C: Execute (portal, maxRows=0 for all)
C: Sync
S: BindComplete, DataRow*, CommandComplete, ReadyForQuery
```

Key facts to encode in tests: an unnamed statement (`""`) is replaced by the next
`Parse`; an error anywhere puts the connection in *failed* state where every
subsequent message is ignored **until `Sync`** — so `Sync` is not optional, it is
the recovery mechanism. `Execute` with `MaxRows > 0` yields `PortalSuspended`
instead of `CommandComplete`, and the portal can be executed again.

---

## Steps

- [ ] **1. RED — Prepare.** Scripted test: `Prepare(ctx, "s1", "SELECT $1::int")`
  sends `Parse`+`Describe`+`Sync`, and returns a `*StatementDescription` with
  `ParamOIDs == []uint32{23}` and one field named `int4`. Error case: server
  returns `ErrorResponse` `42601` (syntax error) → `Prepare` returns `*PgError`
  and the conn is idle again (assert a following `Ping` works — that proves the
  `Sync` recovery).

- [ ] **2. GREEN — `Prepare`.**
  ```go
  type StatementDescription struct {
      Name      string
      SQL       string
      ParamOIDs []uint32
      Fields    []pgwire.FieldDescription
  }
  func (c *PgConn) Prepare(ctx context.Context, name, sql string, paramOIDs []uint32) (*StatementDescription, error)
  func (c *PgConn) Deallocate(ctx context.Context, name string) error
  ```
  Send all three messages, then drain to `ReadyForQuery` collecting the
  description. Always drain — even on error — or the next query reads stale bytes.
  *Commit:* `feat(pgconn): prepare and describe statements`

- [ ] **3. RED — ExecParams.** `ExecParams(ctx, "SELECT $1::text", [][]byte{[]byte("hi")}, paramOIDs, paramFormats, resultFormats)` returns a
  `*ResultReader` yielding one row. Cases: NULL parameter (`nil` element → wire
  length `-1`); zero parameters; more parameters than the statement declares →
  server error surfaced cleanly; result format codes honoured (all-text vs
  all-binary vs per-column).

- [ ] **4. GREEN — `ExecParams` / `ExecPrepared`.**
  ```go
  func (c *PgConn) ExecParams(ctx context.Context, sql string, paramValues [][]byte,
      paramOIDs []uint32, paramFormats []int16, resultFormats []int16) *ResultReader
  func (c *PgConn) ExecPrepared(ctx context.Context, stmtName string, paramValues [][]byte,
      paramFormats []int16, resultFormats []int16) *ResultReader
  ```
  `ExecParams` = Parse(unnamed) + Bind + Describe(portal) + Execute + Sync in one
  write, then one flush — that single flush is the pipelining win; measure it in
  a benchmark against three separate flushes.
  Format-code slices follow the protocol's shorthand: length 0 = all text,
  length 1 = apply to every column, length n = per column. Implement and test all three.
  *Commit:* `feat(pgconn): extended-query execution`

- [ ] **5. RED — the statement cache.** `internal/stmtcache/lru_test.go`:
  capacity 2 → inserting a third evicts the least recently used; `Get` promotes;
  `Invalidate` removes one; `InvalidateAll` clears; eviction returns the name so
  the caller can send `Close`. Include a concurrency test under `-race` if the
  cache is to be shared (decide: forge's cache is per-connection, so it needs no
  lock — write the test that documents that decision instead).

- [ ] **6. GREEN — `internal/stmtcache`.**
  ```go
  type Cache interface {
      Get(sql string) *StatementDescription
      Put(sd *StatementDescription)
      Invalidate(sql string)
      InvalidateAll()
      HandleInvalidated() []*StatementDescription
      Len() int
      Cap() int
  }
  func NewLRU(cap int) Cache
  ```
  `container/list` + `map[string]*list.Element` is the classic shape; write it by
  hand rather than pulling a dependency. Statement names must be unique per
  connection — generate `stmtcache_<counter>` values, never a hash of the SQL
  (collisions across schema changes are a real production incident class).
  *Commit:* `feat(stmtcache): LRU prepared statement cache`

- [ ] **7. RED — cache integration + invalidation.** Scripted: the same SQL run
  twice sends `Parse` only once. Then: server returns SQLSTATE `26000`
  (invalid_sql_statement_name) or `0A000`/`42P05` after a schema change → forge
  invalidates the entry and retries once, transparently. Assert the retry happens
  exactly once, not in a loop.

- [ ] **8. GREEN — cached exec path.** Add `Config.StatementCacheCapacity` (default
  512, `0` disables) and a `QueryExecMode` enum: `QueryExecModeCacheStatement`
  (default), `QueryExecModeCacheDescribe`, `QueryExecModeDescribeExec`,
  `QueryExecModeExec`, `QueryExecModeSimpleProtocol`. Document what each costs in
  round trips — users on PgBouncer in transaction-pooling mode need
  `QueryExecModeExec` or `SimpleProtocol`, and that is exactly the kind of
  operational knowledge worth writing down.
  *Commit:* `feat(pgconn): statement caching with query exec modes`

- [ ] **9. RED/GREEN — transactions.** `Begin`/`Commit`/`Rollback` on `PgConn`
  built from `Exec("BEGIN")` etc., plus `TxOptions` rendering
  (`BEGIN ISOLATION LEVEL SERIALIZABLE READ ONLY DEFERRABLE`). Assert the
  rendered SQL exactly, and that `Rollback` on an already-closed conn returns
  `ErrTxClosed` rather than panicking. `TxStatus()` reads the byte from the last
  `ReadyForQuery`.
  *Commit:* `feat(pgconn): transaction control`

- [ ] **10. REFACTOR + benchmark.** `BenchmarkExecPreparedCached` vs
  `BenchmarkExecParams`: quantify the saved round trip against the mock. Put the
  numbers in the PR description — that habit is worth more than the numbers.

## Security (OWASP)

**Risks: [A03 Injection](https://owasp.org/Top10/A03_2021-Injection/) · [A01 Broken Access Control](https://owasp.org/Top10/A01_2021-Broken_Access_Control/)**

The extended protocol *is* the injection defence. References:
[Query Parameterization](https://cheatsheetseries.owasp.org/cheatsheets/Query_Parameterization_Cheat_Sheet.html) ·
[SQL Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html). ASVS V5.

- [ ] **Arguments travel in `Bind`, always.** Write the test that proves it:
  `ExecParams(ctx, "SELECT $1", [][]byte("'; DROP TABLE users; --"))` sends the
  payload as a parameter and returns it verbatim as a value. Capture the wire
  bytes and assert the payload never appears in the `Parse` message.
- [ ] **`QueryExecModeSimpleProtocol` is the dangerous mode — say so loudly.**
  Values are interpolated into SQL text there. Its doc comment must state that
  it is unsafe with untrusted input, and it must never be the default. Consider
  requiring `Config.AllowUnsafeSimpleProtocol` to enable it.
  This is the highest-risk API in the whole project.
- [ ] **Transaction options render from a closed set (A01).** `TxOptions` fields
  are typed constants; `ToSQL` switches over them and errors on an unknown value.
  An `IsoLevel("READ COMMITTED; DROP …")` must be impossible to construct
  meaningfully and must error if it is.
  Same rule for any future `SET ROLE` / `SET search_path` helper: quoted
  identifier or nothing.
- [ ] **Statement names are generated, never derived from user input.**
  `stmtcache_<counter>`; no hashing of SQL, no caller-supplied names reaching the
  cache key. Collisions across schema changes are a correctness *and* an access
  control bug.
- [ ] **Bound the cache.** `StatementCacheCapacity` is a hard limit; an
  application generating unique SQL per request must not grow server-side
  memory without bound (DoS on the *server*, which is worse than on the client).
- [ ] **Retry exactly once** on cache invalidation. An unbounded retry loop
  against a failing server is a self-inflicted DoS. Test the retry count.

**Verify:** a wire-level test asserting no argument byte ever appears inside a
`Parse` message body.

## Done when

- Same SQL twice = one `Parse` (asserted, not assumed).
- Every error path still ends at `ReadyForQuery`; a follow-up query succeeds.
- Cache eviction sends `Close` for the evicted statement.

## Stretch

- Full pipelining: `SendBatch` queueing N statements before one `Sync`, with
  per-statement results and correct error attribution.
- `docs/adr/0008-query-exec-modes.md` on why forge exposes modes rather than
  auto-detecting PgBouncer.
