# Harness architecture

This is the current design authority. Verify details against source when
changing behavior, update this document with design decisions, and append
work and verification evidence to [work-log.md](work-log.md). The original
plan and reviews are archived there; future features are in
[backlog.md](backlog.md). To write a suite, use
[suite-authoring.md](suite-authoring.md).

## Purpose and ownership

The harness checks named invariants of small HTTP servers. One suite per
pattern tests implementations in different languages and storage engines
through the same contract. Each suite is an ordinary Go test package, so
Go's test filtering, race detector, timeouts, and reporting remain available.

This directory is the independent `systems-trinkets/harness` Go module.
The metadata CLI in `../../cli/` and correct counter reference in
`../../examples/counter/` are separate modules. Correct application logic
belongs to the user. Harness mechanics, deliberate faults, and bug-only
interfaces belong here.

Configured targets are tested over HTTP. The counter suite also imports the
reference's public HTTP/memory package for an explicit in-process fallback.
The harness's `go.mod` replaces that dependency with `../examples/counter`;
the reference does not import the harness or its DuckDB dependency.

## Package map

Paths in this document's code examples are relative to the harness module.

| Path | Responsibility | Start reading |
|------|----------------|---------------|
| `suites/counter/` | HTTP contract and six invariants | `CONTRACT.md`, `INVARIANTS.md`, `contract_test.go` |
| `suitekit/` | Suite startup, per-test context, target configuration, restart | `test_context.go`, `suite_lifecycle.go` |
| `httpclient/` | JSON HTTP requests and per-request samples | `client.go`, `context.go` |
| `load/` | Barrier-started closed-loop workers and phase labels | `load.go` |
| `check/` | Named checks and recorded metrics | `check.go` |
| `process/` | Start, kill, stop, and reap one process group | `process.go` |
| `results/` | Row definitions, recorder, DuckDB collector, Parquet export/query | `rows.go`, `sink.go` |
| `cmd/harness/` | Run, report, SQL, target listing, suite scaffolding | `main.go`, then the command file |
| `queries/` | SQL reports read at runtime | `summary.sql`, `latency.sql` |
| `targets/` | TOML configurations for four counter engines | `counter-go-memory.toml` |
| `infra/` | Manually started Valkey/PostgreSQL backing stores | Engine compose file |
| `internal/counterfault/` | Deliberate fault decorators and their tests | `bugs.go` |
| `cmd/counter-fault/` | Harness-owned fault server composition | `main.go` |
| `artifacts/runs/` | Generated, ignored run data | `<run_id>/*.parquet`, `sut.log` |

Within `suitekit/`, `target.go` owns `Target`, TOML/environment loading,
and environment helpers. `paths.go` resolves module/output paths;
`health.go` probes listener ownership and readiness; `restart.go` implements
`H.Restartable` and `H.Restart`. Tests live beside the code they exercise.

## Suite and request flow

1. A suite's `TestMain` calls `suitekit.Main(m, options...)` and passes its
   return value to `os.Exit`.
2. `Main` reads `HARNESS_TARGET`, or `HARNESS_URL` plus metadata variables.
   A target file takes precedence over URL environment variables. Explicit
   CLI target selection clears inherited target metadata first.
3. If a target has `cmd`, `Main` refuses an already answering URL, installs
   signal handling before startup, and starts a process group via `process`.
   Output goes to the run's `sut.log`. It waits for `/healthz` and fails
   promptly if its child exits. A URL-only target is managed by the user.
4. `Main` opens the results sink, runs the Go tests, stops its child, and
   exports the sink. With `suitekit.InProcess(newHandler)` and no target,
   it serves the handler on a loopback `httptest` server. Those results are
   discarded unless `HARNESS_RESULTS` is set. Without either, tests using
   `suitekit.New` skip.
5. Each test calls `suitekit.New(t)`: reset through `POST /_reset`, check
   `/healthz`, bind an `httpclient.Client`, and register cleanup to record
   the test outcome and flush rows into the in-memory database.
6. Suite requests go through that client. `load.Closed` and `ClosedFor`
   start workers behind a barrier and attach worker/phase/count context.
   `check.Invariant` records a named check and fails the test when false;
   `check.Metric` records a number without asserting a performance limit.

`httpclient.Client.Do` records one sample per request, including failures.
Non-2xx status is a response to inspect; transport, timeout, cancellation,
and JSON decoding failures are errors. Path templates are recorded before
substitution, so reports group requests by route.

`load.Result.Conclusive(h)` fails an ordinary load test when requests or
worker functions errored. Final-state assertions would otherwise guess
whether ambiguous requests were applied. Crash tests use explicit bounds.

## Targets and process lifecycle

```toml
# targets/counter-go-sqlite.toml
pattern = "counter"
language = "go"
engine = "sqlite"
url = "http://127.0.0.1:8081"
cmd = ["go", "run", "./cmd/counter", "--addr", "127.0.0.1:8081", "--engine", "sqlite"]
cwd = "../../examples/counter"
```

`cwd` resolves against the target file, not the invoking shell's working
directory; an omitted `cwd` means the target file's directory. `env` adds
or overrides environment variables. `[expect]` is parsed but thresholds
are not enforced. The checked-in target files are the runnable authority
for engine ports and commands.

`process.Start(Spec)` starts without waiting for HTTP health. `Proc.Kill`
sends SIGKILL to the whole process group and reaps the child; `Proc.Stop`
tries SIGTERM, waits a grace period, then kills. `Exited`, `PID`, `ExitState`,
and `Err` expose lifecycle observations. This package takes a `Spec`, not a
suitekit `Target`, and does not depend on `suitekit`.

`H.Restart` kills and starts the same target, waits for health, and does
not reset its state. Call it from the test goroutine; it may call `t.Fatal`
and is not safe to call concurrently with itself. Child publication and
shutdown share a mutex so a signal cannot miss a newly started child.

The counter crash test checks `all 2xx <= final <= all 2xx + errored`, counting
acknowledgements before and after restart. It skips memory targets and runs
without a harness-managed process. Killing the application leaves its backing
store running (or SQLite's file/page cache intact); this probes application
acknowledgement discipline, not a whole-machine power failure.

## Results and reporting

The pipeline is: test goroutines send rows to a channel; one collector owns
the DuckDB appenders; close drains and exports five Parquet tables. The
DuckDB driver is `github.com/duckdb/duckdb-go/v2`, requiring cgo and a C compiler.

| Table | One row per | Main fields |
|-------|-------------|-------------|
| `runs` | Suite execution | Run ID, pattern, language, engine, target, versions, start/finish |
| `tests` | Go test | Name, status, duration, error |
| `checks` | Invariant evaluation | Invariant ID, boolean result, message, JSON details |
| `samples` | HTTP request | Test, worker, phase, route, status, latency, error |
| `metrics` | Recorded number | Name, value, unit, JSON labels |

`results.Recorder` decouples writers from the sink; `Buffer` and `Discard`
support tests and unrecorded runs. `Sink.Flush` commits appended rows into
the in-memory database. `Sink.Close` exports; concurrent callers wait for
the same completed export and share its error. JSON fields are VARCHAR;
check/metric timestamps use the column name `recorded_at`.

Default output is `<module>/artifacts/runs/<run_id>/`. `HARNESS_RESULTS`
overrides a suite's run directory; report/SQL `--runs-dir` overrides the
parent directory scanned for runs. `results/` contains source only. Existing
local `results/runs/` data was moved to `artifacts/runs/` during this refactor;
other checkouts can move their old directory or read it using `--runs-dir`.

`results.Query` creates views over `<runs-dir>/*/<table>.parquet`.
`samples_measured` excludes setup reset/health requests. Runtime SQL files
provide `summary`, `latency`, `checks`, `compare`, and `history`; parameters
are bound with `sql.Named`. Percentiles are defined in SQL, not suites.

SIGINT/SIGTERM stop the child and export partial results. A test panic or
`go test -timeout` kills the test process before cleanup, loses in-memory
results, and can leave its child alive. The next managed run refuses an
already answering URL. Persistent result storage remains backlog.

## Commands and batch semantics

See [the README](../README.md) for runnable commands and flags.
`run` wraps `go test` and returns its exit code. `--all-targets` validates
all matching files before starting, runs in filename order with separate
run IDs/reports, continues through failures, and returns the first nonzero
code. `report` and `sql` query saved results; `targets` lists configurations.
`new-suite` writes contract/invariant documents and separate setup, contract,
and concurrency test files without overwriting existing files.

## Verification

From the repository root:

```sh
go -C cli test ./...
go -C examples/counter test ./...
go -C harness test -race ./...
go -C harness vet ./...
go -C harness build ./...
```

The harness tests cover its own packages, the in-process suite, managed
SQLite startup/restart, signal shutdown/export, and CLI batch/report behavior.
Valkey/PostgreSQL require their backing services for live target checks.
Scaffold into a temporary module to verify generated tests compile without
adding an unagreed pattern suite to the repository.
