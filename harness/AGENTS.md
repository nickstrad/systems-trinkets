# Working in `harness/`

`test-plan.md` is the source of truth for design and decisions **and the
living log of the work** (its §0). Read it before changing anything here:
the last §0 entries plus §10a say where things stand. When a decision
changes, change it there first; when work starts, is delegated, is
reviewed, or lands, append a §0 line. Roles, review, and commit rules are
in its §10c. This file is the operating procedure derived from its §8.

## Starting a new pattern

1. Read the pattern: `trinkets pattern show <slug>` (notes CLI at the repo
   root) and its row in `docs/systems-patterns.md`. The `invariants` column
   is the seed.
2. Run the **invariant interview** with the user — the guide's own questions
   applied to this pattern. Record the answers in `INVARIANTS.md`:
   1. What invariant must the system preserve? (list; each gets an ID
      `INV-<PATTERN>-NN`)
   2. Which primitive provides each guarantee on each engine? (a failure
      points back here)
   3. What happens under concurrent access? (→ concurrency tests)
   4. What happens if the process crashes between steps? (→ crash tests;
      they need a target with `cmd`, see §4 kind 3 of the plan)
   5. How are retries, duplicates, ordering, expiration, recovery handled?
      (→ contract details: idempotency of endpoints, ack semantics, TTLs)
   6. Which guarantees come from the store vs from application convention?
      (→ "must hold" vs "should hold")
   7. Performance expectations, if any (→ metrics; optional `[expect]`)
3. `go run ./cmd/harness new-suite <pattern>` scaffolds
   `suites/<pattern>/{CONTRACT.md,INVARIANTS.md,<pattern>_test.go}`.
   Propose `CONTRACT.md` (endpoints, JSON, error codes, plus `/healthz` and
   `/_reset`) and `INVARIANTS.md` (table: ID, statement, guaranteeing
   primitive, kind, must/should, status). **Iterate with the user until
   both say "agreed" before writing test code.**
4. Write tests in this order: contract/sequential → concurrency invariants →
   performance metrics → crash (needs `h.Restartable()`; skip otherwise). Use `suites/counter/` as the model.
5. If the pattern has an `example-sut/<pattern>`, give it `--bug` modes that
   break each invariant on purpose and prove the suite catches them.
6. Run: `go run ./cmd/harness run <pattern> --url http://…` (or
   `--target targets/<file>.toml`), then `go run ./cmd/harness report --run last`.
   Record lessons with `trinkets attempt add`.

## Rules

- Never assert timing in an invariant test; assert final state or recorded
  history. Performance is recorded (`check.Metric`), not asserted.
- Every `check.Invariant` cites an ID from that suite's `INVARIANTS.md`.
- Every request goes through `hx` (via `h.Get/Post/Delete` or
  `h.Client.<Method>(w.Ctx, …)` inside load workers) so it is sampled.
  Never use `net/http` directly in a suite.
- Concurrency tests use `load.Closed` / `load.ClosedFor` so workers start
  behind a barrier and the `hx.Counter` tallies are correct.
- After a load run, gate final-state invariants on `r.Conclusive(h)`: a
  request that errored may or may not have been applied, so the test fails
  as inconclusive rather than passing or failing by luck.
- Don't compute latency percentiles in a suite; every request is a sample and
  `latency.sql` derives p50/p95/p99 from `samples_measured`. Record
  throughput and counts with `check.Metric` if you want them in `metrics`.
- Keep `test-plan.md` and this file current; they are the context after a
  context reset.

## Adding a tool to the toolkit

Add it to the package it belongs to (`hx`, `load`, `check`, `results`,
`sut`, `history`); add a unit test (`httptest` + `results.Buffer` for
anything that records); note the API in test-plan.md §9.

## Quick reference

```
go test ./...                                   # unit tests; suites skip without a target
go run ./example-sut/cmd/counter --addr 127.0.0.1:8080 [--engine memory] [--bug lost-update|drop-reset|write-behind|slow]
go run ./cmd/harness run counter --url http://127.0.0.1:8080 [--language go --engine memory --label "…"]
go run ./cmd/harness report --run last [--query summary|latency|checks|compare|history]
go run ./cmd/harness sql "select … from samples"
go run ./cmd/harness targets
```

Env read by a suite's `TestMain`: `HARNESS_TARGET` or `HARNESS_URL`
(+ `HARNESS_PATTERN|LANGUAGE|ENGINE|LABEL`), `HARNESS_RUN_ID`,
`HARNESS_RESULTS`, `HARNESS_SUT_REF`, `HARNESS_HEALTH_TIMEOUT`.
