# M15 — COPY protocol  `feat/copy-protocol`

**Covers:** streaming IO · backpressure · `io.Reader`/`io.Writer` composition ·
pprof profiling · bulk-load performance

**Links**
- [COPY](https://www.postgresql.org/docs/current/sql-copy.html)
- [Protocol flow — COPY operations](https://www.postgresql.org/docs/current/protocol-flow.html#PROTOCOL-COPY)
- [Binary COPY file format](https://www.postgresql.org/docs/current/sql-copy.html#id-1.9.3.55.9.4)
- [Profiling Go Programs](https://go.dev/blog/pprof)
- [Learn Go with Tests — IO & sorting](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/io)

**Package:** `postgres/pgconn`, surfaced through `postgres/` and `pool/`.

---

## Protocol shape

`COPY table FROM STDIN` → server replies `CopyInResponse` → client streams
`CopyData` frames → `CopyDone` → server sends `CommandComplete` + `ReadyForQuery`.
On failure the client sends `CopyFail{message}` instead.
`COPY … TO STDOUT` is the mirror: `CopyOutResponse`, then `CopyData` frames from
the server, then `CopyDone`.

Binary COPY payload (worth implementing — it is 2–5× faster than text):

| part | bytes |
|---|---|
| header | `PGCOPY\n\377\r\n\0` (11 bytes) + int32 flags (0) + int32 header extension length (0) |
| each tuple | int16 field count, then per field: int32 length (−1 = NULL) + bytes |
| trailer | int16 `-1` |

Field bytes are exactly the M08 binary encodings — this milestone is where that
work pays off a second time.

---

## Steps

- [ ] **1. RED — CopyFrom, text.** Scripted: `CopyFrom(ctx, reader, "COPY t FROM STDIN")`
  sends `Query`, expects `CopyInResponse`, streams the reader's bytes as
  `CopyData` frames, ends with `CopyDone`, returns the `CommandTag` with
  `RowsAffected`. Cases: an empty reader (still valid — header + trailer only);
  a reader returning an error mid-stream → `CopyFail` sent, error returned,
  connection still usable; a server `ErrorResponse` mid-copy → the client stops
  writing promptly rather than streaming a gigabyte into a dead socket.

- [ ] **2. GREEN — `CopyFrom`.**
  ```go
  func (c *PgConn) CopyFrom(ctx context.Context, r io.Reader, sql string) (CommandTag, error)
  func (c *PgConn) CopyTo(ctx context.Context, w io.Writer, sql string) (CommandTag, error)
  ```
  Chunk the reader into ~64 KiB `CopyData` frames (measure; do not guess the
  constant — benchmark 4 KiB / 64 KiB / 1 MiB and pick with data). The
  "stop promptly on server error" requirement means you cannot simply loop on
  `Read`: run the write loop while a second goroutine (or a non-blocking
  `Receive` with a zero deadline) watches for an `ErrorResponse`. Getting that
  concurrency right — with no leaked goroutine on either exit path — is the
  hard part of this milestone. Reuse the M09 `contextWatcher` discipline.
  *Commit:* `feat(pgconn): COPY FROM and COPY TO streaming`

- [ ] **3. RED — binary COPY encoding.** `copy_binary_test.go`: encode two rows
  of `(int4, text, timestamptz, bool)` and compare to a golden fixture generated
  from a real server (`COPY (SELECT …) TO STDOUT WITH (FORMAT binary)` piped to a
  file). Include a NULL and an empty string in the same column.

- [ ] **4. GREEN — `CopyFromRows` / binary writer.**
  ```go
  type CopyFromSource interface {
      Next() bool
      Values() ([]any, error)
      Err() error
  }
  func CopyFromRows(rows [][]any) CopyFromSource
  func CopyFromSlice(n int, fn func(i int) ([]any, error)) CopyFromSource
  func (c *Conn) CopyFrom(ctx context.Context, table string, cols []string, src CopyFromSource) (int64, error)
  ```
  (The last one on the `postgres` adapter, since it needs `pgtype`.) It builds
  the `COPY "table" ("c1","c2") FROM STDIN WITH (FORMAT binary)` statement —
  quoting identifiers via `qb`'s dialect, which is the moment the layering pays
  off — writes the header, encodes each tuple through the type map, and writes
  the trailer. Stream directly into the `CopyData` buffer; never build the whole
  payload in memory.
  *Commits:* `feat(pgconn): binary copy encoder`, `feat(postgres): CopyFrom convenience API`

- [ ] **5. RED — backpressure.** Feed a `CopyFromSource` of 1 000 000 rows into a
  mock server that reads slowly. Assert peak memory stays bounded (sample
  `runtime.ReadMemStats` and assert `HeapAlloc` under a threshold) — this proves
  you are streaming, not buffering. A test that would pass on a buffering
  implementation is not testing backpressure.

- [ ] **6. GREEN.** Fix whatever the test finds: a single reusable buffer, a
  bounded channel between producer and writer, and no `append` that grows without
  limit.
  *Commit:* `perf(pgconn): bounded memory during bulk copy`

- [ ] **7. RED/GREEN — `CopyTo`.** Server sends `CopyOutResponse` + N `CopyData`
  + `CopyDone`; the client writes payloads to the `io.Writer` in order and
  returns the tag. Test a writer that errors mid-stream: the client must drain
  the server's remaining frames to reach `ReadyForQuery`, then return the writer
  error. Draining is the difference between a usable connection and a poisoned one.
  *Commit:* `feat(pgconn): COPY TO with drain-on-writer-error`

- [ ] **8. Benchmarks + profile.** Integration benchmark, 100 000 rows:
  `BenchmarkInsertLoop` (one INSERT per row), `BenchmarkInsertBatch` (multi-row
  VALUES), `BenchmarkCopyText`, `BenchmarkCopyBinary`. Then profile the winner:
  ```bash
  go test -tags=integration -run=XXX -bench=BenchmarkCopyBinary -cpuprofile=cpu.out -memprofile=mem.out ./postgres
  go tool pprof -top cpu.out
  ```
  Publish the four numbers in the package doc. The profile, not the wall clock,
  justifies the chosen strategy.
  *Commit:* `perf: benchmark copy against insert strategies`

- [ ] **9. Pool integration.** `pool.Pool.CopyFrom` acquiring and releasing
  correctly, including on the error path. Add the copy tracer hook
  (`CopyFromTracer`) from M10.
  *Commit:* `feat(pool): CopyFrom passthrough`

## Security (OWASP)

**Risks: [A03 Injection](https://owasp.org/Top10/A03_2021-Injection/) · [DoS](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html)**

COPY is the one place forge builds SQL from caller-supplied identifiers. That
makes it the highest-risk API outside `qb`.

- [ ] **Table and column names go through `QuoteIdentifier`.** `CopyFrom(ctx,
  "users; DROP TABLE …", cols, src)` must produce one quoted identifier or an
  error. Add that exact payload to the test table.
- [ ] **`COPY … FROM` cannot take parameters** — there is no placeholder to hide
  behind. This is precisely why the quoting choke point from M13 is reused here
  rather than reimplemented. Do not write a second quoting function.
- [ ] **Never expose `COPY FROM PROGRAM` or `COPY FROM '/path'`.** Both execute
  or read on the *server*, and both are remote code execution / arbitrary file
  read if a caller can influence the string. forge builds `FROM STDIN` only, and
  a comment says why the others are absent.
- [ ] **Bounded memory is a security requirement.** A `CopyFromSource` feeding
  faster than the network drains must not grow the heap without limit; the M15
  backpressure test is the DoS control.
- [ ] **Cap incoming `CopyData` frames on `CopyTo`.** A hostile server can stream
  indefinitely; enforce the caller's context deadline and a maximum frame size.
- [ ] **Drain on error, always.** An aborted copy that leaves unread frames in
  the socket poisons the connection — the next borrower reads this copy's data
  (cross-request disclosure). Test the writer-error path explicitly.
- [ ] **`CopyFail` messages are attacker-visible on the server side.** Do not put
  row data or arguments into the failure message.
- [ ] **CSV parsing in the CLI (M16) is untrusted input** — a malformed file must
  error, not panic, and must not be able to inject extra columns into the
  binary tuple stream.

**Verify:** `TestCopyRejectsInjectedTableName`; the 1M-row bounded-memory test;
a writer-error copy leaves the connection usable for a following `SELECT 1`.

## Done when

- Binary COPY round-trips against a real server for every codec from M08.
- A 1M-row copy runs in bounded memory (asserted, not assumed).
- Copy benchmarks documented; binary beats text beats batched inserts.
- Errors on either side leave the connection usable.

## Stretch

- `CopyBoth` / logical replication start (`START_REPLICATION`) — the foundation
  for a CDC tool.
- A `CopyFromSource` adapter over `encoding/csv` for a `forge copy` CLI subcommand.
