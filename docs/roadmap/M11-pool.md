# M11 — Connection pool  `feat/pool`

**Covers:** semaphores & channels · `sync` primitives · resource lifecycles ·
race testing · benchmarking under contention

**Links**
- [Learn Go with Tests — Sync](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/sync)
- [Learn Go with Tests — Concurrency](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/concurrency)
- [`golang.org/x/sync/semaphore`](https://pkg.go.dev/golang.org/x/sync/semaphore) — read it, then write your own with a buffered channel
- [Go Memory Model](https://go.dev/ref/mem) · [Race Detector](https://go.dev/doc/articles/race_detector)
- [`database/sql` pool behaviour](https://pkg.go.dev/database/sql#DB.SetMaxOpenConns) — prior art worth comparing

**Package:** `pool/`. **Imports allowed:** `forge` only — never `postgres/*`.
The pool is engine-agnostic; it takes a constructor function.

---

## Design to hold in your head

A pool is three things: a **bounded resource count**, a **queue of waiters**, and
a **background reaper**. The classic Go shape is a buffered channel of idle
resources plus a semaphore for total count. Correctness hinges on:

- Never handing out a connection twice (obvious).
- Never *losing* a slot when a connection fails to construct or fails a health
  check (subtle — the leak that manifests as a mysterious hang under load).
- Making `Acquire` respect `ctx` while queued, and removing the waiter cleanly on
  cancellation so it does not receive a resource nobody will release.

---

## Steps

- [ ] **1. RED — bounds.** `pool_test.go`: `MaxConns=2`; acquire twice; a third
  `Acquire` blocks; releasing one unblocks it; a third `Acquire` with an
  already-cancelled context returns `context.Canceled` immediately without
  creating a connection (assert with a counting constructor).

- [ ] **2. GREEN — the core.**
  ```go
  type Config struct {
      MaxConns          int32          // default: max(4, NumCPU)
      MinConns          int32          // default 0
      MaxConnLifetime   time.Duration  // default 1h
      MaxConnLifetimeJitter time.Duration
      MaxConnIdleTime   time.Duration  // default 30m
      HealthCheckPeriod time.Duration  // default 1m
      NewConn        func(ctx context.Context) (forge.Conn, error)
      BeforeAcquire  func(ctx context.Context, c forge.Conn) bool
      AfterRelease   func(c forge.Conn) bool
      BeforeClose    func(c forge.Conn)
  }
  type Pool struct{ /* … */ }
  func New(ctx context.Context, cfg Config) (*Pool, error)
  func (p *Pool) Acquire(ctx context.Context) (*Conn, error)
  func (p *Pool) Close()
  type Conn struct{ /* wraps forge.Conn */ }
  func (c *Conn) Release()
  func (c *Conn) Hijack() forge.Conn   // remove from pool, caller owns it
  ```
  Internals: `idle chan *poolConn` (buffered to `MaxConns`), `sem chan struct{}`
  (also `MaxConns`), `mu sync.Mutex` guarding stats and the closed flag.
  `Acquire`: try a non-blocking receive from `idle`; else try to take a semaphore
  slot and construct; else block on `select { case c := <-idle: … case <-ctx.Done(): … }`.
  **Every early return after taking a semaphore slot must give it back** — use
  `defer` with a boolean, and write the test that fails if you forget.
  *Commits:* `feat(pool): bounded acquire and release`, `feat(pool): lifecycle hooks`

- [ ] **3. RED — release semantics.** A double `Release()` must panic with a clear
  message (programmer error, not a runtime condition) or be a no-op — pick one,
  document it, test it. Releasing a connection whose `AfterRelease` returns false
  destroys it and frees the slot. Releasing after `Close()` destroys rather than
  returning to the pool.

- [ ] **4. GREEN.** Track per-`Conn` state (`acquired`/`idle`/`destroyed`) with an
  explicit enum, not a bool. State machines beat boolean pairs every time.
  *Commit:* `feat(pool): connection state machine`

- [ ] **5. RED — reaping.** With `MaxConnIdleTime=50ms` and a controllable clock,
  an idle connection is closed by the background loop; with `MinConns=1`, the
  pool refills afterwards; `MaxConnLifetime` closes a *busy-then-released*
  connection at release time rather than mid-query. Inject the clock
  (`Config.clock func() time.Time`, unexported, set by tests) so nothing sleeps
  for real.

- [ ] **6. GREEN — background loop.** One goroutine started by `New`, stopped by
  `Close` via a context. It runs `HealthCheckPeriod` ticks doing: destroy expired
  idle conns, top up to `MinConns`, and refresh stats. `Close` must wait for it
  (`sync.WaitGroup`) so tests see no leaked goroutine.
  *Commit:* `feat(pool): background health check and reaper`

- [ ] **7. RED — concurrency, for real.** `TestPoolConcurrent`: 100 goroutines ×
  500 acquire/query/release cycles against fakes, run with `-race`. Assert:
  no race, `Stat().AcquiredConns() == 0` at the end, `TotalConns() <= MaxConns`
  at all times (sample it from a watcher goroutine), and no goroutine leak.
  This test is the milestone. Expect it to find at least one real bug.

- [ ] **8. GREEN — fix what it finds.** Typical culprits: stats updated outside
  the mutex; a waiter cancelled between `select` arms losing a resource; the
  reaper destroying a connection another goroutine just took from `idle`.
  *Commit:* `fix(pool): resolve races found under concurrent acquire`

- [ ] **9. RED/GREEN — convenience API.** Mirror the surface Quriny needs
  (M14 depends on this being exact):
  ```go
  func (p *Pool) Query(ctx context.Context, sql string, args ...any) (forge.Rows, error)
  func (p *Pool) Exec(ctx context.Context, sql string, args ...any) (forge.Result, error)
  func (p *Pool) QueryRow(ctx context.Context, sql string, args ...any) forge.Row
  func (p *Pool) Begin(ctx context.Context) (forge.Tx, error)
  func (p *Pool) Ping(ctx context.Context) error
  ```
  Each acquires, runs, and releases — except `Query`, which must hold the
  connection until `Rows.Close()`. Getting that ownership transfer right (and
  releasing on `Rows.Close`, including the error path) is the trap; write the
  test that acquires, abandons the rows, and asserts the pool eventually recovers
  the connection (or documents that it does not — `database/sql` has the same
  hazard).
  *Commit:* `feat(pool): query helpers with connection ownership`

- [ ] **10. Stats & benchmarks.**
  ```go
  type Stat struct{ /* AcquireCount, AcquireDuration, AcquiredConns,
      CanceledAcquireCount, ConstructingConns, EmptyAcquireCount,
      IdleConns, MaxConns, TotalConns, NewConnsCount, MaxLifetimeDestroyCount,
      MaxIdleDestroyCount */ }
  func (p *Pool) Stat() *Stat
  ```
  Use `sync/atomic` counters, not the mutex, for the hot ones.
  `BenchmarkAcquireRelease` at 1/8/64 goroutines; record ns/op and allocs/op in
  the PR. Compare against `pgxpool` if you have it handy — being slower is fine,
  not knowing by how much is not.
  *Commit:* `feat(pool): statistics and acquire benchmarks`

## Security (OWASP)

**Risks: [DoS](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html) · [A01 Broken Access Control](https://owasp.org/Top10/A01_2021-Broken_Access_Control/)**

A pool is a shared resource and a shared *state* container. Both are security
surfaces.

- [ ] **Session state must not leak between borrowers.** A connection returned to
  the pool after `SET ROLE`, `SET search_path`, a prepared statement, a temp
  table, or an open transaction carries that state to the next caller — a real
  cross-tenant access-control bug (A01). Decide the policy and test it:
  `AfterRelease` issues `DISCARD ALL` / `RESET ALL`, or the connection is
  destroyed. Document which.
- [ ] **A connection in a failed transaction is never reused.** Check `TxStatus`
  at release; `'E'` means destroy, not return.
- [ ] **Exhaustion returns an error, never an unbounded wait.** `MaxConns`
  reached plus a caller with no context deadline is a hang. Document that callers
  must set deadlines, and consider a `MaxWaitDuration` guard.
- [ ] **Slot accounting is a DoS control.** A leaked semaphore slot on an error
  path permanently shrinks the pool — the failure mode is a gradual outage under
  partial errors. The concurrency test from step 7 is what catches it.
- [ ] **Do not log the DSN or config on connection creation failure.** The pool
  logs the loudest and most often.
- [ ] **`Stat()` must not expose credentials** and should be safe to publish to
  a metrics endpoint. Review every field with that in mind.
- [ ] **`MinConns` warm-up is an amplification risk** — a pool that aggressively
  reconnects against a down database becomes a thundering herd. Add backoff, and
  test that N failed constructions do not produce N immediate retries.

**Verify:** `TestNoSessionStateLeak` — set a session variable on a borrowed
connection, release, re-acquire until you get the same one, assert it is clean.

## Done when

- `go test -race -count=10 ./pool/...` green.
- `Stat()` after a full test run shows zero acquired, zero constructing.
- `pool` imports only `forge` + stdlib (the M01 architecture test proves it).

## Stretch

- Multi-host read replica routing with a pluggable selection policy.
- Warm-up on `New` when `MinConns > 0`, with the first-connect error surfaced
  rather than swallowed.
