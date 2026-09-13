# HTTP invariant harness

The harness is an independent Go module under `harness/`. It tests a server
under test (SUT) over HTTP; configured external runs never link to the SUT or talk to its
backing store. The counter suite also links its memory reference for the
explicit in-process fallback. The contract and invariant documents under `harness/suites/`
define the protocol an implementation in any language, on any engine, must
provide. [Harness architecture](../harness/docs/architecture.md) is the current design
authority. [Suite authoring](../harness/docs/suite-authoring.md) describes the
new-pattern procedure; [work log](../harness/docs/work-log.md) preserves the
historical plan and verification. Read the architecture before changing code.

## Current package flow

`suitekit.Main` (a suite's `TestMain`) reads `HARNESS_TARGET` (a TOML target
file) or `HARNESS_URL` plus metadata variables. If the target file has a
`cmd`, Main first refuses to run when something already answers at the
target URL, then starts the SUT itself (package `process`, process group of its
own, output to `<results>/sut.log`), waits for `GET /healthz`, opens the
results sink, runs the suite, stops the SUT, and exports. With
`suitekit.InProcess(handler)` and no target in the environment, the suite runs
against the handler on a loopback `httptest` server instead of skipping, so
`go test ./...` exercises the counter suite; those in-process results are
discarded unless `HARNESS_RESULTS` is set. `suitekit.New` resets the SUT with
`POST /_reset`, checks health, binds an `httpclient.Client`, and registers cleanup
that records the test row and flushes results.

Package responsibilities:

- `harness/suitekit`: suite lifecycle (`suite_lifecycle.go`), per-test context
  (`test_context.go`), target loading, health checks, paths, and restart, each
  in a named file. The counter suite separates setup, helpers, contract,
  concurrency, and crash tests; scaffolding uses the same setup/contract/
  concurrency filenames.
- `harness/httpclient`: JSON-aware HTTP requests, path-template escaping, per-request
  timeout options, and exactly one `SampleRow` per call. A non-2xx response
  is a response, not an error; transport, timeout, and decode failures are.
- `harness/load`: barrier-started closed-loop workers (`Closed`, `ClosedFor`)
  and phase labels. `Result.Conclusive` fails the test if any request
  errored, because a final-state invariant cannot be judged then.
- `harness/check`: records every invariant evaluation and fails the test on
  a false result; `Metric` records numbers that are reported, not asserted.
- `harness/results`: flat run/test/check/sample/metric rows, a DuckDB-backed
  in-memory sink, Parquet export under `artifacts/runs/<run_id>/`, and `Query`
  views over every run. One collector goroutine owns the appenders.
- `harness/process`: start, kill (SIGKILL the process group, so a `go run`
  grandchild's listener really closes), stop with grace, and exit
  notification for one SUT process.
- `examples/counter/`: correct implementation only — `Store`, `NewHandler`,
  memory and engine implementations, ordinary unit tests, and a normal
  command (`--engine`, `--dsn`). The user's work belongs here.
- `harness/internal/counterfault`: deliberate `LostUpdate`, `DropReset`,
  `WriteBehind`, and `Slow` decorators, their tests, and a harness-only
  overwrite adapter. `Set` is not required by the correct application's Store.
- `harness/cmd/counter-fault`: fault server (`--engine`, `--dsn`, `--bug`),
  reusing `examples/counter/store.Open` and the real HTTP handler. The slow
  server is used by the interruption regression. User implementations do not
  need to implement these faults or any harness internals.

The reference is a separate Go module. `harness/go.mod` requires it through
`replace systems-trinkets/examples/counter => ../examples/counter` for the
in-process fallback. Keep that dependency one-way: the counter module has no
harness imports or DuckDB dependency. Subprocess tests build from the counter
module, and target `cwd` values resolve relative to `harness/targets/`, so
`../../examples/counter` is required. Moving only the command path would run
Go from the wrong module.

## Source and generated output

`harness/results/` is source only. `suitekit.RunsDir()` defaults to
`harness/artifacts/runs/`, ignored by Git. Existing local run files were moved
intact during the layout refactor; on another checkout move the old
`results/runs/` directory or pass it explicitly to report/SQL `--runs-dir`.
`HARNESS_RESULTS` still selects an individual suite run directory. Keep CLI
help, path helpers, documentation, and ignore rules aligned when this default
changes. Target files stayed in place, so their relative `cwd` values did not
change with the toolkit package renames.

The layout change was verified with harness build/vet/race tests, CLI and
counter module tests, a generated suite compiled in a temporary module,
and a fresh managed memory contract run using the new output default.
Checksums confirmed 138 migrated files unchanged, and the history report
read them from the new location. Current documentation links were checked.

## Targets and runs

`harness/targets/counter-go-{memory,sqlite,valkey,postgres}.toml` each carry
`cmd`/`cwd`, so `harness run counter --target <file>` starts and stops the SUT
itself (ports 8080–8083); do not also start one by hand, the second cannot
bind. `--url` is for a SUT you started yourself; crash tests skip there.
Valkey and Postgres come from `harness/infra/*.compose.yml`. The sqlite
default DSN is a stable file per listen address under the temp dir, so a
restarted SUT reopens the same database.

`harness run counter --all-targets` validates every matching target, then runs
in filename order with separate result directories and reports. It continues
through target failures and returns the first nonzero code. CLI selection
clears inherited target variables: otherwise `HARNESS_TARGET` in the shell
would override an explicit `--url` because `TargetFromEnv` prefers files.

## Crash tests

`suites/counter/crash_test.go` (INV-COUNTER-06) runs load in a goroutine,
kills the SUT with `h.Restart()` from the test goroutine once a third of the
increments are acknowledged, and checks a bound, `all 2xx ≤ final ≤ all 2xx +
errored`, never a timing claim. It skips without `h.Restartable()` and on the
memory engine (whose state is the process). `--bug write-behind` on the harness-owned `counter-fault` command is the
reference violation on every persistent engine.

## Verifying the ownership boundary

Run `go -C examples/counter test -race ./...` and
`go -C harness test -race ./...` separately. The former covers correct stores
and HTTP behavior; the latter includes fault decorators and the slow-server
interruption regression. The four standard targets must pass. Fault-command
runs must fail their intended invariants: memory lost-update (INV-01), memory
drop-reset (INV-04), and a harness-managed SQLite write-behind crash (INV-06)
were checked after the separation. Do not route standard target files through
the fault command or require users to implement fault flags to run a suite.

## Development traps

- Every suite request goes through `httpclient`, or it is not sampled; every
  `check.Invariant` cites an ID from that suite's `INVARIANTS.md`.
- A stale SUT on a target port makes a run test the wrong process; Main now
  refuses in that case, but for `--url` runs check `lsof -nP -iTCP:<port>`
  first. A test panic or `go test -timeout` leaves a harness-started SUT
  running (nothing can stop it from the dying process); the next run's
  refusal is how that surfaces.
- Initial child startup and sink publication use the same mutex as shutdown;
  registering a child after starting it without that lock permits SIGINT to
  miss it. `TestMainInterruptStopsSUTAndExports` exercises interruption during
  health waiting and load using a race-instrumented child test binary.
- Concurrent `Sink.Close` callers must wait for export completion, not just
  observe a closed flag: normal test exit can race signal shutdown and kill
  the process before Parquet files exist. `sync.Once` now shares the completed
  export and its error; the signal integration test reproduced this failure.
- Results are in-memory until `Close`; SIGINT exports partial results, a
  panic does not.
- A default the SUT computes at startup must be a pure function of its
  flags, or a restart-based test silently breaks (the sqlite temp-file DSN
  did exactly that before it was made stable).
- `modernc.org/sqlite` applies pragmas per connection: pass them as
  `_pragma=` DSN parameters, not `db.Exec`. Postgres's `ON CONFLICT DO
  UPDATE` needs the table-qualified column (`counters.value`).

## Not built

Open-loop load (`load.Open`), `[expect]` thresholds, an
on-disk DuckDB for crash-durable results, and Porcupine history checking are
backlog items in [the backlog](../harness/docs/backlog.md), each tied to the pattern that first needs
it.
