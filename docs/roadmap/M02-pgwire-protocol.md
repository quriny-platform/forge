# M02 — PostgreSQL wire protocol v3  `feat/pgwire-protocol`

**Covers:** binary framing · `encoding/binary` · `io.Reader`/`io.Writer` ·
fuzzing · golden files · buffer reuse

**Links**
- [Message Formats](https://www.postgresql.org/docs/current/protocol-message-formats.html) — the spec you implement, keep it open
- [Protocol overview](https://www.postgresql.org/docs/current/protocol.html)
- [Error and Notice fields](https://www.postgresql.org/docs/current/protocol-error-fields.html)
- [`encoding/binary`](https://pkg.go.dev/encoding/binary)
- [Go fuzzing tutorial](https://go.dev/doc/tutorial/fuzz)
- [Learn Go with Tests — Reading files / IO](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/reading-files)

**Package:** `postgres/pgwire`. **Imports allowed:** stdlib only.

> The single most important milestone. Everything above it is a client of this
> package. Take your time; get the framing exactly right.

---

## Background you must internalise first

Every v3 message except the startup family is framed identically:

| offset | width | meaning |
|---|---|---|
| 0 | 1 byte | message type, an ASCII letter (`R`, `T`, `D`, `C`, `Z`, `E`, …) |
| 1 | 4 bytes | length, big-endian **int32, including these 4 bytes but not the type byte** |
| 5 | length-4 | payload |

The startup message, `SSLRequest` and `CancelRequest` have **no type byte** —
they are length-prefixed only, and their first payload int32 is a version or
magic code (`196608` = protocol 3.0, `80877103` = SSL, `80877102` = cancel).
That asymmetry is the first bug everyone writes. Encode it in a test.

Payload primitives: `Int16`/`Int32` big-endian, `String` = NUL-terminated UTF-8,
`Byte1` = one byte, `ByteN` = raw bytes with a preceding length where the message
defines one. A field length of `-1` means SQL NULL and carries no bytes.

---

## Steps

- [ ] **1. RED — the primitive buffer.** `buf_test.go`, table-driven:
  writing then reading back `int16`, `int32`, string, raw bytes yields identical
  values; a truncated buffer returns `io.ErrUnexpectedEOF`; a string with no NUL
  terminator errors rather than reading past the end.

- [ ] **2. GREEN — `buf.go`, unexported.** Two helpers:
  - `type writeBuf struct { b []byte }` with methods `int16(int16)`,
    `int32(int32)`, `string(string)` (appends bytes then `0x00`), `bytes([]byte)`,
    and `startMsg(typ byte)` / `finishMsg()` which reserve four bytes for the
    length and back-patch them via `binary.BigEndian.PutUint32`.
  - `type readBuf []byte` with `int16() int16`, `int32() int32`,
    `string() (string, error)`, `next(n int) ([]byte, error)`. It advances
    itself by reslicing (`*b = (*b)[n:]`), which is why it's a pointer receiver
    on a slice type.
  Use `append` and let the slice grow; do **not** reach for `bytes.Buffer` —
  the buffer stays allocation-transparent. Bounds-check before every read.
  *Commit:* `feat(pgwire): add read/write buffer primitives`

- [ ] **3. RED — frontend messages round-trip.** `frontend_test.go`, one table
  covering every message you are about to define. Each case: build the struct →
  `Encode(nil)` → assert the first byte, the big-endian length, and a golden hex
  dump of the whole frame (store dumps in `testdata/frontend/<name>.hex` and
  compare with a `-update` flag guarded by `flag.Bool`).

- [ ] **4. GREEN — `frontend.go`.** Interface:
  ```go
  type FrontendMessage interface {
      Encode(dst []byte) []byte  // appends the full framed message
      frontend()                 // unexported marker, prevents outside impls
  }
  ```
  Types to define, with their payload fields:
  - `StartupMessage{ProtocolVersion uint32, Parameters map[string]string}` —
    no type byte; key/value strings, terminated by a lone `0x00`. **Sort the
    keys** before encoding or golden files go flaky on map iteration order.
  - `SSLRequest{}` · `CancelRequest{ProcessID, SecretKey uint32}`
  - `Query{String string}` — `'Q'`
  - `Parse{Name, Query string, ParameterOIDs []uint32}` — `'P'`
  - `Bind{DestinationPortal, PreparedStatement string, ParameterFormatCodes []int16,
    Parameters [][]byte, ResultFormatCodes []int16}` — `'B'`; nil element =
    length `-1` = NULL
  - `Describe{ObjectType byte, Name string}` — `'D'`, ObjectType `'S'` or `'P'`
  - `Execute{Portal string, MaxRows uint32}` — `'E'`
  - `Sync{}` `'S'` · `Flush{}` `'H'` · `Terminate{}` `'X'`
  - `PasswordMessage{Password string}` `'p'` · `SASLInitialResponse{AuthMechanism string, Data []byte}` `'p'` ·
    `SASLResponse{Data []byte}` `'p'` (note: three distinct types share one tag —
    disambiguated by conversation state, not by the byte)
  - `CopyData{Data []byte}` `'d'` · `CopyDone{}` `'c'` · `CopyFail{Message string}` `'f'`
  *Commits:* one per message group — `feat(pgwire): startup and ssl messages`,
  `feat(pgwire): simple query messages`, `feat(pgwire): extended query messages`, …

- [ ] **5. RED — backend decoding.** `backend_test.go`: feed byte slices captured
  from the spec, assert the decoded struct. Must include the nasty cases:
  `DataRow` with a `-1` column (NULL) and a zero-length column (empty string —
  *not* the same thing); `ErrorResponse` with unknown field codes (must be
  preserved, not dropped); `RowDescription` with zero fields; a message whose
  declared length exceeds the buffer.

- [ ] **6. GREEN — `backend.go`.** Interface mirrors the frontend:
  `Decode(src []byte) error` plus a `backend()` marker. Types:
  `AuthenticationOk`, `AuthenticationSASL{AuthMechanisms []string}`,
  `AuthenticationSASLContinue{Data []byte}`, `AuthenticationSASLFinal{Data []byte}`,
  `AuthenticationMD5Password{Salt [4]byte}`, `AuthenticationCleartextPassword`
  (all share tag `'R'`, discriminated by the first payload int32 —
  0/3/5/10/11/12), `BackendKeyData{ProcessID, SecretKey uint32}` `'K'`,
  `ParameterStatus{Name, Value string}` `'S'`, `ReadyForQuery{TxStatus byte}` `'Z'`
  (`'I'` idle, `'T'` in transaction, `'E'` failed transaction),
  `RowDescription{Fields []FieldDescription}` `'T'`, `DataRow{Values [][]byte}` `'D'`,
  `CommandComplete{CommandTag []byte}` `'C'`, `EmptyQueryResponse` `'I'`,
  `ErrorResponse` / `NoticeResponse` `'E'`/`'N'`, `ParseComplete` `'1'`,
  `BindComplete` `'2'`, `CloseComplete` `'3'`, `PortalSuspended` `'s'`,
  `ParameterDescription{ParameterOIDs []uint32}` `'t'`, `NoData` `'n'`,
  `CopyInResponse`/`CopyOutResponse`/`CopyBothResponse`, `CopyData`, `CopyDone`,
  `NotificationResponse{PID uint32, Channel, Payload string}` `'A'`.
  `ErrorResponse` deserves real fields — `Severity`, `Code` (SQLSTATE), `Message`,
  `Detail`, `Hint`, `Position`, `SchemaName`, `TableName`, `ConstraintName`, …
  plus `UnknownFields map[byte]string` — and an `Error() string` method so it can
  be returned directly. **Decode must copy** any `[]byte` it keeps: the caller
  owns and reuses the source buffer.
  *Commits:* `feat(pgwire): decode authentication messages`, `…query results`, `…copy messages`

- [ ] **7. RED — the chunker.** `chunkreader_test.go`: given a `net.Conn`-like
  reader that returns one byte at a time, the reader still yields whole messages;
  given a reader that returns a message split across three `Read` calls, same;
  given a length under 4, it returns a protocol error, not a panic.

- [ ] **8. GREEN — `chunkreader.go` + `Frontend`/`Backend` drivers.**
  - `type ChunkReader struct` wrapping an `io.Reader` with an internal buffer;
    `Next(n int) ([]byte, error)` returns a slice valid until the next call.
  - `type Frontend struct` (client side): `Send(FrontendMessage) error`,
    `Flush() error`, `Receive() (BackendMessage, error)`. `Receive` reads the type
    byte + length, dispatches on the type byte into a reused message struct per
    type (that reuse is why `Decode` must copy), and returns it.
  - Guard against absurd lengths: reject anything over, say, 20 MB with a typed
    error rather than allocating.
  *Commit:* `feat(pgwire): add chunk reader and Frontend driver`

- [ ] **9. RED/GREEN — fuzzing.** `fuzz_test.go`: `FuzzBackendDecode` seeds the
  corpus with your golden dumps, then asserts *no panic and no unbounded
  allocation* for arbitrary input; `FuzzFrontendRoundTrip` encodes a message,
  decodes it with a mirror-image backend decoder, and asserts equality.
  Run `go test -fuzz=FuzzBackendDecode -fuzztime=2m ./postgres/pgwire`.
  Commit any crashers found under `testdata/fuzz/` along with the fix.
  *Commit:* `test(pgwire): fuzz backend decoding`

- [ ] **10. REFACTOR.** Extract the repeated “read null-terminated strings until
  a lone NUL” loop. Add `String() string` methods to the message types for
  debugging — and make sure `PasswordMessage.String()` prints `[redacted]`.
  Benchmark `BenchmarkDecodeDataRow` and cut allocations to zero per message by
  reusing the per-type structs.
  *Commit:* `refactor(pgwire): reduce allocations in hot decode path`

## Security (OWASP)

**Risks: [A08 Software & Data Integrity Failures](https://owasp.org/Top10/A08_2021-Software_and_Data_Integrity_Failures/) · [DoS](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html)**

Every byte decoded here is attacker-controlled if the server is hostile or
MITM'd. This package is forge's memory-safety boundary (TB2).

- [ ] **Cap declared lengths before allocating.** A `DataRow` claiming
  `math.MaxInt32` columns, or a message header claiming 4 GB, must return a
  typed error — never `make([]byte, n)`. Define `maxMessageSize = 20 << 20` and
  `maxColumns = 1 << 16`, both configurable, both enforced.
  Test: `TestRejectOversizedMessage`, `TestRejectAbsurdColumnCount`.
- [ ] **Never trust a length to be non-negative.** `int32(-1)` is legal as a NULL
  marker in *field* positions and illegal everywhere else. A signed length used
  as a slice bound is a classic overflow; assert on each path.
- [ ] **Bounds-check every `readBuf` read.** No `b[0:4]` without first checking
  `len(b) >= 4`. Return `io.ErrUnexpectedEOF`, do not panic — a panic in a
  library is a DoS in the caller.
- [ ] **`Decode` copies what it retains.** Handing the caller a slice that aliases
  the reused read buffer is both a correctness bug and a data-disclosure bug
  (one query's bytes surfacing in another's result).
- [ ] **Redaction at the type level:** `PasswordMessage.String()`,
  `SASLInitialResponse.String()` and `SASLResponse.String()` render
  `[redacted]`. Test each. A `%v` on a message struct must never print a secret.
- [ ] Fuzzing is a security requirement here, not a stretch goal:
  `FuzzBackendDecode` must survive arbitrary input with no panic and no
  allocation proportional to an unvalidated field. Run it 10 minutes before
  merging and commit the corpus.

**Verify:** `go test -fuzz=FuzzBackendDecode -fuzztime=10m ./postgres/pgwire`
clean; a hand-written 4 GB-length header errors in microseconds without
memory growth.

## Done when

- Every message in the spec's Message Formats page is either implemented or
  listed with a `// TODO(M15): COPY both` style note.
- `go test -race ./postgres/pgwire/...` green; `go test -fuzz` runs 2 minutes clean.
- `BenchmarkDecodeDataRow` reports 0 allocs/op.
- Golden files committed under `testdata/`.

## Stretch

- Capture a real conversation: `tcpdump`/Wireshark against `psql`, then replay
  the bytes through your decoder as an extra golden test.
- Write `docs/adr/0004-message-struct-reuse.md` on the copy-on-decode contract.
