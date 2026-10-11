# M05 — TLS & sslmode  `feat/tls-sslmode`

**Covers:** TLS handshakes · X.509 verification · protocol upgrade negotiation ·
security defaults · matrix testing

**Links**
- [libpq SSL support & sslmode table](https://www.postgresql.org/docs/current/libpq-ssl.html)
- [Protocol flow — SSL session encryption](https://www.postgresql.org/docs/current/protocol-flow.html#PROTOCOL-FLOW-SSL)
- [`crypto/tls`](https://pkg.go.dev/crypto/tls) · [`crypto/x509`](https://pkg.go.dev/crypto/x509)
- [`tls.Config` `VerifyPeerCertificate`](https://pkg.go.dev/crypto/tls#Config)

**Package:** `postgres/pgconn` (`config_tls.go`).

---

## The negotiation

Before the startup message, the client sends `SSLRequest` (8 bytes: length 8,
int32 `80877103`). The server answers with **one byte**: `'S'` (proceed with TLS)
or `'N'` (no TLS available). Only after `'S'` do you wrap the socket in
`tls.Client` and send `StartupMessage` over the encrypted connection. A one-byte
reply is unusual enough that it needs its own read path — not `Frontend.Receive`.

| sslmode | try TLS | require TLS | verify CA | verify hostname |
|---|---|---|---|---|
| `disable` | no | — | — | — |
| `allow` | only after plaintext fails | no | no | no |
| `prefer` (default) | yes | no | no | no |
| `require` | yes | yes | no | no |
| `verify-ca` | yes | yes | yes | no |
| `verify-full` | yes | yes | yes | yes |

`prefer`/`require` without verification are encryption without authentication —
they stop passive sniffing, not an active MITM. Say so in the doc comment.

---

## Steps

- [ ] **1. RED — sslmode parsing matrix.** `config_tls_test.go`, one table row per
  mode × (`sslrootcert` present/absent) × (`sslcert`+`sslkey` present/absent).
  Assert the produced `[]*tls.Config` (a slice — `prefer` yields TLS *then*
  plaintext fallback) has the right `InsecureSkipVerify`, `ServerName`, and
  `RootCAs`. Explicit assertions: `verify-full` sets `ServerName` to the host and
  `InsecureSkipVerify=false`; `require` sets `InsecureSkipVerify=true` **with a
  custom `VerifyPeerCertificate` that still checks nothing** (document why);
  `disable` yields a nil TLS config; an unknown mode is an error, not a default.

- [ ] **2. GREEN — `configTLS`.**
  ```go
  func configTLS(settings map[string]string, host string) ([]*tls.Config, error)
  ```
  Inputs from DSN or env: `sslmode`, `sslrootcert` (`PGSSLROOTCERT`), `sslcert`,
  `sslkey`, `sslpassword`, `sslsni`. Load the root pool with
  `x509.NewCertPool()` + `AppendCertsFromPEM`; error if the file parses to zero
  certs (silent empty pool is a real-world footgun). Load client certs with
  `tls.LoadX509KeyPair`; support encrypted keys by decrypting with `sslpassword`
  or returning a clear error if you choose not to.
  Set `MinVersion: tls.VersionTLS12` always. Never leave a bare
  `InsecureSkipVerify: true` without an accompanying comment naming the sslmode
  that justifies it — `gosec` will flag it and you should have to think each time.
  *Commit:* `feat(pgconn): build tls.Config from sslmode settings`

- [ ] **3. RED — the `'S'`/`'N'` handshake.** Using the mock: server replies `'N'`
  when the client requested `require` → `Connect` errors with “server refused TLS”;
  replies `'N'` under `prefer` → connection continues in plaintext; replies `'S'`
  → client proceeds with `tls.Client`. Test the truncated case too: server closes
  after zero bytes → `io.ErrUnexpectedEOF`, socket closed.

- [ ] **4. GREEN — `startTLS`.** In the connect path, before the startup message:
  write `SSLRequest` via the raw `net.Conn`, `io.ReadFull` exactly one byte,
  switch on it, and on `'S'` replace `c.conn` with `tls.Client(conn, tlsCfg)` and
  call `HandshakeContext(ctx)` explicitly (so cancellation works) before
  recreating the `pgwire.Frontend` over the new conn. Set `c.secure = true`,
  which retires the M04 `// TODO(M05)` on cleartext auth.
  *Commit:* `feat(pgconn): negotiate TLS before startup`

- [ ] **5. RED — fallback chain.** `prefer` must produce two connect attempts.
  Test with two scripted servers (or one that fails the first attempt): assert
  attempt order, and that the *first* error is returned when all attempts fail
  (wrapped with `errors.Join` so all are inspectable).

- [ ] **6. GREEN — `Fallbacks`.** Generalise `Config.Fallbacks []*FallbackConfig`
  (`Host`, `Port`, `TLSConfig`) built from the sslmode slice × multi-host list.
  `connect` loops over `[primary] + fallbacks`, honouring `ctx` between attempts.
  Return `errors.Join(all...)` on total failure so `errors.As` still finds the
  `*PgError` from the most informative attempt.
  *Commit:* `feat(pgconn): connection fallback chain`

- [ ] **7. RED/GREEN — real TLS in tests.** Generate a throwaway CA + server cert
  at test time with `crypto/x509.CreateCertificate` (no fixtures on disk, no
  expiry surprises in two years) and run a real `tls.Server` over `net.Pipe`.
  Assert `verify-full` succeeds against the right hostname and fails with an
  `x509.HostnameError` against the wrong one.
  *Commit:* `test(pgconn): end-to-end TLS with generated certificates`

- [ ] **8. Integration.** Extend `docker-compose.yml` with a second Postgres
  service configured for `ssl=on` and `hostssl ... scram-sha-256` in
  `pg_hba.conf`, with certs generated by an init script. Add
  `//go:build integration` tests connecting under `require` and `verify-ca`.
  *Commit:* `test: integration coverage for TLS connections`

- [ ] **9. REFACTOR + ADR.** `docs/adr/0007-tls-defaults.md`: forge defaults to
  `prefer` for libpq compatibility but the README recommends `verify-full`;
  `InsecureSkipVerify` is never settable directly by users, only implied by an
  sslmode they named.

## Security (OWASP)

**Risks: [A02 Cryptographic Failures](https://owasp.org/Top10/A02_2021-Cryptographic_Failures/) · [A05 Misconfiguration](https://owasp.org/Top10/A05_2021-Security_Misconfiguration/) · [A10 SSRF](https://owasp.org/Top10/A10_2021-Server-Side_Request_Forgery_%28SSRF%29/)**

Reference: [Transport Layer Security cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/Transport_Layer_Security_Cheat_Sheet.html). ASVS V9.

- [ ] **`MinVersion: tls.VersionTLS12` always** — `crypto/tls` defaults are
  reasonable but explicit is auditable. Consider TLS 1.3 minimum with a documented
  opt-down for old servers.
- [ ] **`InsecureSkipVerify` is never a user-facing option.** It is implied only
  by an sslmode the user named. Every occurrence in the code carries a comment
  naming that mode. A `Config.TLSConfig` supplied by the caller is used as-is —
  document that forge does not second-guess it.
- [ ] **Document the honest truth about `prefer`/`require`:** encryption without
  authentication stops passive sniffing and nothing else. README recommends
  `verify-full`. This is the single most valuable sentence you will write in this
  milestone.
- [ ] **Refuse the downgrade.** Under `require` and above, a server answering
  `'N'` to `SSLRequest` is an error — never a silent plaintext fallback.
  Test: `TestRequireRefusesPlaintextDowngrade`.
- [ ] **`verify-full` sets `ServerName` from the *configured* host**, never from
  anything the server said (A10). Test the wrong-hostname case yields
  `x509.HostnameError`.
- [ ] **A root cert file that parses to zero certificates is an error**, not an
  empty pool that silently falls back to system roots.
- [ ] **Handshake under the caller's context** — `HandshakeContext(ctx)`, so a
  slow or malicious server cannot pin a goroutine indefinitely (DoS).
- [ ] **Client key files:** check the key is not world-readable and warn (libpq
  refuses outright — decide, document, test).
- [ ] Once TLS state exists, revisit M04: enable channel binding
  (`SCRAM-SHA-256-PLUS`) where the server offers it. It defeats a MITM that
  relays the SCRAM exchange.

**Verify:** the sslmode matrix test passes end to end against a real
`tls.Server` with generated certs; `gosec` clean or every `#nosec` justified.

## Done when

- Every sslmode has a passing test; unknown modes error.
- No code path sets `InsecureSkipVerify` without a mode-naming comment.
- `golangci-lint` (with `gosec`) passes with at most explicitly annotated `#nosec`.

## Stretch

- Implement `sslnegotiation=direct` (PostgreSQL 17: TLS without the `SSLRequest`
  round trip) behind a config flag.
- Add SCRAM channel binding now that a TLS state is available (M04 stretch).
