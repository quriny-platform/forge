# M03 — Connection layer & simple query  `feat/pgconn-simple-query`

**Covers:** `net.Conn` · protocol state machines · test doubles / scripted mocks ·
DSN parsing · connection lifecycle

**Links**
- [Message Flow — start-up & simple query](https://www.postgresql.org/docs/current/protocol-flow.html)
- [libpq connection strings](https://www.postgresql.org/docs/current/libpq-connect.html)
- [`net` package](https://pkg.go.dev/net) · [`net.Pipe`](https://pkg.go.dev/net#Pipe)
- [Learn Go with Tests — Mocking](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/mocking)
- [Learn Go with Tests — HTTP server / dependency injection](https://quii.gitbook.io/learn-go-with-tests/build-an-application/http-server)

**Package:** `postgres/pgconn` + `internal/testutil`.
**Imports allowed:** stdlib, `pgwire`, `pgtype` (not yet built — none needed here).

---

## Steps

- [ ] **1. RED — the scripted backend.** Before any connection code, write the
  test tool. `internal/testutil/script_test.go` asserts that the harness itself
  behaves: a script expecting `Query{"SELECT 1"}` and replying with
  `RowDescription`+`DataRow`+`CommandComplete`+`ReadyForQuery` fails loudly when
  the client sends something else.

- [ ] **2. GREEN — `internal/testutil/script.go`.** Design:
  ```go
  type Step interface{ Step(*pgwire.Frontend) error }   // expectation or reply
  type Script struct{ Steps []Step }
  func (s *Script) Run(backend *pgwire.Backend) error
  func (s *Script) Server() (net.Conn, chan error)      // net.Pipe based
  ```
  `ExpectMessage(want pgwire.FrontendMessage)` compares with `reflect.DeepEqual`
  and reports a diff; `SendMessage(msg pgwire.BackendMessage)` writes one.
  Build it on `net.Pipe()` so there is no OS socket, no port, no flake, and tests
  run in microseconds. Add `ExpectAnyStartup()` sugar since startup parameters
  vary. This harness is used by every later milestone — invest in its error messages.
  *Commit:* `test: add scripted mock backend harness`

- [ ] **3. RED — config parsing.** `config_test.go`, table-driven over both forms:
  URL (`postgres://user:pw@host:5432/db?sslmode=disable&connect_timeout=5`) and
  keyword/value (`host=/var/run/postgresql user=x dbname=y`). Assert defaults
  (port 5432, user from `os/user`, dbname = user, host = `/tmp` socket or
  `localhost`), env fallback (`PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`,
  `PGDATABASE`, `PGSSLMODE`, `PGAPPNAME`), precedence (explicit > env > default),
  multi-host (`host=a,b port=5432,5433`), and — critically —
  **`Config.String()` must not contain the password.**

- [ ] **4. GREEN — `config.go`.**
  ```go
  type Config struct {
      Host, User, Password, Database string
      Port uint16
      ConnectTimeout time.Duration
      RuntimeParams map[string]string   // application_name, search_path, …
      TLSConfig *tls.Config             // nil = no TLS; filled in M05
      Fallbacks []*FallbackConfig
      DialFunc  DialFunc                // func(ctx, network, addr) (net.Conn, error)
      OnNotice  func(*PgConn, *Notice)
      Tracer    QueryTracer             // interface stub, implemented in tracelog
  }
  func ParseConfig(dsn string) (*Config, error)
  ```
  `String()` renders every field except `Password`, which prints as `xxxxx`.
  Add a `Copy()` returning a deep clone — later layers must not mutate a shared config.
  *Commit:* `feat(pgconn): parse DSN, URL and environment configuration`

- [ ] **5. RED — startup handshake.** `pgconn_test.go`: script expects
  `StartupMessage` with `user` and `database` parameters, replies
  `AuthenticationOk` → three `ParameterStatus` → `BackendKeyData` →
  `ReadyForQuery{'I'}`. Assert `Connect` returns a live conn whose
  `ParameterStatus("server_version")` is set and whose `PID()`/`SecretKey()` are
  the ones sent. Second case: server replies `ErrorResponse` (SQLSTATE `28P01`)
  — `Connect` must return a `*pgconn.PgError` with `.Code == "28P01"` and the
  underlying socket must be closed (assert with a `closeTracking` wrapper).

- [ ] **6. GREEN — `pgconn.go` connect path.**
  ```go
  type PgConn struct {
      conn        net.Conn
      frontend    *pgwire.Frontend
      config      *Config
      pid, secret uint32
      parameters  map[string]string
      txStatus    byte
      status      connStatus   // uninitialized|idle|busy|closed
      // …
  }
  func Connect(ctx context.Context, dsn string) (*PgConn, error)
  func ConnectConfig(ctx context.Context, cfg *Config) (*PgConn, error)
  ```
  Sequence: dial (honouring `ctx` and `ConnectTimeout` — use `net.Dialer` with
  `DialContext`) → send `StartupMessage{ProtocolVersion: 196608, Parameters:
  {"user":…, "database":…, plus RuntimeParams}}` → loop on `Receive()`:
  `AuthenticationOk` sets authenticated; any other `Authentication*` returns
  `errors.New("unsupported auth")` **for now** (M04 fills these in);
  `ParameterStatus` accumulates; `BackendKeyData` stores pid/secret;
  `ReadyForQuery` ends the loop; `ErrorResponse` converts to `*PgError` and aborts.
  On any error, close the socket before returning — a leaked socket is the classic
  bug here, and your `closeTracking` fake catches it.
  *Commit:* `feat(pgconn): implement startup handshake`

- [ ] **7. RED — `PgError`.** `errors_test.go` in `pgconn`: `*PgError` implements
  `error`, exposes SQLSTATE `Code`, `Message`, `Detail`, `Hint`, `Position`,
  `ConstraintName`; `errors.As` finds it through a `fmt.Errorf("%w")` wrap;
  `Error()` renders `severity: message (SQLSTATE code)`; a helper
  `pgconn.SQLState(err) string` returns `""` for non-Pg errors.

- [ ] **8. GREEN.** Convert `pgwire.ErrorResponse` → `*PgError` in one place.
  Do not embed the wire type in the public error — the layers stay separable.
  *Commit:* `feat(pgconn): add PgError with SQLSTATE access`

- [ ] **9. RED — simple query.** Scripted cases:
  (a) `SELECT` returning two rows → `ResultReader` yields both, correct
  `FieldDescriptions`, `CommandTag` = `SELECT 2`;
  (b) `INSERT` → `RowsAffected() == 1`;
  (c) multi-statement `"SELECT 1; SELECT 2"` → `MultiResultReader` yields two
  results (this is the reason the simple protocol still matters);
  (d) mid-stream `ErrorResponse` → the reader surfaces the error *and* still
  consumes to `ReadyForQuery` so the connection stays usable;
  (e) calling `Exec` while a reader is open returns `ErrConnBusy`.

- [ ] **10. GREEN — query path.**
  ```go
  func (c *PgConn) Exec(ctx context.Context, sql string) *MultiResultReader
  type MultiResultReader struct{ /* … */ }
  func (m *MultiResultReader) NextResult() bool
  func (m *MultiResultReader) ResultReader() *ResultReader
  func (m *MultiResultReader) Close() (CommandTag, error)
  type ResultReader struct{ /* … */ }
  func (r *ResultReader) NextRow() bool
  func (r *ResultReader) Values() [][]byte
  func (r *ResultReader) FieldDescriptions() []pgwire.FieldDescription
  func (r *ResultReader) Close() (CommandTag, error)
  ```
  Note the shape: readers return themselves rather than `(x, error)`, and errors
  surface at `Close()`. That mirrors pgx and makes the state machine explicit.
  The connection carries a `status` field; every entry point checks it and every
  exit path restores `idle` — including error paths. Write the transition table
  in a comment above the type:
  | state | event | next |
  |---|---|---|
  | idle | Exec | busy |
  | busy | ReadyForQuery | idle |
  | busy | io error | closed |
  *Commits:* `feat(pgconn): simple query result readers`, `feat(pgconn): connection status machine`

- [ ] **11. REFACTOR.** Pull the “drain until ReadyForQuery” loop into one
  unexported method used by every path. Ensure a returned `CommandTag` parses
  `INSERT 0 5` into `RowsAffected() == 5` (a `CommandTag []byte` type with
  `RowsAffected()`, `Insert()`, `Update()`, `Select()` predicates).
  *Commit:* `refactor(pgconn): centralise ready-for-query draining`

## Security (OWASP)

**Risks: [A09 Logging Failures](https://owasp.org/Top10/A09_2021-Security_Logging_and_Monitoring_Failures/) · [A05 Misconfiguration](https://owasp.org/Top10/A05_2021-Security_Misconfiguration/) · [A10 SSRF](https://owasp.org/Top10/A10_2021-Server-Side_Request_Forgery_%28SSRF%29/)**

`Config` is where credentials enter forge. Everything about it is a security
control. See [Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html).

- [ ] **`Config.String()` masks the password** — already in step 3; treat that
  test as mandatory, not illustrative. Add the same for any `MarshalJSON` you
  write, and consider making `Password` a distinct type whose `String()`/
  `Format()` methods render `xxxxx` so redaction survives careless `%v`.
- [ ] **No DSN in error messages.** A failed connect must report host and port,
  never the full DSN — DSNs land in logs, tickets, and screenshots.
  Test: `TestConnectErrorHasNoPassword`.
- [ ] **Connection targets come only from caller config (A10).** Nothing the
  server sends may influence where forge connects next. Note it in a comment on
  the fallback loop; it becomes load-bearing at M05.
- [ ] **Unix socket permissions.** When `host` is a path, document that forge
  relies on filesystem permissions for authentication and does not check them.
- [ ] **Fail closed on unknown config keys.** An unrecognised DSN parameter is an
  error, not a silent ignore — `sslmode=requrie` must not degrade to plaintext.
  Test the typo case explicitly; this exact bug has caused real breaches.
- [ ] **Bound the startup loop.** A server that sends `ParameterStatus` forever
  before `ReadyForQuery` must hit a message-count or deadline cap, not grow the
  parameter map without limit.
- [ ] Prefer reading the password from the environment or a file
  (`PGPASSFILE`) over the DSN, and say so in the docs — DSNs end up in shell
  history and process listings.

**Verify:** `TestNoCredentialLeak` captures every byte written to the logger and
every error string across a failed handshake; the password appears in neither.

## Done when

- Every test runs on `net.Pipe` — no Docker needed yet, whole package under 200 ms.
- Killing the mock mid-message surfaces an error and marks the conn closed, never hangs.
- `Config.String()` has a test proving the password is masked.

## Stretch

- Add `PgConn.Deallocate`/`Ping` and a `NotificationHandler` for `LISTEN`/`NOTIFY`.
- Record a `docs/adr/0005-result-reader-shape.md` on the “errors at Close()” choice.
