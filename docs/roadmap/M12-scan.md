# M12 — Row scanner  `feat/scan`

**Covers:** reflection · generics · struct tags · caching reflective work ·
API design for ergonomics

**Links**
- [Learn Go with Tests — Reflection](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/reflection)
- [Learn Go with Tests — Generics](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/generics)
- [The Laws of Reflection](https://go.dev/blog/laws-of-reflection)
- [`reflect`](https://pkg.go.dev/reflect) · [`reflect.StructTag`](https://pkg.go.dev/reflect#StructTag)
- [pgx `RowToStructByName`](https://pkg.go.dev/github.com/jackc/pgx/v5#RowToStructByName) — the API you are mirroring

**Package:** `scan/`. **Imports allowed:** `forge` only.

> Names and semantics here are **contractual**: M14 swaps this in for pgx in
> Quriny with no call-site changes. Check each signature against the pgx doc
> before implementing.

---

## Steps

- [ ] **1. RED — `CollectRows` with a value function.** `scan_test.go` against a
  fake `forge.Rows`: collect 3 rows into `[]int32`; zero rows returns an empty
  **non-nil** slice (assert `len == 0` *and* `!= nil` — callers marshal this to
  JSON and `null` vs `[]` matters); a scan error aborts and returns the error
  with nothing partial; `Rows.Close` is always called, including on the error path.

- [ ] **2. GREEN — `collect.go`.**
  ```go
  type RowToFunc[T any] func(row forge.CollectableRow) (T, error)
  func CollectRows[T any](rows forge.Rows, fn RowToFunc[T]) ([]T, error)
  func AppendRows[T any, S ~[]T](slice S, rows forge.Rows, fn RowToFunc[T]) (S, error)
  func CollectOneRow[T any](rows forge.Rows, fn RowToFunc[T]) (T, error)          // ErrNoRows if none
  func CollectExactlyOneRow[T any](rows forge.Rows, fn RowToFunc[T]) (T, error)   // + ErrTooManyRows
  func ForEachRow(rows forge.Rows, scans []any, fn func() error) (forge.Result, error)
  ```
  `defer rows.Close()` in every one; check `rows.Err()` after the loop — a
  `Next()` returning false means "done *or* failed" and forgetting the second
  case silently truncates result sets. That bug class is why `Err()` exists.
  *Commit:* `feat(scan): collect rows helpers`

- [ ] **3. RED — `RowToMap`.** A row with columns `id, name, created_at` becomes
  `map[string]any` with those keys; NULL becomes a nil value present in the map
  (assert the key exists); duplicate column names — decide (last wins, or error)
  and test it.

- [ ] **4. GREEN.** `func RowToMap(row forge.CollectableRow) (map[string]any, error)`
  built on `Rows.Values()` + `FieldDescriptions()`. Pre-size the map with
  `make(map[string]any, len(fields))`.
  *Commit:* `feat(scan): RowToMap`

- [ ] **5. RED — struct mapping.** Cases, all table-driven:
  - exact-name match, case-insensitive match (`user_id` ↔ `UserID`)
  - `db:"user_id"` tag wins over the field name
  - `db:"-"` skips the field
  - unexported fields are skipped, never panic
  - embedded structs are flattened (`type Base struct{ID int}` inside `User`)
  - a pointer field receives nil for NULL
  - a column with no matching field → error naming the column
  - a field with no matching column → error naming the field
  Both directions must error: silently ignoring a mismatch is how production
  data goes missing.

- [ ] **6. GREEN — `struct.go`.**
  ```go
  func RowToStructByName[T any](row forge.CollectableRow) (T, error)
  func RowToAddrOfStructByName[T any](row forge.CollectableRow) (*T, error)
  func RowToStructByNameLax[T any](row forge.CollectableRow) (T, error)  // missing fields tolerated
  func RowToStructByPos[T any](row forge.CollectableRow) (T, error)
  ```
  Algorithm: build a `[]any` of pointers to the target fields in column order,
  then hand it to `row.Scan(dest...)` — one reflective pass, then the normal scan
  path does the typed work. Do **not** scan into `any` and then reflect-assign;
  that loses type information and doubles allocations.
  Name normalisation: lowercase and strip `_` from both sides before comparing.
  *Commit:* `feat(scan): struct row mapping by name and position`

- [ ] **7. RED — the reflection cache.** Assert that mapping 10 000 rows of the
  same struct performs the field-walk once: expose an unexported hit counter, or
  benchmark and assert `allocs/op` stays flat as row count grows.

- [ ] **8. GREEN — cache.** `sync.Map` keyed by
  `struct{ typ reflect.Type; cols string }` (the column-name list matters — the
  same struct can be scanned from different SELECTs). Value is the resolved
  `[]int` field-index path per column, ready for `reflect.Value.FieldByIndex`.
  *Commit:* `perf(scan): cache struct field resolution`

- [ ] **9. RED/GREEN — the Quriny surface.** Confirm against the real call sites
  in `platform/backend/internal/store/postgres.go`: `CollectRows(rows, RowToMap)`,
  `CollectExactlyOneRow(rows, RowToMap)`, `pgx.ErrNoRows` → `forge.ErrNoRows`.
  Write a test file `quriny_compat_test.go` that reproduces each call shape
  verbatim so M14 is a rename, not a rewrite.
  *Commit:* `test(scan): pin the API shapes Quriny depends on`

- [ ] **10. REFACTOR + docs.** Every exported function gets a runnable `Example`.
  Benchmark `BenchmarkRowToStructByName` vs `BenchmarkRowToMap` vs manual `Scan`;
  put the ratio in the package doc so users can make an informed choice.
  *Commit:* `docs(scan): examples and performance notes`

## Security (OWASP)

**Risks: [A04 Insecure Design](https://owasp.org/Top10/A04_2021-Insecure_Design/) · [A09 Logging Failures](https://owasp.org/Top10/A09_2021-Security_Logging_and_Monitoring_Failures/)**

Scanning is where database rows become application objects. Silent mismatches
here become missing authorisation fields upstream.

- [ ] **Fail closed on mismatch, in both directions.** A column with no field, or
  a field with no column, is an error — not a skip. The dangerous case is
  concrete: a struct with `IsAdmin bool` scanned from a query that forgot to
  select it yields `false` under a lax scanner and a security decision made on a
  default value. Test that exact scenario by name.
- [ ] **`Lax` variants are opt-in and documented as such.** Their doc comment
  states the risk above.
- [ ] **`Rows.Err()` checked after every loop.** A truncated result set that
  looks like an empty one is how "user has no active bans" gets decided by a
  network error. This is the single most consequential bug in the package.
- [ ] **Never log row contents on a scan error.** The message names the column
  and the Go type, never the value — result rows are the most PII-dense data
  forge touches.
- [ ] **Reflection stays inside the package.** Do not expose an API that lets a
  caller scan into an arbitrary `any` chosen at runtime from user input.
- [ ] **Cache key must include the column set**, not just the type. A cache
  collision across two different SELECTs would write column A's value into field
  B — a data-integrity bug with authorisation consequences.
- [ ] **No panics.** `RowToStructByName` on an unexported field, a nil pointer, a
  non-struct type parameter — all errors, never panics.

**Verify:** `TestMissingColumnIsError` using an `IsAdmin` field; assert the error
rather than a `false`.

## Done when

- Zero rows returns an empty non-nil slice — tested explicitly.
- Column/field mismatches error in both directions with the offending name.
- `allocs/op` for `RowToStructByName` is constant in the number of rows.
- `scan` imports only `forge` + stdlib.

## Stretch

- `RowToStructByNameLax` with a `scan:"omitempty"`-style option set.
- Support scanning into `map[string]any` fields for `jsonb` columns.
