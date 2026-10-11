**Milestone:** M<!-- 00 -->
**Summary:**

<!-- One line. What changed and why. -->

---

## Definition of done

- [ ] All new behaviour arrived test-first; no untested exported function.
- [ ] `go test ./...` green; `go test -race ./...` green.
- [ ] `gofmt -l -s -w .` produces no output.
- [ ] `golangci-lint run ./...` clean.
- [ ] Exported identifiers have doc comments starting with the identifier name.
- [ ] No credentials, DSN passwords, or query argument values in any log line or
      error string.
- [ ] Import direction respected — see the layer rules in
      [CONTRIBUTING.md](../CONTRIBUTING.md).
- [ ] ADR written if anything diverges from pgx or `database/sql/driver`, and
      [docs/adr/README.md](../docs/adr/README.md) updated in the same commit.

## Security gates

- [ ] The milestone's `## Security (OWASP)` section is worked through, not
      skimmed.
- [ ] At least one *negative* test exists — something an attacker would try,
      asserted to fail safely.
- [ ] `grep -nE '%[vsq].*(assword|ecret|oken|DSN|args)'` over the diff returns
      nothing.
- [ ] `gosec` clean, or every `#nosec` carries a one-line justification.
- [ ] Any new parsing of untrusted bytes has a fuzz target.
- [ ] `govulncheck ./...` clean.
- [ ] Changes read against
      [docs/SECURITY-PRACTICES.md](../docs/SECURITY-PRACTICES.md).
- [ ] New exported APIs classified against TB1–TB4 in
      [ADR 0002](../docs/adr/0002-security-boundaries.md).

## Notes for the reviewer

<!-- Anything non-obvious: a deliberate divergence, a deferred follow-up, a
     benchmark number, a decision you are unsure about. Delete if empty. -->
