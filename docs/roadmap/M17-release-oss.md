# M17 — Release & open source  `chore/release-v0.1.0`

**Covers:** semver in Go · changelogs · pkg.go.dev · runnable examples ·
release automation · maintainership

**Links**
- [Publishing a Go module](https://go.dev/doc/modules/publishing)
- [Module version numbering](https://go.dev/doc/modules/version-numbers)
- [Semantic Versioning 2.0.0](https://semver.org/) · [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
- [Go doc comments](https://go.dev/doc/comment) · [pkg.go.dev about](https://pkg.go.dev/about)
- [govulncheck](https://go.dev/blog/vuln)
- [Go 1 compatibility promise](https://go.dev/doc/go1compat) — the standard you are opting into at v1

---

## Steps

- [ ] **1. API review, on paper.** `go doc -all ./... > /tmp/api.txt` and read it
  as a stranger would. Questions to answer for every exported symbol: does it
  need to be exported? Is the name right *forever*? Does it take `context` where
  it blocks? Does it return an interface where a struct would do (usually wrong)?
  Anything you are unsure about, unexport it — `v0.x` is the only cheap time to
  do this.
  *Commit:* `refactor: reduce exported surface before v0.1.0`

- [ ] **2. Documentation sweep.** Every package gets a `doc.go` with a real
  overview: what it is, where it sits in the layer stack, a short example, and a
  pointer to the next layer up. Every exported identifier's comment starts with
  its own name and says *when you would use it*, not what the code does.
  Add `Example` functions for the top 10 entry points — they run in CI, so they
  cannot rot.
  *Commit:* `docs: package overviews and runnable examples`

- [ ] **3. `CHANGELOG.md`.** Keep-a-changelog format. For v0.1.0 write one
  `### Added` section per package, in layer order, describing capabilities not
  commits. Add an explicit `### Known limitations` section — no SASLprep,
  no logical replication, no composite types, whatever is true. Honest limits
  build more trust than a long feature list.
  *Commit:* `docs: add changelog for v0.1.0`

- [ ] **4. Stability statement.** In the README and `doc.go`: what `v0` means
  here (API may break between minor versions; patch releases never break), what
  you support (PostgreSQL 14+, Go 1.26+), and how you will handle breaking
  changes at v1 (the Go 1 compatibility promise, applied to forge).
  *Commit:* `docs: state versioning and support policy`

- [ ] **5. Security & supply chain.** Run `govulncheck ./...` and fix or document
  findings. Add it to CI. Verify `go.sum` is complete and the dependency list is
  as short as you claim (`go list -m all` — the goal is stdlib plus test-only
  deps). Finalise `SECURITY.md` with a real disclosure address.
  *Commit:* `chore: add govulncheck to CI`

- [ ] **6. Badges & README final pass.** CI status, `pkg.go.dev` reference,
  Go report card, licence, latest release. Above the fold: what forge is, why it
  exists, a 10-line example, and a comparison table against `pgx` and `lib/pq`
  that does not oversell. Link the M14 migration write-up.
  *Commit:* `docs: README badges and comparison`

- [ ] **7. Release automation.** `.github/workflows/release.yml` triggered on
  `v*` tags: run the full suite, build the CLI matrix from M16, generate release
  notes from the changelog section, attach checksums (`sha256sum`), publish the
  GitHub Release. Sign the checksums file if you have a key.
  *Commit:* `ci: automate tagged releases`

- [ ] **8. Tag it.**
  ```bash
  git switch main && git pull --ff-only
  go test ./... && go test -race ./... && golangci-lint run ./...
  git tag -a v0.1.0 -m "forge v0.1.0"
  git push origin v0.1.0
  GOPROXY=proxy.golang.org go list -m github.com/quriny-platform/forge@v0.1.0
  ```
  That last command warms the module proxy and is what makes pkg.go.dev index
  you. Confirm the docs render, examples appear, and the licence is detected.

- [ ] **9. Maintainership scaffolding.** A `docs/ROADMAP-next.md` for post-v0.1
  work (logical replication, composite types, MySQL dialect, `database/sql`
  shim). Label the issue tracker (`good first issue`, `help wanted`). Decide and
  write down your review policy and response-time expectation — the parts of open
  source that are not code.
  *Commit:* `docs: post-v0.1 roadmap and maintainer policy`

- [ ] **10. Announce.** A short write-up: what forge does, the design calls
  behind the wire-protocol implementation, the pgx comparison numbers from M14,
  and the known trade-offs. Publish it alongside the release.

## Security (OWASP)

**Risks: [A06 Vulnerable Components](https://owasp.org/Top10/A06_2021-Vulnerable_and_Outdated_Components/) · [A08 Integrity Failures](https://owasp.org/Top10/A08_2021-Software_and_Data_Integrity_Failures/) · [CI/CD Top 10](https://owasp.org/www-project-top-10-ci-cd-security-risks/)**

At release, forge becomes part of someone else's supply chain. References:
[Vulnerable Dependency Management](https://cheatsheetseries.owasp.org/cheatsheets/Vulnerable_Dependency_Management_Cheat_Sheet.html) ·
[SCVS](https://owasp.org/www-project-software-component-verification-standard/).

- [ ] **Full security review before tagging.** Walk
  [SECURITY-PRACTICES.md](../SECURITY-PRACTICES.md) §2 and §3 top to bottom and
  confirm each control has a test. Anything unverified goes in the changelog's
  *Known limitations*, not in a claim.
- [ ] **`govulncheck ./...` clean**, in CI and at tag time.
- [ ] **Publish an SBOM** (CycloneDX or SPDX) as a release asset. With a
  stdlib-only runtime tree this is a short file and a strong claim — make it,
  and let consumers verify it.
- [ ] **Sign the checksums.** `sha256sum` of every artefact, signed (minisign,
  cosign, or GPG), with the public key in the README. Unsigned binaries on a
  GitHub Release are an A08 finding waiting to happen.
- [ ] **Release workflow uses a scoped, short-lived token.** `contents: write`
  on the release job only; no long-lived PAT; actions still pinned by SHA
  (CICD-SEC-3, CICD-SEC-5).
- [ ] **Document the security posture honestly in the README:** what forge
  defends against (TB1–TB4), what it does not (the non-goals), and the recommended
  configuration (`sslmode=verify-full`, `scram-sha-256`, args never logged).
  Over-claiming is itself a security problem — it stops users from adding the
  controls forge lacks.
- [ ] **`SECURITY.md` is real before v0.1.0 ships.** Disclosure address,
  response-time commitment, supported-version table, and a statement that you
  will credit reporters.
- [ ] **Set up private vulnerability reporting** on the repo and subscribe to
  `golang-announce` for stdlib CVEs — a stdlib-only project inherits stdlib CVEs.
- [ ] **Write the security section of the release notes** as a reader would want
  it: what is hardened, what is fuzzed, what is untested.

**Verify:** a clean-machine `go install` of the tagged version; checksum
signature verifies; SBOM lists exactly the dependencies you claim; `govulncheck`
clean.

## Done when

- `pkg.go.dev/github.com/quriny-platform/forge` renders with examples and no
  “no documentation” gaps.
- `go install github.com/quriny-platform/forge/cmd/forge@latest` works on a
  clean machine.
- A GitHub Release exists with binaries and checksums.
- `govulncheck` clean in CI.

## Stretch

- Cut `v0.2.0` from real user feedback rather than your own roadmap.
- Add a `fuzz` CI job running each target for 10 minutes nightly, with crashers
  auto-filed as issues.
