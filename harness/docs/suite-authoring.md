# Writing an invariant suite

Use this procedure for a new pattern. All command paths below are relative
to the harness module unless stated otherwise. See
[architecture.md](architecture.md) for runtime behavior and
[the counter suite](../suites/counter/) for the worked example.

## Starting a new pattern

1. Read the pattern: `trinkets pattern show <slug>` (notes CLI in `cli/`) and its row in the repository guide `docs/systems-patterns.md`. The `invariants` column
   is the seed.
2. Run the **invariant interview** with the user — the guide's own questions
   applied to this pattern. Record the answers in `INVARIANTS.md`:
   1. What invariant must the system preserve? (list; each gets an ID
      `INV-<PATTERN>-NN`)
   2. Which primitive provides each guarantee on each engine? (a failure
      points back here)
   3. What happens under concurrent access? (→ concurrency tests)
   4. What happens if the process crashes between steps? (→ crash tests;
      they need a target with `cmd`, see [architecture.md](architecture.md#targets-and-process-lifecycle))
   5. How are retries, duplicates, ordering, expiration, recovery handled?
      (→ contract details: idempotency of endpoints, ack semantics, TTLs)
   6. Which guarantees come from the store vs from application convention?
      (→ "must hold" vs "should hold")
   7. Performance expectations, if any (→ metrics; optional `[expect]`)
3. `go run ./cmd/harness new-suite <pattern>` scaffolds
   `suites/<pattern>/{CONTRACT.md,INVARIANTS.md,main_test.go,contract_test.go,concurrency_test.go}`.
   Propose `CONTRACT.md` (endpoints, JSON, error codes, plus `/healthz` and
   `/_reset`) and `INVARIANTS.md` (table: ID, statement, guaranteeing
   primitive, kind, must/should, status). **Iterate with the user until
   both say "agreed" before writing test code.**
4. Write tests in this order: contract/sequential → concurrency invariants →
   performance metrics → crash (needs `h.Restartable()`; skip otherwise). Use `suites/counter/` as the model.
5. Keep deliberate faults and their tests inside this harness. For counter,
   use `cmd/counter-fault` and `internal/counterfault`. The user builds the
   correct implementation and must not need bug flags or harness machinery.
6. Run: `go run ./cmd/harness run <pattern> --url http://…` (or
   `--target targets/<file>.toml`), then `go run ./cmd/harness report --run last`.
   Record lessons with `trinkets attempt add`.

## Rules

- Never assert timing in an invariant test; assert final state or recorded
  history. Performance is recorded (`check.Metric`), not asserted.
- Every `check.Invariant` cites an ID from that suite's `INVARIANTS.md`.
- Every request goes through `httpclient` (via `h.Get/Post/Delete` or
  `h.Client.<Method>(w.Ctx, …)` inside load workers) so it is sampled.
  Never use `net/http` directly in a suite.
- Concurrency tests use `load.Closed` / `load.ClosedFor` so workers start
  behind a barrier and the `httpclient.Counter` tallies are correct.
- After an ordinary load run, gate final-state invariants on `r.Conclusive(h)`: a
  request that errored may or may not have been applied, so the test fails
  as inconclusive rather than passing or failing by luck. Crash tests instead
  bound final state by all acknowledgements and ambiguous errors; their
  deliberate transport errors must not go through this gate.
- Don't compute latency percentiles in a suite; every request is a sample and
  `latency.sql` derives p50/p95/p99 from `samples_measured`. Record
  throughput and counts with `check.Metric` if you want them in `metrics`.
- Keep [architecture.md](architecture.md) and this file current; they are the context after a
  context reset.


## File responsibilities and reading order

Read `CONTRACT.md`, then `INVARIANTS.md`, then `contract_test.go`,
`concurrency_test.go`, and `crash_test.go` when present. `main_test.go`
contains suite setup and any in-process fallback. Put shared request/load
helpers and their unit tests in `helpers_test.go` as the suite grows.
The scaffold starts with setup, contract, and concurrency files; add helpers
and crash tests when needed.

Correct reference and user applications live outside the harness module.
Fault decorators and fault commands stay under `internal/` and `cmd/` here.
Use `cmd/counter-fault` when validating that the counter suite detects its
intended failures; standard target files run the correct reference command.
