# M09 — Cancellation, deadlines & connection health  `feat/cancel-context`

**Covers:** `context` propagation · goroutine lifecycle · deadline-based IO ·
leak detection · the difference between cancelling a *query* and closing a *socket*

**Links**
- [Go Concurrency Patterns: Context](https://go.dev/blog/context)
- [Learn Go with Tests — Context](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/context)
- [Protocol flow — Canceling requests in progress](https://www.postgresql.org/docs/current/protocol-flow.html#PROTOCOL-FLOW-CANCELING-REQUESTS)
- [`net.Conn.SetDeadline`](https://pkg.go.dev/net#Conn)
- [`testing/synctest`](https://pkg.go.dev/testing/synctest)

**Package:** `postgres/pgconn`.

---

## The two mechanisms

1. **`CancelRequest`** — open a *second* connection to the same server, send
   `{length: 16, code: 80877102, pid, secretKey}`, close it. The server cancels
   whatever that backend is running. It is advisory and racy by design: the query
   may finish first. The original connection stays usable.
2. **Deadline enforcement** — `conn.SetDeadline(time.Now())` unblocks a stuck
   `Read`, but the connection is then in an unknown protocol state and must be
   marked closed. This is the fallback when the cancel request does not land.

Getting this right means: on `ctx.Done()`, fire a cancel request, then wait a
short grace period for the server to reply with an `ErrorResponse` (SQLSTATE
`57014`, `query_canceled`). If it arrives, the connection is **still healthy** —
that is the whole point. If it does not, hard-close.

---

## Steps

- [ ] **1. RED — cancel request bytes.** Unit test in `pgwire` (if not already
  covered at M02): `CancelRequest{ProcessID: 42, SecretKey: 99}.Encode(nil)`
  produces exactly 16 bytes with the magic `80877102`.

- [ ] **2. RED — the watcher.** `cancel_test.go`: a scripted server that receives
  `Query` and then never replies. Call `Exec` with a context cancelled after
  10 ms. Assert: (a) `Close()` on the reader returns an error satisfying
  `errors.Is(err, context.Canceled)`; (b) a `CancelRequest` was observed on a
  second connection (have the harness record it); (c) no goroutine outlives the
  call — assert with a `runtime.NumGoroutine()` delta after a
  `time.Sleep`, or better, `testing/synctest`.

- [ ] **3. GREEN — `contextWatcher`.**
  ```go
  type contextWatcher struct {
      onCancel  func()
      onUnwatch func()
      // …
  }
  func (cw *contextWatcher) Watch(ctx context.Context)
  func (cw *contextWatcher) Unwatch()
  ```
  One goroutine per watched context, started in `Watch`, stopped in `Unwatch` via
  an internal channel. Every blocking `PgConn` method wraps its body in
  `Watch`/`defer Unwatch`. The rules that make this leak-free: `Unwatch` must be
  idempotent; `Watch` on an already-cancelled context calls `onCancel` inline
  rather than spawning; the goroutine selects on `ctx.Done()` and the stop channel
  only — never on anything that could block forever.
  *Commit:* `feat(pgconn): add context watcher`

- [ ] **4. GREEN — `CancelRequest`.**
  ```go
  func (c *PgConn) CancelRequest(ctx context.Context) error
  ```
  Dial a fresh connection using the same `Config` (including TLS), send the
  cancel frame, close. Never reuse `c.conn`. On failure, return the dial error —
  callers log it but do not treat it as fatal.
  *Commit:* `feat(pgconn): send cancel requests on a side connection`

- [ ] **5. RED — graceful vs hard cancel.** Two scripted cases:
  (a) after the cancel request, the server sends `ErrorResponse{Code: "57014"}`
  then `ReadyForQuery` → the connection status returns to `idle` and a subsequent
  `Ping` succeeds;
  (b) the server sends nothing → after `Config.CancelGracePeriod` (default ~5 s,
  overridable to 10 ms in tests) the socket is closed and `c.IsClosed()` is true.

- [ ] **6. GREEN — the grace path.** In `onCancel`: fire `CancelRequest`, then
  `c.conn.SetDeadline(time.Now().Add(gracePeriod))`. If the read loop returns a
  `57014` error before the deadline, clear the deadline and keep the connection.
  Otherwise close. Add `Config.CancelGracePeriod time.Duration`.
  *Commit:* `feat(pgconn): graceful query cancellation with fallback close`

- [ ] **7. RED/GREEN — deadlines everywhere.** Every exported method taking a
  `ctx` must respect a deadline set *before* the call (`ctx` already expired →
  immediate `context.DeadlineExceeded`, no IO at all) and one that expires
  mid-flight. Write this as a shared table test that runs the whole method set
  through both scenarios — a "contract test" over the API surface.
  *Commit:* `test(pgconn): context contract across the API surface`

- [ ] **8. RED/GREEN — health checks.** Add:
  ```go
  func (c *PgConn) IsClosed() bool
  func (c *PgConn) IsBusy() bool
  func (c *PgConn) Ping(ctx context.Context) error        // "-- ping" via Exec
  ```
  plus internal bookkeeping the pool needs at M11: `CreatedAt() time.Time`,
  `LastUsedAt() time.Time`. `Ping` must work on an idle connection and return
  `ErrConnBusy` on one mid-result.
  *Commit:* `feat(pgconn): connection health accessors`

- [ ] **9. Leak test as a first-class citizen.** Add `TestMain` in `pgconn` that
  snapshots goroutines before and after the package's tests and fails on a delta
  (a hand-rolled 30-line version of `go.uber.org/goleak`; no new dependency for
  30 lines). Then run the whole suite with `-race -count=5`.
  *Commit:* `test(pgconn): fail on leaked goroutines`

- [ ] **10. Integration.** `//go:build integration`: run
  `SELECT pg_sleep(30)`, cancel after 100 ms, assert the error is SQLSTATE
  `57014`, the call returned in well under a second, and the connection is
  reusable for a following `SELECT 1`.
  *Commit:* `test(pgconn): integration cancellation of pg_sleep`

## Security (OWASP)

**Risks: [DoS](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html) · [A02 Cryptographic Failures](https://owasp.org/Top10/A02_2021-Cryptographic_Failures/)**

Cancellation is a resource-exhaustion control. Get it wrong and a slow server
becomes an outage in every caller.

- [ ] **The cancel key is a secret.** `BackendKeyData` lets anyone who holds it
  cancel that backend's queries. Never log `pid`/`secretKey`, never include them
  in an error, and redact them in any `String()` method. Test it.
- [ ] **The cancel connection inherits the full TLS config.** Sending a cancel
  request in plaintext to a `verify-full` connection leaks the cancel key on the
  wire. Test that the side connection uses the same `TLSConfig`, not a bare dial.
- [ ] **Every blocking call must be interruptible.** A connection with no
  deadline and no watcher is a goroutine an attacker can pin. Audit the whole
  API surface: any method without a `ctx` path is a finding.
- [ ] **Bound the grace period.** `CancelGracePeriod` must have a sane default
  and a hard ceiling; an unbounded wait after cancellation is the DoS you were
  trying to prevent.
- [ ] **No goroutine leaks — this is a security property, not hygiene.** A leak
  per cancelled request is a slow-motion memory exhaustion. The `TestMain`
  goroutine check makes it a build failure.
- [ ] **Failure to cancel must close the connection**, never leave it in an
  unknown protocol state where a subsequent query could read another query's
  response bytes (a genuine cross-request data-disclosure path).

**Verify:** 10 000 cancelled queries in a loop leave goroutine count and heap
flat; `pid`/`secretKey` absent from all log output.

## Done when

- `go test -race -count=5 ./postgres/pgconn/...` green with zero goroutine delta.
- A cancelled query leaves the connection usable in the common case.
- No path calls `SetDeadline` without also marking the connection closed on timeout.

## Stretch

- Explore `testing/synctest` to make the timing tests instant and deterministic.
- Handle PostgreSQL 17's new cancel-key length (server versions negotiate longer
  keys); guard with a version check.
