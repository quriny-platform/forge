# M16 — The `forge` CLI  `feat/cli`

**Covers:** CLI ergonomics · testable `main` · `flag` subcommands · signal
handling · output formatting · script-driven testing

**Links**
- [Command Line Interface Guidelines](https://clig.dev/) — read it once, end to end
- [`flag` package](https://pkg.go.dev/flag) · [`flag.FlagSet`](https://pkg.go.dev/flag#FlagSet)
- [`signal.NotifyContext`](https://pkg.go.dev/os/signal#NotifyContext)
- [`testscript`](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
- [Learn Go with Tests — Command line & package structure](https://quii.gitbook.io/learn-go-with-tests/build-an-application/command-line)
- [`text/tabwriter`](https://pkg.go.dev/text/tabwriter)

**Package:** `cmd/forge/` (thin `main`) + `internal/cli/` (everything testable).

> Deliberately **no cobra**. Subcommand dispatch over `flag.FlagSet` is 60 lines
> and keeps dispatch explicit rather than framework-hidden. Reconsider only if
> the CLI outgrows ~8 commands.

---

## The testable-main pattern

```go
// cmd/forge/main.go — the entire file
func main() { os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
```

Everything else lives in `internal/cli` where `Run` is an ordinary function
returning an exit code. No globals, no `os.Exit` outside `main`, no
`log.Fatal` anywhere. That single constraint makes the whole CLI unit-testable
with `bytes.Buffer`s — write the test that proves it before writing `Run`.

Exit codes: `0` success · `1` runtime failure · `2` usage error ·
`3` connection failure · `130` interrupted. Document them in `forge help`.

---

## Steps

- [ ] **1. RED — `Run` contract.** `cli_test.go`: `Run(nil, …)` prints usage to
  **stderr** and returns 2; `Run([]string{"help"}, …)` prints usage to **stdout**
  and returns 0 (usage-on-request goes to stdout, usage-on-error to stderr —
  clig.dev is explicit about this); `Run([]string{"nope"}, …)` returns 2 and the
  message names the unknown command and suggests `forge help`;
  `Run([]string{"version"}, …)` prints a version line and returns 0.

- [ ] **2. GREEN — dispatch.**
  ```go
  type command struct {
      name, summary string
      run  func(ctx context.Context, env *Env, args []string) error
  }
  type Env struct{ Stdin io.Reader; Stdout, Stderr io.Writer; DSN string }
  func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int
  ```
  `Run` builds a `context.Context` from `signal.NotifyContext(ctx,
  os.Interrupt, syscall.SIGTERM)`, looks up the subcommand, runs it, maps the
  returned error to an exit code (`errors.Is(err, context.Canceled)` → 130,
  `*pgconn.PgError` → 1, `usageError` → 2, dial failure → 3). Version comes from
  `runtime/debug.ReadBuildInfo()` so it is correct for `go install` builds
  without ldflags.
  *Commit:* `feat(cli): subcommand dispatch and exit codes`

- [ ] **3. RED/GREEN — global flags & DSN resolution.** Precedence:
  `--dsn` flag > `FORGE_DSN` env > `DATABASE_URL` env > `PG*` env vars > error
  with a message showing all four options. Test each level and the precedence
  between them. `forge --help` and `forge <cmd> --help` both work.
  *Commit:* `feat(cli): DSN resolution with documented precedence`

- [ ] **4. RED/GREEN — `forge ping`.** `forge ping [--timeout 5s]` connects,
  pings, prints `ok  postgres 17.2  1.2ms` and returns 0; on failure prints a
  diagnosis to stderr (`connection refused — is postgres running on localhost:5432?`)
  and returns 3. Errors should suggest the fix, not just state the failure.
  Test against a real server behind the `integration` tag and against a mock
  otherwise.
  *Commit:* `feat(cli): ping command`

- [ ] **5. RED/GREEN — `forge query`.** `forge query [--format table|json|csv]
  [--param v]... "<sql>"`, reading SQL from `stdin` when the argument is `-`.
  - `table` (default when stdout is a TTY): `text/tabwriter`, NULL rendered as a
    dim `NULL`, column headers, row count footer.
  - `json`: an array of objects, streamed — do not buffer the result set.
  - `csv`: `encoding/csv`, NULL as an empty field, with `--csv-null` to override.
  Auto-select `json` when stdout is **not** a TTY (`golang.org/x/term.IsTerminal`
  or a stat on `os.Stdout` — stdlib-only via `os.Stdout.Stat()` and
  `ModeCharDevice`). Machine-readable by default when piped is the single most
  useful CLI behaviour you can implement.
  Test by passing `bytes.Buffer` and asserting exact output.
  *Commits:* `feat(cli): query command`, `feat(cli): table, json and csv formatters`

- [ ] **6. RED/GREEN — `forge copy`.** `forge copy --table users --format csv
  [file]` streaming a CSV file (or stdin) into `CopyFrom` from M15, with a
  progress line on stderr every second (never on stdout — stdout is data).
  `--dry-run` parses and validates without writing.
  *Commit:* `feat(cli): bulk copy command`

- [ ] **7. RED/GREEN — signals.** Send `SIGINT` mid-query: the CLI cancels the
  context (which fires forge's `CancelRequest` from M09), prints
  `canceled` to stderr, and exits 130. Second `SIGINT` exits immediately.
  Test with `testscript` or by invoking `Run` in a goroutine and cancelling.
  *Commit:* `feat(cli): graceful interrupt handling`

- [ ] **8. RED/GREEN — script tests.** `testdata/script/*.txtar` driven by
  `testscript`: each file is a small shell-like scenario with expected stdout,
  stderr and exit code. Start with `usage.txtar`, `query_json.txtar`,
  `bad_dsn.txtar`. These read like documentation and catch output regressions no
  unit test will.
  *Commit:* `test(cli): script-driven end-to-end tests`

- [ ] **9. Polish.** `forge help <cmd>` per-command help with examples;
  `--no-color` and `NO_COLOR` env support; colours only when stdout is a TTY;
  a `forge version --json` for tooling.
  *Commit:* `feat(cli): help, color and version polish`

- [ ] **10. Ship it.** Add a `goreleaser` config (or a plain
  `GOOS/GOARCH` build matrix in a workflow) producing darwin/linux
  amd64+arm64 binaries on tag. Wire it in at M17 but write the config here.
  *Commit:* `chore(cli): release build configuration`

## Security (OWASP)

**Risks: [A09 Logging Failures](https://owasp.org/Top10/A09_2021-Security_Logging_and_Monitoring_Failures/) · [A05 Misconfiguration](https://owasp.org/Top10/A05_2021-Security_Misconfiguration/) · [Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)**

A CLI leaks credentials in ways a library cannot: process listings, shell
history, terminal scrollback, CI logs.

- [ ] **`--dsn` on the command line is visible in `ps` to every user on the
  host.** Support it, but document the hazard and prefer `FORGE_DSN`,
  `DATABASE_URL`, `PGPASSFILE`, or a `--dsn-file` flag. Consider reading a
  password from the terminal with echo disabled when none is configured.
- [ ] **Never echo the DSN.** Not in `--help`, not in verbose mode, not in an
  error. Connection errors name host and port only. Test it:
  `TestCLINeverPrintsPassword` runs every command with a password-bearing DSN and
  greps all output.
- [ ] **`forge version --json` and any diagnostic output must be scrubbed** —
  a "paste your output in the issue" workflow is a credential-disclosure pipeline.
- [ ] **stdout is data, stderr is chatter.** Progress, warnings, and diagnostics
  never touch stdout — a caller piping `forge query` into `jq` must not receive a
  warning line. This is correctness *and* it prevents log content contaminating
  data pipelines.
- [ ] **Respect file permissions.** A `--dsn-file` that is world-readable earns a
  warning; a client key file likewise (mirrors M05).
- [ ] **Do not write query results to a temp file** without `0600` and explicit
  cleanup. Prefer streaming to stdout.
- [ ] **Exit codes must not encode data.** Use the documented set; never
  `os.Exit(rowCount)`.
- [ ] **`forge query` is an interactive foot-gun by design** — it takes raw SQL,
  as `psql` does. Document that it is a developer tool and should not be wrapped
  by a service that passes user input into it. If you add `--param`, values go
  through parameters, never string interpolation.
- [ ] **Signals must not leave a partial write** that a consumer reads as
  complete. Flush and exit cleanly on `SIGINT`.

**Verify:** `TestCLINeverPrintsPassword` across every subcommand and both success
and failure paths; a `testscript` case asserting warnings land on stderr.

## Done when

- `main.go` is one line; `internal/cli` is 100% driven by injected streams.
- No `os.Exit`, `log.Fatal`, or `panic` outside `main`.
- Piped output is machine-readable without a flag; TTY output is human-readable.
- `testscript` suite green.

## Stretch

- `forge repl` — an interactive prompt with history and multiline SQL.
- `forge explain "<sql>"` rendering the plan tree with box-drawing characters.
- Shell completions for bash/zsh/fish generated by a hidden subcommand.
