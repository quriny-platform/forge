# M06 — Type system & text codecs  `feat/pgtype-text`

**Covers:** type systems · generics · interface satisfaction at runtime ·
NULL modelling · `database/sql` interop

**Links**
- [`pg_type` catalog](https://www.postgresql.org/docs/current/catalog-pg-type.html) — where OIDs come from
- [Data types](https://www.postgresql.org/docs/current/datatype.html)
- [Tutorial: Generics](https://go.dev/doc/tutorial/generics)
- [Learn Go with Tests — Generics](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/generics)
- [`sql.Scanner` / `driver.Valuer`](https://pkg.go.dev/database/sql#Scanner)
- [`sql.Null[T]`](https://pkg.go.dev/database/sql#Null)

**Package:** `postgres/pgtype`. **Imports allowed:** stdlib only.

> Find OIDs with `SELECT oid, typname FROM pg_type WHERE typname IN (...)` or
> read `include/catalog/pg_type.dat` in the PostgreSQL source. Hard-code the
> built-ins as constants; look up user types at runtime (M08 stretch).

---

## Steps

- [ ] **1. RED — OID constants.** `pgtype_test.go`: assert
  `pgtype.BoolOID == 16`, `Int2OID == 21`, `Int4OID == 23`, `Int8OID == 20`,
  `Float4OID == 700`, `Float8OID == 701`, `TextOID == 25`, `VarcharOID == 1043`,
  `ByteaOID == 17`, `DateOID == 1082`, `TimestampOID == 1114`,
  `TimestamptzOID == 1184`, `UUIDOID == 2950`, `JSONOID == 114`, `JSONBOID == 3802`,
  `NumericOID == 1700`, `BoolArrayOID == 1000`, `Int4ArrayOID == 1007`,
  `TextArrayOID == 1009`. A dumb test, but it catches transcription errors that
  would otherwise surface as baffling decode failures at M10.

- [ ] **2. GREEN — `oid.go`.** A `const (…)` block of `uint32` values with a
  comment pointing at the catalog page.
  *Commit:* `feat(pgtype): add built-in type OIDs`

- [ ] **3. RED — the Codec contract.** `codec_test.go`: for each built-in, a
  round-trip table — Go value → text encoding → bytes match the expected
  PostgreSQL literal (`true` → `t`, `int32(-5)` → `-5`, `time.Time` →
  `2026-09-06 10:11:12.5+07`) — and back. Separately: encoding a nil Go value
  must produce a *NULL marker*, not the string `"<nil>"`; decoding a NULL into a
  non-pointer must return a typed error naming the destination.

- [ ] **4. GREEN — `codec.go`.**
  ```go
  type Format int16
  const (TextFormat Format = 0; BinaryFormat Format = 1)

  type Codec interface {
      OID() uint32
      PreferredFormat() Format
      // Encode appends the wire representation of value to dst.
      // A nil return with nil error means SQL NULL.
      Encode(value any, format Format, dst []byte) ([]byte, error)
      // Decode assigns src (nil == SQL NULL) into dest, which is a pointer.
      Decode(src []byte, format Format, dest any) error
  }
  ```
  Implement text format only in this milestone; `Encode`/`Decode` return
  `ErrUnsupportedFormat` for `BinaryFormat` so M08 has an obvious hole to fill.
  Concrete codecs, one file each: `codec_bool.go`, `codec_int.go` (generic over
  the integer widths), `codec_float.go`, `codec_text.go`, `codec_bytea.go`
  (`\x` hex form), `codec_time.go`, `codec_uuid.go`, `codec_json.go`,
  `codec_numeric.go`.
  Use `strconv.AppendInt`, `AppendFloat`, `time.AppendFormat` — allocating a
  string then converting is the slow path and you will feel it at M11's benchmarks.
  *Commits:* one per codec family.

- [ ] **5. RED — NULL and the "null-able" wrappers.** Assert that
  `pgtype.Null[int32]{}` (or your chosen shape) encodes to NULL and decodes from
  NULL, and that decoding NULL into `*int32` sets the pointer to nil rather than
  erroring.

- [ ] **6. GREEN — nullable types.** Two mechanisms, both needed:
  - Pointer destinations: `*T` — nil means NULL.
  - A generic wrapper for value contexts:
    ```go
    type Null[T any] struct { V T; Valid bool }
    ```
    with `Scan(any) error` and `Value() (driver.Value, error)`… except
    `driver.Value` would import `database/sql/driver`, which is stdlib, so it is
    allowed — decide and record it. (`database/sql` interop is a feature, not a
    layer violation; the rule is “no upward imports inside forge”, and
    `database/sql/driver` is stdlib.)
  *Commit:* `feat(pgtype): nullable value wrapper`

- [ ] **7. RED — the registry.** `registry_test.go`: lookup by OID and by Go
  type; registering a duplicate OID replaces or errors (pick and test one);
  a default registry contains every built-in; a `Map` derived from the default
  is independent (mutating the copy does not affect the default — test this,
  because shared mutable global state is the bug this design prevents).

- [ ] **8. GREEN — `registry.go`.**
  ```go
  type Map struct {
      oidToCodec  map[uint32]Codec
      typeToCodec map[reflect.Type]Codec
  }
  func NewMap() *Map                       // pre-populated with built-ins
  func (m *Map) RegisterType(c Codec) error
  func (m *Map) CodecForOID(oid uint32) (Codec, bool)
  func (m *Map) CodecForValue(v any) (Codec, bool)
  func (m *Map) Encode(oid uint32, format Format, v any, dst []byte) ([]byte, error)
  func (m *Map) Scan(oid uint32, format Format, src []byte, dest any) error
  ```
  `Map` is per-connection (each `PgConn` owns one), which is why `NewMap` copies
  rather than sharing. Fast paths first: before touching `reflect`, switch on the
  concrete common types (`string`, `int32`, `int64`, `bool`, `[]byte`,
  `time.Time`) in `Encode`/`Scan`. Reflection is the fallback, not the default.
  *Commit:* `feat(pgtype): per-connection type map`

- [ ] **9. RED/GREEN — `sql.Scanner` / `driver.Valuer` support.** If `dest`
  implements `sql.Scanner`, call it; if `value` implements `driver.Valuer`, call
  `Value()` first. Test with a user-defined type in the test file. This is what
  makes third-party types (`uuid.UUID`, `decimal.Decimal`) work without forge
  knowing about them.
  *Commit:* `feat(pgtype): honour sql.Scanner and driver.Valuer`

- [ ] **10. REFACTOR.** Extract the “try concrete type, then Valuer, then
  reflect” ladder into one helper. Benchmark `BenchmarkEncodeInt4` and
  `BenchmarkScanText`; aim for 0 allocs on the fast path.
  *Commit:* `refactor(pgtype): unify value resolution ladder`

- [ ] **11. Wire it into pgconn.** Give `PgConn` a `TypeMap() *pgtype.Map`.
  Simple-query results are always text format — decode them through the map and
  extend the M03 tests to assert typed values, not raw bytes.
  *Commit:* `feat(pgconn): decode results through the type map`

## Security (OWASP)

**Risks: [A03 Injection](https://owasp.org/Top10/A03_2021-Injection/) · [Input Validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)**

Codecs are where user data becomes wire bytes. A codec that produces SQL text
instead of a parameter value is an injection vector.

- [ ] **Encoders produce *values*, never SQL fragments.** No codec ever emits a
  quote character to delimit itself — quoting is the server's job once the value
  travels as a parameter. Assert it: no encoder output starts with `'`.
- [ ] **NULL is a protocol-level marker, not the string `NULL`.** Test that a nil
  value encodes to the NULL signal and that decoding NULL into a non-pointer
  errors rather than silently yielding a zero value — silent zeros are how
  "deleted" flags become false.
- [ ] **Validate on decode, do not assume.** A `bool` codec receiving `"maybe"`
  errors. A UUID codec receiving 15 bytes errors. Never coerce.
- [ ] **Reject invalid UTF-8** on text decode, or document explicitly that you
  pass bytes through. Ambiguity here surfaces as mojibake at best and as a
  filter bypass at worst.
- [ ] **No panics.** A codec is called with attacker-influenced bytes on every
  row; a panic is a DoS. Every `Decode` returns an error instead.
- [ ] **`sql.Scanner`/`driver.Valuer` calls are untrusted code.** A `Valuer`
  returning an unexpected type must produce an error, not a type assertion panic.
- [ ] **Registry immutability.** `NewMap()` returns an independent copy so one
  connection cannot poison another's type handling. Test it — shared mutable
  global state is a genuine cross-tenant risk in a pooled server.

**Verify:** `FuzzTextDecode` per codec — arbitrary bytes, never a panic.

## Done when

- Every OID constant has a codec, or a `// TODO(M08)` naming the milestone.
- Round-trip tests cover NULL, zero values, and extremes (`math.MinInt64`,
  empty string, `time.Time{}`).
- No global mutable registry — `NewMap()` returns an independent copy.

## Stretch

- `FuzzTextDecode` per codec: never panic on arbitrary bytes.
- Composite and enum types: decode `(1,"a")` tuples into a struct.
