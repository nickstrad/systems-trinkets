# harness

`test-plan.md` in this directory is the source of truth for design decisions.
Read it before changing anything here — this README is just the practical
on-ramp.

## What this is

A small, isolated Go toolkit that tests **invariants** of tiny HTTP servers
implementing the systems patterns in `docs/systems-patterns.md` (counter,
idempotency key, FIFO queue, lease, lock, rate limiter, ...). The user
implements each pattern in several languages and against several engines
(Valkey, SQLite, PostgreSQL); the harness verifies every implementation the
same way over HTTP and records results so they can be compared with DuckDB
instead of read off the terminal.

## Layout

```
harness/
  test-plan.md            design source of truth
  AGENTS.md               working agreement for agents (interview, rules)
  go.mod                  module systems-trinkets/harness
  harness/                glue: Main (TestMain), New(t) -> H, Target loading
  cmd/harness/            CLI: run | report | sql | new-suite | targets
  hx/                     HTTP client: JSON helpers, timeouts, path templates,
                          every request recorded as a sample
  load/                   concurrency: Closed (N workers), Barrier, Phases
  check/                  invariant assertions -> check rows + t.Errorf
  results/                Run/Test/Check/Sample/Metric row types, in-memory
                          sink, flush to Parquet
  suites/
    counter/
      CONTRACT.md
      INVARIANTS.md
      counter_test.go
    fifo-queue/ ...       (planned)
  targets/
    counter-go-{memory,sqlite,valkey,postgres}.toml
                          (one per engine of example-sut/cmd/counter)
  infra/
    valkey.compose.yml    postgres.compose.yml
  queries/                *.sql report templates run with DuckDB
  example-sut/            tiny Go reference server(s), with --bug flags that
                          break invariants on purpose
  results/                gitignored; results/runs/<run_id>/*.parquet
```

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
`results/runs/<run_id>/sut.log`. Do not also start one by hand; the second
process cannot bind the port. `run` executes `suites/counter` via `go test`
and writes `results/runs/<run_id>/*.parquet`. Swap the target file for another
engine and `report --query history --pattern counter` puts the runs side by
side.

For an implementation of your own — anything the harness should not start —
run it yourself and point `--url` at it:

```
go run ./example-sut/cmd/counter --addr 127.0.0.1:8082 --engine valkey &
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
memory reference; suites without an `InProcess` fallback skip. The reference SUT takes `--bug lost-update|drop-reset|write-behind|slow` to
prove the suite catches broken implementations.

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
label    = "example-sut valkey (INCRBY)"
cmd = ["go", "run", "./example-sut/cmd/counter", "--addr", "127.0.0.1:8082", "--engine", "valkey", "--dsn", "redis://127.0.0.1:6379/1"]
cwd = ".."                     # relative to the target file → the module root
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

1. Read the pattern: `trinkets pattern show <slug>` and its row in
   `docs/systems-patterns.md`.
2. Interview with the user: what must hold, which primitive guarantees it,
   what happens under concurrency, what happens on crash (kill + restart mid-load), how
   retries/duplicates/ordering/expiration are handled, which guarantees are
   store-provided vs application convention, and any performance
   expectations.
3. Write `CONTRACT.md` (endpoints, request/response shapes, error codes, plus
   the required `GET /healthz` and `POST /_reset`) and `INVARIANTS.md` (ID,
   statement, guaranteeing primitive, test kind, status) **before any test
   code**, and agree them with the user.
4. `harness new-suite <pattern>` scaffolds `suites/<pattern>/` from
   templates. Write tests in order: contract/sequential, then concurrency
   invariants, then performance metrics, then crash tests (`h.Restartable()` / `h.Restart()`; see `suites/counter/crash_test.go`).
5. Run against `example-sut` first if one exists, then the real
   implementation, and check with `harness report --run last`; record
   lessons with `trinkets attempt add`.

## Results

Every run writes five Parquet tables under `results/runs/<run_id>/`: `runs`
(one row per run — pattern, language, engine, target, timestamps), `tests`
(one row per Go test/subtest — status, duration, error), `checks` (one row
per invariant evaluation — invariant ID, ok, message, details), `samples`
(one row per HTTP request — method, path, status, latency), and `metrics`
(free-form recorded numbers — throughput and counts). Percentiles are
computed from samples by the SQL reports.

`harness sql` registers these as views over `results/runs/*/` so you can
query across every run with plain SQL, e.g. `select * from checks where not
ok`. `recorded_at` is the timestamp column on `checks` and `metrics` rows —
use it to compare runs over time or filter to a specific run's history.
