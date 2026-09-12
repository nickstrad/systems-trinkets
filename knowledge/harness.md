# HTTP invariant harness

The harness is an independent Go module under `harness/`. It tests a server
under test (SUT) over HTTP; it does not link to the SUT or talk directly to its
Valkey/PostgreSQL backing store. The contract and invariant documents under
`harness/suites/` define the protocol a language/engine implementation must
provide.

## Current package flow

`harness.Main` reads either `HARNESS_TARGET` (a TOML target file) or
`HARNESS_URL` plus metadata environment variables, waits for `GET /healthz`,
opens a results sink, runs the Go suite, and closes the sink. `harness.New`
resets the SUT with `POST /_reset`, checks health again, binds an `hx.Client`,
and registers cleanup that records the test row and flushes results.

The implemented package responsibilities are:

- `harness/hx`: JSON-aware HTTP requests, path-template escaping, per-request
  timeout/header options, and exactly one `SampleRow` per call. A non-2xx HTTP
  response is returned as a response (not a Go error); transport, timeout, and
  decode failures are errors.
- `harness/load`: barrier-started closed-loop workers (`Closed` and
  `ClosedFor`) and phase labels. `ClosedFor` starts its timeout at barrier
  release. `Phase` mutates the test handle and is explicitly not
  goroutine-safe.
- `harness/check`: records every invariant evaluation and fails the test on a
  false result; `Eventually` is for eventual-state checks, not timing claims.
- `harness/results`: flat run/test/check/sample/metric rows, a DuckDB-backed
  sink, and Parquet export under `results/runs/<run_id>/`. One collector
  goroutine owns the DuckDB appenders; callers submit rows through a channel.
- `harness/example-sut/counter`: a small in-memory HTTP reference server with
  deliberate `lost-update`, `drop-reset`, and `slow` modes. Its own Go tests
  are not the same thing as a completed harness suite.

## Development traps

Every suite request should go through `hx`, or it will not be sampled. Every
`check.Invariant` ID should come from that pattern's `INVARIANTS.md`, so the
failure can be tied to the primitive under test. `harness.New` mutates the SUT
by resetting it; use it only when the test owns that reset boundary.

Results are in-memory until `Close`; rows sent after close are dropped. Query
views scan Parquet files with a run-directory glob and skip tables for which no
Parquet file exists. Keep timestamps in UTC as the row types expect.

## Implemented vs plan-only surface

The source currently includes the core package code, the command entrypoint
and `run`/`report`/`sql`/`new-suite`/`targets` subcommands, SQL templates,
target parsing, and a counter contract/invariant/test suite. Inspect `targets/`
for the available configurations; `counter-example-memory.toml` describes the
in-memory reference counter. Recheck the current files before assuming a target
or running service is available.

`harness/test-plan.md` still describes the broader roadmap. The remaining
phase-2/3 items include a FIFO suite, open-loop load, SUT process lifecycle and
crash tests, operation-history/Porcupine checking, performance thresholds, and
a future hand-off to `trinkets attempt`. (The current `history.sql` query is a
run-history report; it is not the planned linearizability checker.) Target
fields `cmd`, `cwd`, and `env` are already represented but are marked phase-2
in the source and are not started by the current runner. Treat these as plans,
not guarantees.
