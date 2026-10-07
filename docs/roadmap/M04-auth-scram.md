# M04 — SCRAM-SHA-256 authentication  `feat/auth-scram`

**Covers:** SASL/SCRAM · HMAC & PBKDF2 · constant-time comparison ·
secret hygiene · testing against published vectors

**Links**
- [RFC 5802 — SCRAM](https://datatracker.ietf.org/doc/html/rfc5802) (§3 algorithm, §5 test vector)
- [RFC 7677 — SCRAM-SHA-256](https://datatracker.ietf.org/doc/html/rfc7677) (§3 has the exact vector to test against)
- [PostgreSQL SASL Authentication](https://www.postgresql.org/docs/current/sasl-authentication.html)
- [`crypto/hmac`](https://pkg.go.dev/crypto/hmac) · [`crypto/subtle`](https://pkg.go.dev/crypto/subtle) · [`crypto/pbkdf2`](https://pkg.go.dev/crypto/pbkdf2)
- [RFC 4013 — SASLprep](https://datatracker.ietf.org/doc/html/rfc4013)

**Package:** `postgres/pgconn` (`auth_scram.go`). **Stdlib only** — Go 1.24+ has
`crypto/pbkdf2`, so no `x/crypto` dependency.

---

## The algorithm, once, in plain terms

```
SaltedPassword  = PBKDF2-HMAC-SHA256(password, salt, iterations, 32)
ClientKey       = HMAC(SaltedPassword, "Client Key")
StoredKey       = SHA256(ClientKey)
AuthMessage     = client-first-bare + "," + server-first + "," + client-final-without-proof
ClientSignature = HMAC(StoredKey, AuthMessage)
ClientProof     = ClientKey XOR ClientSignature
ServerKey       = HMAC(SaltedPassword, "Server Key")
ServerSignature = HMAC(ServerKey, AuthMessage)
```

Wire exchange (all inside `PasswordMessage`-tagged `'p'` frames):

| direction | message | contents |
|---|---|---|
| S→C | `AuthenticationSASL` | mechanism list, contains `SCRAM-SHA-256` |
| C→S | `SASLInitialResponse` | mechanism name + `n,,n=,r=<clientNonce>` |
| S→C | `AuthenticationSASLContinue` | `r=<client+serverNonce>,s=<b64 salt>,i=<iters>` |
| C→S | `SASLResponse` | `c=biws,r=<nonce>,p=<b64 proof>` |
| S→C | `AuthenticationSASLFinal` | `v=<b64 server signature>` |
| S→C | `AuthenticationOk` | — |

`biws` is base64 of `n,,` — the GS2 header echoed back. Verifying the server
signature is **mandatory**: skipping it means a malicious server can impersonate
yours. That verification is why `crypto/subtle` appears in this milestone.

---

## Steps

- [ ] **1. RED — RFC vectors first.** `auth_scram_test.go`, `TestSCRAMVectors`:
  take the exact username/password/nonce/salt/iteration values from RFC 7677 §3
  and assert your `clientFirstMessage`, `clientFinalMessage`, and computed
  `ClientProof`/`ServerSignature` match the RFC's base64 strings byte for byte.
  Injecting the nonce is the point — the type must accept a nonce source so the
  test is deterministic.

- [ ] **2. GREEN — `scramClient`.**
  ```go
  type scramClient struct {
      serverAuthMechanisms []string
      password             []byte
      clientNonce          []byte
      clientFirstMessageBare string
      serverFirstMessage     string
      clientAndServerNonce   []byte
      salt                   []byte
      iterations             int
      saltedPassword         []byte
      authMessage            string
  }
  func newScramClient(mechs []string, password string, nonce []byte) (*scramClient, error)
  func (c *scramClient) clientFirstMessage() []byte
  func (c *scramClient) recvServerFirstMessage(msg []byte) error
  func (c *scramClient) clientFinalMessage() []byte
  func (c *scramClient) recvServerFinalMessage(msg []byte) error
  ```
  Nonce generation for production: 18 bytes from `crypto/rand`, base64-encoded —
  never `math/rand`. Add a package-level `var randReader io.Reader = rand.Reader`
  so tests can substitute; that is the standard seam.
  Parse the server messages strictly: reject a `r=` that does not begin with your
  own client nonce (that check defeats a replay), reject `i=` below 4096, reject
  unknown leading attributes.
  *Commit:* `feat(pgconn): implement SCRAM-SHA-256 client`

- [ ] **3. RED — server signature verification.** Test that a tampered `v=` value
  makes `recvServerFinalMessage` return an error mentioning *invalid server
  signature*, and that the comparison uses `subtle.ConstantTimeCompare` (assert
  behaviourally: correct signature passes, one-bit-flipped fails).

- [ ] **4. GREEN.** Compare with `subtle.ConstantTimeCompare(...) != 1`. Do not
  use `bytes.Equal` here and write a comment saying why: timing leaks on a MAC
  comparison are a textbook side channel.
  *Commit:* `feat(pgconn): verify server signature in constant time`

- [ ] **5. RED — the wire integration.** Extend the M03 scripted backend: server
  sends `AuthenticationSASL{["SCRAM-SHA-256"]}`, expects `SASLInitialResponse`,
  replies `AuthenticationSASLContinue`, expects `SASLResponse`, replies
  `AuthenticationSASLFinal` then `AuthenticationOk` then `ReadyForQuery`.
  Assert `Connect` succeeds. Negative case: wrong password → server sends
  `ErrorResponse` `28P01` → `Connect` returns a `*PgError`, and
  **assert the returned error string does not contain the password**.

- [ ] **6. GREEN — wire it into the connect loop.** In the handshake switch from
  M03, add cases for `AuthenticationSASL`, `AuthenticationSASLContinue`,
  `AuthenticationSASLFinal`. Keep the SCRAM state in a local variable inside
  `Connect`, not on `PgConn` — the credential material must not outlive the
  handshake. Zero the `saltedPassword` slice when done (`clear(...)`).
  *Commit:* `feat(pgconn): negotiate SCRAM during startup`

- [ ] **7. RED/GREEN — the other auth methods.**
  - `AuthenticationCleartextPassword`: send `PasswordMessage{password}` — but
    **only** if the connection is TLS-protected or the host is a Unix socket;
    otherwise return an error telling the user to enable TLS. (M05 provides the
    flag; stub the check now with a `c.isSecure()` returning false and a `//
    TODO(M05)`.)
  - `AuthenticationMD5Password`: `"md5" + hex(md5(hex(md5(password+user)) + salt))`.
    Implement it, and add a doc comment stating MD5 auth is deprecated by
    PostgreSQL and offered only for legacy servers.
  - Any other `Authentication*` code: a clear `unsupported authentication method
    %d` error, never a silent success.
  *Commits:* `feat(pgconn): cleartext and md5 authentication`

- [ ] **8. RED — no secrets anywhere.** `TestNoCredentialLeak`: build a `Config`
  with password `"hunter2"`, run a failing handshake against the mock while
  capturing everything written to the configured logger and to the returned
  error, and assert `"hunter2"` appears in neither. Also assert
  `pgwire.PasswordMessage.String()` renders `[redacted]` (from M02 step 10).

- [ ] **9. REFACTOR.** Extract `saslprep`-lite: for now, reject passwords with
  non-ASCII characters and a `// TODO` linking RFC 4013, rather than silently
  mis-encoding them. Note the limitation in the README's known-issues section.
  *Commit:* `refactor(pgconn): guard non-ASCII passwords pending SASLprep`

- [ ] **10. ADR.** `docs/adr/0006-auth-methods-supported.md`: which methods forge
  supports, why cleartext requires TLS, why SASLprep is deferred.

## Security (OWASP)

**Risks: [A02 Cryptographic Failures](https://owasp.org/Top10/A02_2021-Cryptographic_Failures/) · [A07 Authentication Failures](https://owasp.org/Top10/A07_2021-Identification_and_Authentication_Failures/)**

The whole milestone is a security control. ASVS V2 (authentication), V6
(cryptography), V8 (data protection). References:
[Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) ·
[Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) ·
[Cryptographic Storage](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html).

- [ ] **Never roll your own primitive.** `crypto/hmac`, `crypto/sha256`,
  `crypto/pbkdf2`, `crypto/rand` only. If you find yourself writing a loop over
  bytes to XOR-and-hash, stop and check the stdlib first.
- [ ] **Nonces come from `crypto/rand`, 18 bytes minimum.** A `math/rand` nonce
  makes the exchange replayable. Test that the injectable `randReader` defaults
  to `crypto/rand` (assert the package-level var identity).
- [ ] **Verify the server signature — mandatory, with `subtle.ConstantTimeCompare`.**
  Skipping it lets a malicious server impersonate yours; comparing with
  `bytes.Equal` leaks timing. Test both: a flipped bit fails, and the comparison
  function used is the constant-time one.
- [ ] **Reject a server nonce that does not begin with your client nonce.**
  That check is the anti-replay control, and it is one line people omit.
- [ ] **Reject iteration counts below 4096** (RFC 7677 floor). A server
  proposing `i=1` is downgrading you; fail closed.
- [ ] **Fail closed on unknown auth methods.** An `Authentication` code you do
  not implement is an error naming the code — never a silent skip to
  `ReadyForQuery`.
- [ ] **Cleartext password requires TLS or a Unix socket.** No exceptions, no
  opt-out flag. MD5 is supported for legacy servers with a doc comment naming it
  deprecated and a `Config.AllowMD5Auth` opt-in — off by default.
- [ ] **Zero secret material.** `clear(saltedPassword)`, `clear(clientKey)` when
  the handshake ends; keep SCRAM state in a local, never on `PgConn` (ASVS V8).
- [ ] **No timing oracle in error messages.** "authentication failed" — never
  "unknown user" vs "wrong password".

**Verify:** RFC 7677 §3 vectors byte-exact; tampered signature rejected;
`grep -riE 'password|secret' <diff>` shows no formatting verb applied to one.

## Done when

- RFC 7677 vectors pass exactly.
- `grep -ri password` over the diff shows no formatting verb applied to a secret.
- Integration test (once M05 lands) authenticates against real PostgreSQL with
  `scram-sha-256` in `pg_hba.conf`.

## Stretch

- Implement channel binding (`SCRAM-SHA-256-PLUS`) using
  `tls.ConnectionState().TLSUnique` / exporter — after M05.
- Fuzz the server-message parser: `FuzzRecvServerFirstMessage` must never panic.
