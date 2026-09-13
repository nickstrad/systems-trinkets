# HTTP invariant harness

The harness is an independent Go module under `harness/`. It tests a server
under test (SUT) over HTTP; configured external runs never link to the SUT or talk to its
backing store. The counter suite also links its memory reference for the
explicit in-process fallback. The contract and invariant documents under `harness/suites/`
define the protocol an implementation in any language, on any engine, must
provide. `harness/test-plan.md` is the design authority and the work log; its
§0 says where the work stands and its §9 is the as-built API. Read it before
changing anything there.

## Current package flow

`harness.Main` (a suite's `TestMain`) reads `HARNESS_TARGET` (a TOML target
file) or `HARNESS_URL` plus metadata variables. If the target file has a
`cmd`, Main first refuses to run when something already answers at the
target URL, then starts the SUT itself (package `sut`, process group of its
own, output to `<results>/sut.log`), waits for `GET /healthz`, opens the
results sink, runs the suite, stops the SUT, and exports. With
`harness.InProcess(handler)` and no target in the environment, the suite runs
against the handler on a loopback `httptest` server instead of skipping, so
`go test ./...` exercises the counter suite; those in-process results are
discarded unless `HARNESS_RESULTS` is set. `harness.New` resets the SUT with
`POST /_reset`, checks health, binds an `hx.Client`, and registers cleanup
that records the test row and flushes results.

Package responsibilities:

- `harness/hx`: JSON-aware HTTP requests, path-template escaping, per-request
  timeout options, and exactly one `SampleRow` per call. A non-2xx response
  is a response, not an error; transport, timeout, and decode failures are.
- `harness/load`: barrier-started closed-loop workers (`Closed`, `ClosedFor`)
  and phase labels. `Result.Conclusive` fails the test if any request
  errored, because a final-state invariant cannot be judged then.
- `harness/check`: records every invariant evaluation and fails the test on
  a false result; `Metric` records numbers that are reported, not asserted.
- `harness/results`: flat run/test/check/sample/metric rows, a DuckDB-backed
  in-memory sink, Parquet export under `results/runs/<run_id>/`, and `Query`
  views over every run. One collector goroutine owns the appenders.
- `harness/sut`: start, kill (SIGKILL the process group, so a `go run`
  grandchild's listener really closes), stop with grace, and exit
  notification for one SUT process.
- `harness/example-sut/counter`: the reference counter as a library — the
  `Store` interface, `NewHandler` (the contract), the memory store, and bug
  decorators `LostUpdate`, `DropReset`, `WriteBehind`, `Slow`, with deliberate violations (`Slow` only affects recorded performance). `StoreTest` is the conformance test every engine runs.
  Engine stores live one package each under `store/{sqlite,valkey,postgres}`;
  `example-sut/cmd/counter` is the binary (`--engine`, `--dsn`, `--bug`).

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
memory engine (whose state is the process). `--bug write-behind` is the
reference violation on every persistent engine.

## Development traps

- Every suite request goes through `hx`, or it is not sampled; every
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
backlog items in `test-plan.md` §7, each tied to the pattern that first needs
it.
