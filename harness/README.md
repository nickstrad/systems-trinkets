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
    counter-example-memory.toml   (points at example-sut/counter)
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

Start a reference SUT, then run its suite:

```
go run ./example-sut/counter --addr 127.0.0.1:8080 &          # reference SUT
go run ./cmd/harness run counter --url http://127.0.0.1:8080  # runs suites/counter via go test, writes results/runs/<run_id>/*.parquet
go run ./cmd/harness report --run last
go run ./cmd/harness sql "select test, status from tests order by 1"
```

Each suite is a normal Go test package, so you can also run it directly
without the CLI wrapper:

```
HARNESS_URL=http://127.0.0.1:8080 HARNESS_LANGUAGE=go HARNESS_ENGINE=memory go test ./suites/counter/ -v
# or, pointing at a target file instead of a bare URL:
HARNESS_TARGET=targets/counter-example-memory.toml go test ./suites/counter/ -v
```

Without `HARNESS_URL`/`HARNESS_TARGET` a suite skips, so `go test ./...`
stays green. The reference SUT takes `--bug lost-update|drop-reset|slow` to
prove the suite catches broken implementations.

## Targets

A target file (`targets/<pattern>-<language>-<engine>.toml`) describes one
concrete server under test: pattern, language, engine, URL, a free-text
label, and (phase 2) a `cmd` to let the harness start/stop it itself. Example:

```toml
# targets/counter-go-valkey.toml
pattern  = "counter"
language = "go"
engine   = "valkey"
url      = "http://127.0.0.1:8080"
label    = "v1 INCR"
[expect]                       # optional hard performance limits
# p99_ms = 20
```

`harness run <pattern> --target targets/x.toml` and `harness run <pattern>
--url http://...` are equivalent ways to point at a target; `harness targets`
lists the available files.

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
   what happens under concurrency, what happens on crash (phase 2), how
   retries/duplicates/ordering/expiration are handled, which guarantees are
   store-provided vs application convention, and any performance
   expectations.
3. Write `CONTRACT.md` (endpoints, request/response shapes, error codes, plus
   the required `GET /healthz` and `POST /_reset`) and `INVARIANTS.md` (ID,
   statement, guaranteeing primitive, test kind, status) **before any test
   code**, and agree them with the user.
4. `harness new-suite <pattern>` scaffolds `suites/<pattern>/` from
   templates. Write tests in order: contract/sequential, then concurrency
   invariants, then performance metrics, then crash tests (phase 2).
5. Run against `example-sut` first if one exists, then the real
   implementation, and check with `harness report --run last`; record
   lessons with `trinkets attempt add`.

## Results

Every run writes five Parquet tables under `results/runs/<run_id>/`: `runs`
(one row per run — pattern, language, engine, target, timestamps), `tests`
(one row per Go test/subtest — status, duration, error), `checks` (one row
per invariant evaluation — invariant ID, ok, message, details), `samples`
(one row per HTTP request — method, path, status, latency), and `metrics`
(free-form recorded numbers — throughput, percentiles, ...).

`harness sql` registers these as views over `results/runs/*/` so you can
query across every run with plain SQL, e.g. `select * from checks where not
ok`. `recorded_at` is the timestamp column on `checks` and `metrics` rows —
use it to compare runs over time or filter to a specific run's history.
