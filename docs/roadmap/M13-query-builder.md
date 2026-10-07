# M13 — Query builder  `feat/query-builder`

**Covers:** fluent API design · dialects · SQL injection surfaces · identifier
quoting · fuzzing a security boundary

**Links**
- [SQL syntax: lexical structure & quoted identifiers](https://www.postgresql.org/docs/current/sql-syntax-lexical.html)
- [SQL injection (OWASP)](https://owasp.org/www-community/attacks/SQL_Injection)
- [`strings.Builder`](https://pkg.go.dev/strings#Builder)
- [Learn Go with Tests — Maps / Structs](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/maps)

**Package:** `qb/`. **Imports:** nothing outside stdlib. `qb` knows nothing about
connections; it produces `(sql string, args []any)` and that is all.

---

## The one rule

**Values are always placeholders. Identifiers are always quoted. Nothing else is
interpolated, ever.** A builder that accepts a raw `WHERE` fragment from the
caller is a builder that ships SQL injection. If you provide an escape hatch
(and you probably should), name it `Raw` / `Unsafe` so the reviewer sees it in
the diff.

Identifier quoting for PostgreSQL: wrap in `"`, and double any embedded `"`.
`foo"; DROP TABLE users; --` becomes `"foo""; DROP TABLE users; --"` — a single
legal identifier. Also reject `\x00`, which cannot be represented at all.

---

## Steps

- [ ] **1. RED — identifier quoting.** `dialect_test.go`, table-driven and
  deliberately hostile: `users` → `"users"`; `User Name` → `"User Name"`;
  `a"b` → `"a""b"`; `sch.tbl` (dotted) → `"sch"."tbl"` *only* via an explicit
  `QualifiedIdent` API, never by splitting a plain string; empty string → error;
  a string containing `\x00` → error; a 100-char name → error or truncation per
  PostgreSQL's 63-byte `NAMEDATALEN` (pick, document, test).

- [ ] **2. GREEN — `dialect.go`.**
  ```go
  type Dialect interface {
      Placeholder(n int) string          // postgres: "$1"; mysql-ish: "?"
      QuoteIdentifier(name string) (string, error)
      SupportsReturning() bool
  }
  type PostgresDialect struct{}
  ```
  Keeping this an interface is what makes `qb` engine-agnostic — and it is the
  place a future MySQL backend plugs in without touching the builders.
  *Commit:* `feat(qb): postgres dialect with identifier quoting`

- [ ] **3. RED/GREEN — fuzz the boundary.** `FuzzQuoteIdentifier`: for arbitrary
  input, the output must either be an error or a string that (a) starts and ends
  with `"`, (b) contains no unescaped `"` in between, (c) round-trips — i.e.
  unquoting yields the original. Property-based assertions like these catch what
  a table never will. Run for 5 minutes; commit the corpus.
  *Commit:* `test(qb): fuzz identifier quoting`

- [ ] **4. RED — SELECT.** Build up in small increments, one test per method:
  `Select("id","name").From("users")` → `SELECT "id", "name" FROM "users"`;
  `.Where("id = ?", 5)` → `WHERE "id" = $1` with `args = [5]`;
  two `Where` calls AND together; `.OrderBy("created_at DESC")`;
  `.Limit(10).Offset(20)`; `.Join`, `.LeftJoin`; `.GroupBy`/`.Having`.
  Assert both the SQL string *and* the args slice every time — a builder that
  gets placeholders and args out of sync is the worst possible bug here.

- [ ] **5. GREEN — `select.go`.**
  ```go
  type SelectBuilder struct{ /* … */ }
  func Select(cols ...string) *SelectBuilder
  func (b *SelectBuilder) From(table string) *SelectBuilder
  func (b *SelectBuilder) Where(cond string, args ...any) *SelectBuilder
  func (b *SelectBuilder) ToSQL() (string, []any, error)
  ```
  Two design decisions to make deliberately:
  1. **Immutable or mutable builders?** Mutable is simpler and matches squirrel;
     immutable (each method returns a copy) makes shared base queries safe. Pick,
     write the test that demonstrates the chosen semantics, and record it in an ADR.
  2. **Error handling.** Accumulate errors in the builder and return them from
     `ToSQL()` rather than panicking or returning `(*Builder, error)` from every
     method — fluent chains and multi-returns do not mix.
  Placeholder rewriting: the caller writes `?`, `ToSQL` renumbers to `$1…$n` in
  argument order. Test the case where a literal `?` appears inside a quoted
  string literal — either forbid string literals in conditions (recommended) or
  parse properly. Forbidding is a legitimate, testable choice.
  *Commits:* `feat(qb): select builder`, `feat(qb): joins and grouping`

- [ ] **6. RED/GREEN — INSERT / UPDATE / DELETE.**
  `Insert("users").Columns("name","email").Values("a","b").Returning("id")`;
  multi-row `Values` calls producing one statement;
  `Update("users").Set("name", x).Where(...)`;
  `Delete("users").Where(...)`.
  Guard rails to test: `Update` with no `Where` returns an error unless
  `.AllowFullTableUpdate()` was called (this has saved real production data);
  `Insert` with a column/value count mismatch errors before producing SQL.
  *Commits:* `feat(qb): insert builder`, `feat(qb): update and delete builders`

- [ ] **7. RED/GREEN — conditions.** A small expression type so callers do not
  hand-write strings:
  ```go
  type Cond interface{ toSQL(d Dialect, argIdx *int) (string, []any, error) }
  func Eq(col string, v any) Cond
  func In(col string, vs ...any) Cond      // expands to ($1,$2,$3); empty → FALSE
  func And(cs ...Cond) Cond
  func Or(cs ...Cond) Cond
  func IsNull(col string) Cond
  ```
  Test `In` with zero values explicitly — the naive implementation emits
  `IN ()`, a syntax error; the correct one emits `FALSE` (or `1=0`).
  *Commit:* `feat(qb): typed condition expressions`

- [ ] **8. RED — injection suite.** `injection_test.go`: for every builder,
  feed the classic payloads (`'; DROP TABLE users; --`, `" OR "1"="1`,
  `\x00`, unicode homoglyph quotes) as **column names**, **table names**, and
  **values**. Assert values always land in `args` (never in the SQL) and
  identifiers are always quoted. This file is the security contract; keep it
  growing.

- [ ] **9. REFACTOR.** Extract the shared `strings.Builder` + arg-collection
  logic into an unexported `stmtWriter`. Benchmark `BenchmarkSelectToSQL`; a
  builder used per request should be well under a microsecond and allocate once.
  *Commit:* `refactor(qb): shared statement writer`

- [ ] **10. ADR.** `docs/adr/0009-query-builder-scope.md`: what `qb` deliberately
  does *not* do — no ORM, no schema knowledge, no migrations, no query execution.
  Scope creep here is how query builders become bad ORMs.

## Security (OWASP)

**Risk: [A03 Injection](https://owasp.org/Top10/A03_2021-Injection/) — this milestone *is* the control.**

References: [SQL Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html) ·
[Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Injection_Prevention_Cheat_Sheet.html) ·
[SQL Injection (attack)](https://owasp.org/www-community/attacks/SQL_Injection). ASVS V5.

- [ ] **`QuoteIdentifier` is the single choke point.** Every identifier in every
  builder passes through it. Grep the package: no other function may emit a
  `"` character. Enforce it with a test that walks the AST or, more simply, by
  keeping quoting in one unexported file and reviewing every diff to it.
- [ ] **Reject rather than mangle.** `\x00` in an identifier is an error.
  Over-length identifiers (>63 bytes, PostgreSQL's `NAMEDATALEN`) error rather
  than truncate — silent truncation can collide two distinct names into one, and
  that is a genuine access-control bypass on a schema-qualified table.
- [ ] **Dotted names are never split implicitly.** `"users.name"` as a single
  identifier is one weird column name; a caller wanting schema qualification uses
  an explicit API. Implicit splitting is how `public.users` becomes injectable.
- [ ] **No raw fragments without a loud name.** If you ship an escape hatch it is
  `Raw`/`Unsafe`, its doc comment says "the caller is responsible for injection
  safety", and it never accepts arguments that look like values.
- [ ] **Placeholder/argument sync is a security invariant.** A desynchronised
  builder can bind argument 2 where argument 1 belongs — an authorisation filter
  silently comparing the wrong value. Assert SQL *and* args together in every
  test; never one alone.
- [ ] **`In()` with zero values emits `FALSE`, never `IN ()`.** And never an
  empty string that a caller might concatenate around.
- [ ] **`UPDATE`/`DELETE` without `WHERE` is refused** unless explicitly allowed.
  Not strictly injection, but the same blast radius.
- [ ] **The injection suite (step 8) is a permanent artefact.** Add a case for
  every new payload encountered; treat a new bypass as a security release.
- [ ] **`FuzzQuoteIdentifier` is a required gate**, with property assertions:
  output is either an error or a string that unquotes to exactly the input.

**Verify:** the full injection suite green across every builder; fuzz corpus
committed; no `Sprintf` anywhere in `qb` (`grep -n Sprintf qb/` returns nothing
outside error messages).

## Done when

- Injection suite green for every builder.
- Fuzz corpus committed; `FuzzQuoteIdentifier` clean for 5 minutes.
- `qb` has zero imports outside stdlib (architecture test proves it).
- Every builder has a runnable `Example`.

## Stretch

- CTEs (`WITH`) and `ON CONFLICT DO UPDATE`.
- A `Dialect` for SQLite or MySQL — implementing a second one is the only real
  proof the abstraction works.
