# harness

Start with the reading order below to study the code. The current design is
in [docs/architecture.md](docs/architecture.md); the original plan and review
history are preserved in [docs/work-log.md](docs/work-log.md). Future work
is separate in [docs/backlog.md](docs/backlog.md).

## What this is

A small, isolated Go toolkit that tests **invariants** of tiny HTTP servers
implementing the systems patterns in `docs/systems-patterns.md` (counter,
idempotency key, FIFO queue, lease, lock, rate limiter, ...). The user
implements each pattern in several languages and against several engines
(Valkey, SQLite, PostgreSQL); the harness verifies every implementation the
same way over HTTP and records results so they can be compared with DuckDB
instead of read off the terminal.

## Reading order

1. [Counter contract](suites/counter/CONTRACT.md) and
   [invariants](suites/counter/INVARIANTS.md): what a correct server guarantees.
2. [Contract tests](suites/counter/contract_test.go),
   [concurrency tests](suites/counter/concurrency_test.go), then
   [crash tests](suites/counter/crash_test.go): how those guarantees are checked.
   [Setup](suites/counter/main_test.go) and [helpers](suites/counter/helpers_test.go)
   support these tests.
3. [Per-test context](suitekit/test_context.go) and
   [suite lifecycle](suitekit/suite_lifecycle.go): how tests get a target,
   client, and recorder. Follow `target.go`, `health.go`, `restart.go`, and
   `paths.go` for each supporting responsibility.
4. [HTTP client](httpclient/client.go), [load](load/load.go), and
   [checks](check/check.go): requests, concurrent work, and invariant evaluation.
5. [Process control](process/process.go), [result rows](results/rows.go),
   [sink](results/sink.go), then [CLI](cmd/harness/main.go) and
   [SQL reports](queries/): lifecycle and the recorded evidence.

## Layout

```text
harness/
  README.md               overview, study order, runnable commands
  AGENTS.md               working agreement for agents
  docs/
    architecture.md       current design authority and package flow
    suite-authoring.md    interview and new-suite procedure
    backlog.md            conditional future features
    work-log.md           work entries and archived plan/reviews
  go.mod                  module systems-trinkets/harness
  suitekit/               per-suite and per-test orchestration
  httpclient/             JSON HTTP requests and sample recording
  load/                   barrier-started workers and phase labels
  check/                  invariant assertions and recorded metrics
  process/                child process groups: start, kill, stop, reap
  results/                source: row types, collector, Parquet export/query
  cmd/harness/            run | report | sql | new-suite | targets
  cmd/counter-fault/      deliberate-fault server for harness validation
  internal/counterfault/  fault decorators and tests
  suites/counter/
    CONTRACT.md           HTTP protocol
    INVARIANTS.md         named guarantees
    main_test.go          suite setup and in-process fallback
    helpers_test.go       shared helpers and helper unit tests
    contract_test.go     sequential behavior
    concurrency_test.go  concurrent correctness
    crash_test.go        acknowledged writes across restart
  targets/                counter-go-{memory,sqlite,valkey,postgres}.toml
  infra/                  Valkey and PostgreSQL compose files
  queries/                SQL report templates, read at runtime
  artifacts/runs/         generated, ignored: <run_id>/*.parquet and sut.log
```

The standalone reference service lives in [../examples/counter/](../examples/counter/README.md),
with its own module, command, and private engine packages. The harness imports
only its public HTTP/memory package for the in-process fallback, via a local
`go.mod` replacement; configured targets launch its command over HTTP.

## Prerequisites

- Go 1.26
- clang / Xcode command line tools (`xcode-select --install`) — needed for the
  cgo DuckDB driver the results pipeline uses
- Docker, if you want the pattern's backing store (Valkey, Postgres) to run
  locally

## Quick start

One engine end to end — start its backing store, run the suite, read the
results:

```
docker compose -f infra/valkey.compose.yml up -d               # the backing store
go run ./cmd/harness run counter --target targets/counter-go-valkey.toml
go run ./cmd/harness report --run last
go run ./cmd/harness sql "select test, status from tests order by 1"
```

Every `targets/*.toml` here carries a `cmd`, so `run` starts the reference SUT
itself, waits for its `/healthz`, and stops it at the end — its output is in
`artifacts/runs/<run_id>/sut.log`. Do not also start one by hand; the second
process cannot bind the port. `run` executes `suites/counter` via `go test`
and writes `artifacts/runs/<run_id>/*.parquet`. Swap the target file for another
engine and `report --query history --pattern counter` puts the runs side by
side.

For an implementation of your own — anything the harness should not start —
run it yourself and point `--url` at it:

```
go -C ../examples/counter run ./cmd/counter --addr 127.0.0.1:8082 --engine valkey &
go run ./cmd/harness run counter --url http://127.0.0.1:8082 --language go --engine valkey
```

Each suite is a normal Go test package, so you can also run it directly
without the CLI wrapper:

```
HARNESS_URL=http://127.0.0.1:8080 HARNESS_LANGUAGE=go HARNESS_ENGINE=memory go test ./suites/counter/ -v
# or, pointing at a target file instead of a bare URL:
HARNESS_TARGET=targets/counter-go-memory.toml go test ./suites/counter/ -v
```

Without `HARNESS_URL`/`HARNESS_TARGET`, counter runs against its in-process
memory reference; suites without an `InProcess` fallback skip. Deliberate faults live in this module, under `internal/counterfault` and
`cmd/counter-fault`. The user's correct implementation has no bug flags and
imports no harness code. To exercise a deliberate fault, from `harness/`:

```sh
go run ./cmd/counter-fault --engine memory --bug lost-update
```

Then point the suite at that server with `--url`. For crash-fault tests, create
a target using `cmd = ["go", "run", "./cmd/counter-fault", "--engine", "sqlite",
"--bug", "write-behind"]` and `cwd = ".."` when placed in `harness/targets/`;
the harness must own the process to kill and restart it. The existing four
targets continue to run the correct standalone command.

## Targets

A target file (`targets/<pattern>-<language>-<engine>.toml`) describes one
concrete server under test: pattern, language, engine, URL, a free-text
label, and a `cmd`/`cwd` to let the harness start/stop it itself. Example:

```toml
# targets/counter-go-valkey.toml
pattern  = "counter"
language = "go"
engine   = "valkey"
url      = "http://127.0.0.1:8082"
label    = "counter reference valkey (INCRBY)"
cmd = ["go", "run", "./cmd/counter", "--addr", "127.0.0.1:8082", "--engine", "valkey", "--dsn", "redis://127.0.0.1:6379/1"]
cwd = "../../examples/counter" # relative to the target file → counter module
[expect]                       # reserved; thresholds are not enforced yet
# p99_ms = 20
```

The reference SUT ships one target per engine, on a port of its own so all
four can run at once:

| target | engine | port | `--dsn` default |
| --- | --- | --- | --- |
| `counter-go-memory.toml` | memory | 8080 | — (map + mutex) |
| `counter-go-sqlite.toml` | sqlite | 8081 | `$TMPDIR/counter-sqlite-<addr>.db`, stable per port and logged at startup |
| `counter-go-valkey.toml` | valkey | 8082 | `redis://127.0.0.1:6379/1` |
| `counter-go-postgres.toml` | postgres | 8083 | `postgres://trinkets:trinkets@127.0.0.1:5432/trinkets?sslmode=disable` |

`harness run <pattern> --target targets/x.toml` starts the SUT from the file's
`cmd` (and stops it afterwards); `harness run <pattern> --url http://...`
talks to a server you started yourself and never manages a process.
`harness targets` lists the available files. Once both backing stores are up,
run and compare all four engines with:

```
go run ./cmd/harness run counter --all-targets
go run ./cmd/harness report --query history --pattern counter
```

Batch runs validate `targets/counter-*.toml` before starting, then execute in
filename order. Each target gets its own run ID, results, and summary. A
failed target does not stop later targets; the command returns the first
nonzero exit code. `run --all-targets counter` is also accepted. Go test flags
after `--` apply to every target.

## Backing stores

Compose files under `infra/` start each pattern's backing store on its
default port, bound to `127.0.0.1`:

```
docker compose -f infra/valkey.compose.yml up -d
docker compose -f infra/postgres.compose.yml up -d
```

The **server under test** (your implementation) connects to these — the
harness never talks to Valkey or Postgres directly, only to the SUT over
HTTP. Postgres credentials are `trinkets` / `trinkets` / db `trinkets`; both
files have healthchecks and `restart: unless-stopped`, and are meant to be
brought up once and left running while you iterate.

## Writing a new suite

Follow [docs/suite-authoring.md](docs/suite-authoring.md) for the interview,
contract agreement, and test-writing rules. `go run ./cmd/harness new-suite <pattern>` generates `CONTRACT.md`, `INVARIANTS.md`, `main_test.go`,
`contract_test.go`, and `concurrency_test.go`. Add shared helpers and crash
tests as the pattern requires them.

## Results

Every run writes five Parquet tables under `artifacts/runs/<run_id>/`: `runs`
(one row per run — pattern, language, engine, target, timestamps), `tests`
(one row per Go test/subtest — status, duration, error), `checks` (one row
per invariant evaluation — invariant ID, ok, message, details), `samples`
(one row per HTTP request — method, path, status, latency), and `metrics`
(free-form recorded numbers — throughput and counts). Percentiles are
computed from samples by the SQL reports.

`harness sql` registers these as views over `artifacts/runs/*/` so you can
query across every run with plain SQL, e.g. `select * from checks where not
ok`. `recorded_at` is the timestamp column on `checks` and `metrics` rows —
use it to compare runs over time or filter to a specific run's history.
