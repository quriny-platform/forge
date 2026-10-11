# M14 — Quriny EntityStore migration  `feat/quriny-entitystore`

**Covers:** API compatibility · incremental migration · differential testing ·
production readiness assessment

**Links**
- [pgx v5 API](https://pkg.go.dev/github.com/jackc/pgx/v5) — the surface you are replacing
- [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool)
- [Go workspaces (`go.work`)](https://go.dev/doc/tutorial/workspaces) — develop both repos together
- [`go mod edit -replace`](https://go.dev/ref/mod#go-mod-edit)

**Repos:** `forge` (this one) + `quriny-platform/platform`.
Target file: `backend/internal/store/postgres.go`.

> This is the acceptance test for M01–M13. If the swap needs changes to
> forge's API, forge's API was wrong — fix forge, not the call sites.

---

## Steps

- [ ] **1. Survey before touching anything.** In the platform repo, produce an
  inventory: `grep -rn "pgx\.\|pgxpool\.\|pgconn\." backend/ > /tmp/pgx-usage.txt`.
  Group by symbol and count. Write the result into
  `docs/migration-quriny.md` in *this* repo as a table: pgx symbol → forge
  equivalent → status (`ready` / `missing` / `differs`). Every `missing` row is a
  forge bug to fix before the migration starts.
  *Commit:* `docs: inventory of pgx usage in Quriny`

- [ ] **2. Close the gaps.** Work each `missing`/`differs` row back into forge
  as its own small TDD cycle. Expected candidates: `pgx.CollectRows` variants,
  `pgxpool.Pool.Begin` semantics, `pgconn.PgError` field names,
  `pgx.ErrNoRows` identity, UUID `[16]byte` handling, `map[string]any` record
  shape from `RowToMap`. Do **not** paper over differences at the call site.
  *Commits:* one per gap, in the relevant package.

- [ ] **3. Characterisation tests, in the platform repo, on pgx.** Before
  swapping: write integration tests around `EntityStore`'s public methods that
  assert current behaviour precisely — returned map key types, error identities,
  NULL handling, ordering. These tests must pass *unchanged* after the swap.
  That is the entire safety net; do not skip it.
  *Commit (platform):* `test(store): characterise EntityStore behaviour`

- [ ] **4. Wire up a workspace.** At the parent directory:
  ```bash
  go work init ./forge ./platform/backend
  ```
  Now the platform builds against your working tree with no publishing loop.
  Remember `go.work` must **not** be committed to the platform repo.

- [ ] **5. Swap, one method at a time.** Order: read-only single-row queries →
  read-only multi-row → writes → transactions. After each method:
  characterisation tests green, commit. Type-level changes to expect:
  `*pgxpool.Pool` → `*pool.Pool`; `pgx.Rows` → `forge.Rows`;
  `pgx.CollectRows(rows, pgx.RowToMap)` → `scan.CollectRows(rows, scan.RowToMap)`;
  `pgx.ErrNoRows` → `forge.ErrNoRows`; `pgconn.PgError` → forge's.
  *Commits (platform):* `refactor(store): migrate <method> to forge`

- [ ] **6. Differential run.** Keep both implementations alive behind an
  interface for one commit and run the characterisation suite against each
  (`EntityStore` backed by pgx vs by forge), asserting identical outputs for the
  same inputs — including error values. Any divergence is a forge bug with a
  perfect reproducer attached. Delete the pgx path once green.
  *Commit (platform):* `test(store): differential pgx vs forge`

- [ ] **7. Load-test the pool.** Run the platform's existing load or smoke test
  (or write a 5-minute one: N workers, mixed read/write) against both. Compare:
  p50/p95/p99 latency, allocations, connection count at steady state,
  behaviour when PostgreSQL is restarted mid-run. Record the numbers in
  `docs/migration-quriny.md`. Being slower than pgx is acceptable and expected;
  hanging, leaking connections, or failing to recover from a restart is not.
  *Commit:* `docs: forge vs pgx benchmark results for Quriny workload`

- [ ] **8. Soak.** Run the platform against forge in a staging environment for a
  few days with tracing on. Watch for: goroutine growth, connection churn,
  statement cache thrash, `57014`s, TLS renegotiation errors. Fix what surfaces,
  each as a TDD cycle in forge with the production symptom as the test name.

- [ ] **9. Remove pgx.** `go mod tidy` in the platform; assert `pgx` is gone from
  `go.mod` and `go.sum`. Replace `go.work` with a proper versioned dependency
  once M17 tags `v0.1.0` (until then, a `replace` directive with a commit SHA).
  *Commit (platform):* `chore: drop pgx dependency`

- [ ] **10. Write it up.** A `docs/migration-quriny.md` postmortem section: what
  forge got wrong, which API decisions proved right, what you would change. This
  is the most valuable document in the repo for a reader deciding whether to
  trust forge.

## Security (OWASP)

**Risks: [A03 Injection](https://owasp.org/Top10/A03_2021-Injection/) · [A09 Logging Failures](https://owasp.org/Top10/A09_2021-Security_Logging_and_Monitoring_Failures/) · [A06 Components](https://owasp.org/Top10/A06_2021-Vulnerable_and_Outdated_Components/)**

Swapping the driver under a running application is a change to its security
posture, whether or not you intend it to be.

- [ ] **Diff the security behaviour, not just the API.** For each migrated
  method, confirm: arguments still travel as parameters (capture the wire bytes
  once and check); TLS mode is unchanged; error messages expose no more than
  before; log volume and content are equivalent.
- [ ] **Audit the platform's own SQL while you are in there.** You will be
  reading every query in `EntityStore`. Any string concatenation into SQL you
  find is a finding — file it, fix it, and note that the migration surfaced it.
  This is free security value and a primary justification for the migration.
- [ ] **Session-state policy must match.** If `pgxpool` was resetting state on
  release and forge's pool is not, tenants can now see each other's session
  settings. Verify explicitly against the M11 policy.
- [ ] **Error-identity changes are authorisation changes.** Code branching on
  `pgx.ErrNoRows` that silently stops matching `forge.ErrNoRows` turns "not
  found" into "unhandled error" — or worse, into a permissive default. The
  characterisation tests exist for exactly this.
- [ ] **Do not commit `go.work` or a `replace` pointing at a local path.** A
  release built from an unpinned local checkout is an unverifiable artefact
  (A08). Pin to a commit SHA until v0.1.0 is tagged.
- [ ] **Run `govulncheck` on the platform before and after.** Removing pgx
  changes the transitive tree; confirm nothing regressed.
- [ ] **Soak with tracing on, then read the logs as an attacker would.** Search
  the staging log store for password-shaped and DSN-shaped strings.

**Verify:** wire capture proving parameterisation preserved; grep of staging
logs for credential patterns returns nothing; `govulncheck` clean on the
platform.

## Done when

- Characterisation tests pass unchanged against forge.
- `pgx` absent from the platform's `go.mod`.
- Staging soak clean for 72 hours: flat goroutines, flat connection count.
- Benchmark comparison documented honestly, including where forge loses.

## Stretch

- Publish the comparison as a blog post / README section — real migration data is
  rare and valuable.
- Add a `forge` build tag in the platform to allow instant rollback to pgx for
  one release cycle.
