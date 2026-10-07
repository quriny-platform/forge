# M08 — Binary codecs  `feat/pgtype-binary`

**Covers:** binary wire encodings · epoch arithmetic & time zones · arbitrary
precision numerics · array encoding · differential fuzzing

**Links**
- [`pg_type` catalog](https://www.postgresql.org/docs/current/catalog-pg-type.html)
- [PostgreSQL source: `utils/adt`](https://github.com/postgres/postgres/tree/master/src/backend/utils/adt) — `*_send`/`*_recv` are the authority
- [Date/time types](https://www.postgresql.org/docs/current/datatype-datetime.html)
- [Arrays](https://www.postgresql.org/docs/current/arrays.html)
- [`time` package](https://pkg.go.dev/time) · [`math/big`](https://pkg.go.dev/math/big)
- [Go fuzzing reference](https://go.dev/security/fuzz/)

**Package:** `postgres/pgtype`.

---

## Encodings you will implement

| type | binary layout |
|---|---|
| `bool` | 1 byte, 0/1 |
| `int2/4/8` | big-endian two's complement, 2/4/8 bytes |
| `float4/8` | IEEE-754 big-endian (`math.Float64bits`) |
| `text/varchar` | raw UTF-8, no terminator (length comes from the field header) |
| `bytea` | raw bytes |
| `date` | int32 days since **2000-01-01** |
| `timestamp`/`timestamptz` | int64 **microseconds** since 2000-01-01 00:00:00 UTC |
| `time` | int64 microseconds since midnight |
| `interval` | int64 micros + int32 days + int32 months |
| `uuid` | 16 raw bytes |
| `json` | raw UTF-8 |
| `jsonb` | **1 version byte (`0x01`)** then raw UTF-8 — the classic gotcha |
| `numeric` | int16 ndigits, int16 weight, int16 sign, int16 dscale, then ndigits × int16 base-10000 digits |
| arrays | int32 ndim, int32 hasnull, int32 elemOID, then per-dim (int32 length, int32 lowerBound=1), then each element as int32 length (−1 = NULL) + bytes |

The 2000-01-01 epoch is the single most common source of “my dates are 30 years
off” bugs. Write `TestEpoch` asserting `2000-01-01T00:00:00Z` encodes to zero.

Infinity: `timestamptz` uses `math.MaxInt64` / `math.MinInt64` for
`infinity`/`-infinity`. Decide how you represent that in Go (`pgtype.Timestamptz`
with an `InfinityModifier` field) and test it.

---

## Steps

- [ ] **1. RED — scalar binary round-trips.** Extend the M06 codec table with a
  binary column: for every scalar type, `Encode(v, BinaryFormat)` must produce
  the exact bytes from a golden hex fixture, and decode back to `v`. Fixtures
  come from real PostgreSQL: run
  `SELECT '2026-09-06'::date` with `resultFormats=[1]` in an integration test and
  dump what the server sends. Generate them once with a `-update` flag.

- [ ] **2. GREEN — scalar binary paths.** Fill in the `BinaryFormat` branch of
  each codec from M06. `binary.BigEndian.AppendUint64` and friends; no manual
  bit-shifting loops. Every decode checks `len(src)` first and returns
  `fmt.Errorf("pgtype: invalid %s binary length %d", name, len(src))`.
  *Commits:* `feat(pgtype): binary integer and float codecs`,
  `feat(pgtype): binary text and bytea codecs`

- [ ] **3. RED — time.** Cases: the epoch itself; a pre-2000 date (negative
  micros); microsecond precision preserved (`time.Time` is nanosecond — assert
  the documented truncation, do not silently round); `timestamptz` decoded as UTC
  and only converted on display; a `time.Time` in `Asia/Jakarta` encoding to the
  same instant as its UTC equivalent; `infinity` both directions; `date` before
  1 AD if you support it.

- [ ] **4. GREEN — `codec_time.go` binary.** Constants:
  `pgEpochJulian = time.Date(2000,1,1,0,0,0,0,time.UTC)`;
  `microsecondsPerDay`. Encode: `t.UTC().Sub(epoch).Microseconds()`. Decode:
  `epoch.Add(time.Duration(micros) * time.Microsecond)`. Write the truncation
  behaviour in the doc comment — nanoseconds are silently lost and users deserve
  to know.
  *Commit:* `feat(pgtype): binary date/time codecs`

- [ ] **5. RED — numeric.** The hard one. Cases: `0`; `1`; `-1`; `0.5`;
  `12345.6789`; a 40-digit value that no `float64` can hold; `NaN`
  (sign = `0xC000`); trailing-zero handling (`1.50` vs `1.5` — dscale differs,
  value does not). Decide your Go representation first: `*big.Rat` is lossless
  but slow, a `pgtype.Numeric{Int *big.Int, Exp int32, NaN bool, Valid bool}` is
  what pgx uses and what you should probably mirror (record the reasoning either way).

- [ ] **6. GREEN — `codec_numeric.go`.** Base-10000 “NBASE” digits, most
  significant first. Weight is the base-10000 exponent of the first digit; dscale
  is the display scale in base-10 digits. Encoding: normalise the `big.Int`,
  chunk it into base-10000 groups, compute weight and dscale. The hardest codec
  in the package. Add
  `Float64() (float64, error)` and `String() string` convenience methods.
  *Commit:* `feat(pgtype): binary numeric codec`

- [ ] **7. RED — arrays.** Cases: `[]int32{1,2,3}`; empty array (ndim = 0, and
  the encoding has **no** dimension block — test it); array containing NULL
  (`hasnull = 1`, element length `-1`); two-dimensional `[][]string`;
  non-default lower bounds when decoding (PostgreSQL allows `[0:2]`, and your
  decoder must not assume 1).

- [ ] **8. GREEN — `codec_array.go`, generic.**
  ```go
  type ArrayCodec[T any] struct{ ElementOID uint32; Element Codec }
  ```
  Encoding a Go slice: reflect once to get the element type, then loop without
  further reflection. Decoding into `*[]T`: allocate once with the known length.
  Register `Int4ArrayOID`, `TextArrayOID`, `BoolArrayOID`, `UUIDArrayOID`,
  `NumericArrayOID` in the default map.
  *Commit:* `feat(pgtype): binary array codec`

- [ ] **9. RED/GREEN — jsonb + uuid.** `jsonb` must write the `0x01` version byte
  and reject any other version on decode with a clear error. `uuid` accepts
  `[16]byte`, `string` (with and without hyphens — decide, then test), and any
  type implementing `encoding.TextUnmarshaler`, which is how third-party UUID
  packages plug in. Quriny stores UUIDs as `[16]byte` → string, so make that path
  allocation-free and benchmark it.
  *Commit:* `feat(pgtype): binary jsonb and uuid codecs`

- [ ] **10. Fuzz + differential test.** `FuzzBinaryDecode` per codec: no panic,
  no OOM (cap declared lengths before allocating — an array header claiming 2^31
  elements must error, not allocate). Then the real prize:
  `//go:build integration` differential fuzzing — send the fuzzed value to a real
  PostgreSQL with `SELECT $1::<type>` in binary format and assert the round trip
  agrees with your codec. Disagreements are always your bug.
  *Commit:* `test(pgtype): fuzz binary codecs`

- [ ] **11. Switch the default to binary.** Where the server describes a column
  with a known OID and the codec supports binary, request `resultFormats=[1]`.
  Benchmark the same query text vs binary: the numbers belong in the PR.
  *Commit:* `perf(pgconn): prefer binary format for known types`

## Security (OWASP)

**Risks: [A08 Integrity Failures](https://owasp.org/Top10/A08_2021-Software_and_Data_Integrity_Failures/) · [DoS](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html)**

Binary decoding is the second memory-safety boundary (TB2). Length fields here
are richer and therefore more dangerous than in M02.

- [ ] **Cap every count before allocating.** The array header carries `ndim`,
  per-dimension lengths, and per-element lengths — all attacker-controlled. An
  array claiming `2^31` elements, or dimensions whose product overflows `int`,
  must error before a single allocation. Test: `TestArrayHeaderBombRejected`.
  Check the multiplication for overflow explicitly; `int` overflow is silent in Go.
- [ ] **`numeric` is the sharpest edge.** `ndigits` drives a loop and an
  allocation; a hostile value produces a multi-gigabyte `big.Int`. Cap `ndigits`
  (PostgreSQL's own limit is ~16 383 base-10000 digits) and reject beyond it.
- [ ] **Exact length checks on fixed-width types.** `uuid` is exactly 16 bytes,
  `int4` exactly 4, `bool` exactly 1. `len(src) >= 4` is not the same as
  `len(src) == 4`, and the difference has produced real CVEs.
- [ ] **`jsonb` version byte validated.** Anything but `0x01` errors; do not skip
  the byte and hope.
- [ ] **No `unsafe`, no `reflect.SliceHeader` tricks** for the "fast path". If a
  benchmark tempts you, record the temptation in an ADR and resist it.
- [ ] **Nested depth limits** if you add composite or range types — recursive
  decoders need a depth cap or a hostile payload becomes a stack overflow.
- [ ] Fuzz every binary decoder; the differential integration fuzz against a real
  server is the strongest correctness *and* safety evidence you can produce.

**Verify:** each `FuzzBinaryDecode` target clean for 10 minutes; a crafted array
header claiming 2³¹ elements returns an error with flat memory usage.

## Done when

- Golden fixtures generated from a real server, committed under `testdata/binary/`.
- Numeric handles values outside `float64` range without loss.
- No decode path allocates based on an unvalidated length field.

## Stretch

- Composite types and enums via `pg_type`/`pg_attribute` introspection at connect.
- `hstore`, `inet`/`cidr`, `tstzrange` — pick one and add it.
