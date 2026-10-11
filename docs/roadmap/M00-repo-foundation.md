# M00 — Repo foundation & CI  `chore/repo-foundation`

**Covers:** Go module layout · GitHub Actions · golangci-lint · OSS hygiene · ADRs

**Links**
- [Go Modules Reference](https://go.dev/ref/mod)
- [Publishing a Go module](https://go.dev/doc/modules/publishing)
- [golangci-lint configuration](https://golangci-lint.run/usage/configuration/)
- [setup-go action](https://github.com/actions/setup-go)
- [Choose a License](https://choosealicense.com/) · [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/)
- [Go doc comments](https://go.dev/doc/comment)

> No Go code in this milestone. It exists so every later milestone lands on a
> repo that already enforces its own rules.

---

## Steps

- [ ] **1. Decide the licence.** MIT (maximally permissive, shortest) vs
  Apache-2.0 (adds an explicit patent grant — what most infra libraries pick).
  Write `LICENSE` with your name and the year. Record the choice in a one-line
  note in the README; if you pick Apache-2.0, also add `NOTICE`.
  *Commit:* `chore: add LICENSE`

- [ ] **2. `README.md`.** Sections, in this order: one-sentence description →
  status banner (“alpha, API unstable”) → install (`go get github.com/quriny-platform/forge`)
  → a 15-line usage sketch (you will fill it in for real at M10) → feature matrix
  with unchecked boxes mirroring the milestone list → architecture diagram as a
  fenced ASCII block copied from the layer table in [ROADMAP.md](../ROADMAP.md)
  → development section (`go test ./...`, docker compose) → licence.
  Leave badge placeholders; wire them up in M17.
  *Commit:* `docs: add README`

- [ ] **3. `.golangci.yml`.** Start strict but survivable. Enable: `govet`,
  `staticcheck`, `errcheck`, `ineffassign`, `unused`, `gocritic`, `revive`,
  `misspell`, `bodyclose`, `errorlint`, `gosec`. Set `run.timeout: 5m`. Add an
  `issues.exclude-rules` entry allowing `gosec` findings in `*_test.go` only.
  Run `golangci-lint run ./...` — it must pass on the empty repo.
  *Commit:* `chore: add golangci-lint config`

- [ ] **4. CI workflow** at `.github/workflows/ci.yml`. Three jobs:
  - `test`: matrix over `go-version: [1.26.x, 1.27.x]` and
    `os: [ubuntu-latest, macos-latest]`; steps = checkout, setup-go (with
    `cache: true`), `go build ./...`, `go vet ./...`, `go test ./...`.
  - `race`: ubuntu only, `go test -race -count=1 ./...`.
  - `lint`: `golangci/golangci-lint-action@v6`.
  Trigger on `push` to `main` and on `pull_request`. Add `permissions: contents: read`.
  *Commit:* `ci: add build, race and lint workflows`

- [ ] **5. Integration test scaffolding.** `docker-compose.yml` with a single
  `postgres:17` service: `POSTGRES_PASSWORD`, `POSTGRES_DB=forge_test`, port
  `5433:5432` (5433 so it never collides with a local install), and a
  `healthcheck` using `pg_isready`. Add `.github/workflows/integration.yml` that
  runs the same image as a `services:` container and executes
  `go test -tags=integration ./...`. It will pass trivially until M10 — that is fine.
  *Commit:* `ci: add postgres integration harness`

- [ ] **6. Contribution docs.** `CONTRIBUTING.md`: how to run tests, the
  red-green-refactor expectation, commit-message format, the layer rules, and
  “every pgx divergence needs an ADR”. `CODE_OF_CONDUCT.md`: Contributor
  Covenant 2.1 verbatim with your contact email. `SECURITY.md`: how to report a
  vulnerability privately (GitHub private advisory), supported-version table.
  *Commit:* `docs: add contributing, conduct and security policies`

- [ ] **7. Templates.** `.github/ISSUE_TEMPLATE/bug_report.yml` (Go version,
  PostgreSQL version, minimal reproducer, expected/actual) and `feature_request.yml`.
  `.github/pull_request_template.md` with the Definition-of-Done checklist from
  [ROADMAP.md](../ROADMAP.md).
  *Commit:* `chore: add issue and PR templates`

- [ ] **8. Housekeeping.** Confirm `.gitignore` covers `*.test`, coverage files,
  `testdata/fuzz/` cache dirs. Add `dependabot.yml` for `gomod` and
  `github-actions`, weekly. Add `Makefile` (or `Taskfile`) with `test`, `race`,
  `lint`, `fmt`, `integration`, `fuzz` targets so CI and local run identical commands.
  *Commit:* `chore: add dependabot and make targets`

- [ ] **9. Branch protection.** On GitHub: require PRs to `main`, require the
  `test`, `race`, `lint` checks, disallow force-push. Do this in the web UI, then
  note it in `CONTRIBUTING.md`.

## Security (OWASP)

**Risk: [A06 Vulnerable & Outdated Components](https://owasp.org/Top10/A06_2021-Vulnerable_and_Outdated_Components/) · [CI/CD Top 10](https://owasp.org/www-project-top-10-ci-cd-security-risks/)**

The supply chain is the attack surface you inherit before writing a line of code.

- [ ] Add `govulncheck` to CI as its own job (`go install golang.org/x/vuln/cmd/govulncheck@latest`), failing the build on any finding.
- [ ] **Pin every GitHub Action by commit SHA, not by tag.** `actions/checkout@v4` is a mutable pointer; `@8ade135...` is not. This is CICD-SEC-3 (dependency chain abuse).
- [ ] Set `permissions: contents: read` at workflow level; grant `contents: write` only to the release job at M17. Default-write tokens are CICD-SEC-5.
- [ ] Enable Dependabot for `gomod` and `github-actions`; enable GitHub secret scanning and private vulnerability reporting on the repo.
- [ ] `SECURITY.md` states the disclosure channel, expected response time, and supported versions — a real address, not a placeholder.
- [ ] `.golangci.yml` enables `gosec` and `errorlint`. Do **not** globally exclude `gosec`; per-line `#nosec` with a justification only.
- [ ] Add `docs/SECURITY-PRACTICES.md` to the PR template checklist so every later PR is read against it.

**Verify:** open a PR adding a knowingly vulnerable dependency; CI must fail on `govulncheck`. Revert it.

## Done when

- Push a throwaway PR: all three checks run and pass.
- `make lint test` green locally on a clean checkout.
- `docs/adr/0001-record-architecture-decisions.md` exists and the index lists it.

## Stretch

- Add a `coverage` job posting `go tool cover -func` output as a PR comment.
- Add `govulncheck` as a scheduled weekly workflow.
