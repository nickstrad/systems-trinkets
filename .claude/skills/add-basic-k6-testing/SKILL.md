---
name: add-basic-k6-testing
description: Turn a completed systems-trinkets lesson into an HTTP adapter, a k6 workload, and a DuckDB analysis that re-run the lesson's own comparison under concurrent traffic. Use when asked to add k6 testing, a performance follow-up, or a load test to a lesson in lessons/go. Starts from the lesson's core, its invariant, and its analyze.sql; does not implement the base lesson for the learner.
---

# Add basic k6 testing

The learner wrote the base lesson: a core with two or more variants, a runner
that times them one at a time, an invariant it panics on, and an `analyze.sql`
that ends with the number the lesson is about. The performance follow-up is
**that same experiment under concurrent traffic**. A learner who has just read
their `measurements.csv` must recognise the same variants, the same invariant,
and the same final table in the k6 analysis, now with arrival rate and
concurrency as new axes.

It is not a generic HTTP benchmark. One endpoint, one fixed mode, and the
shared latency table are the floor, not the deliverable. If the output of two
different lessons would look the same apart from the numbers, the follow-up
has not been built yet.

## 1. Extract the lesson's experiment before writing anything

Read `AGENTS.md`, `knowledge/index.md`, `knowledge/lesson-cores.md`, and
`knowledge/performance-labs.md`. Start the `trinkets-work-log` skill. Then read
the lesson itself: `core/core.go`, `main.go`,
`analyze.sql`, and the current `measurements.csv`. Write these five lines into
the work log before designing anything; every later choice is checked against
them.

Read `docs/lessons/completed/<slug>.md`, falling back to
`docs/lessons/planned/<slug>.md` (or a user-supplied guide path) when present;
its `k6 build plan` section is the implementation brief. Also read a
lesson-local `K6_PLAN.md` if one exists. Follow the plan's workload phases,
metrics, isolation, and acceptance criteria after
checking them against the actual core and measurements. Update stale parts
of the plan when the base lesson has changed, explaining the adjustment.
If the base lesson is finished but its guide is still in planned, follow
`docs/lessons/README.md` to move it and update links. k6 is optional and is
not the completion gate for the base plan.
For lessons created with `$create-lesson`, keep the follow-up README focused
on run commands and interpreting the comparison; an extended writeup is
unnecessary.

- **Contrast:** the variants the runner compares, using the names from its
  variants or strategies table (`sequential` vs `pipeline`, `naive` vs
  `idempotent`, `for update` vs `skip locked`, `WAL` vs `DELETE`). A lesson
  with one mechanism has a knob instead (TTL, batch size, work time); say so.
- **Invariant:** the condition the runner panics on, and the two numbers it
  compares (stored sum vs batch size, counter total vs unique events).
- **Question:** what the last query in `analyze.sql` computes (a ratio of
  medians, an overcount, a p95 by mode). This is the number the k6 analysis
  must reproduce.
- **Knob:** the constant in the runner a learner would change next.
- **Baseline:** the base lesson's own numbers from `measurements.csv`, so the
  prediction prompt can ask "the base lesson measured X; what happens at
  N per second with M in flight?"

State one hypothesis in those terms: under concurrent traffic, the contrast
still shows the question's answer, and the invariant holds. If the lesson's
code has a defect, follow `AGENTS.md`: explain the location, cause, and
concept, and wait for permission. Build everything that does not depend on
the fix, and say what is blocked.

## 2. Put the contrast inside one run

The base lesson's table has one row per variant. The k6 analysis must too.
Decide how the contrast reaches the server:

- **Per-request variant** (the variant is a function or strategy choice the
  core exposes: increment mode, counter strategy, crash flag). Expose it as a
  validated request parameter, `POST /operation?variant=<name>`, answering 400
  for any name the core does not export. The workload alternates variants
  across iterations, so both receive the same arrival share under the same
  conditions, and tags each request with `variant`. k6 writes custom tags to
  the CSV `extra_tags` column as `variant=<name>`, which the lesson SQL
  extracts. One run then yields the base lesson's comparison table.
- **Server-level setting** (the variant is fixed at startup: journal mode,
  pool size, TTL, worker count). Read it with `perf.Choice` or `perf.Int`, so
  `GET /health` reports it and the run's `settings.json` records it. The lesson
  SQL prints those settings as its first table, and the walkthrough's default
  is a pair of runs, one per value, with the exact numbers to compare named.
  Never let a silent default mode stand in for the comparison.

When a per-request variant is possible, prefer it: the same run, the same
minute, the same background load. Use the server-level route only when the
core cannot switch per call. A knob-only lesson uses the server-level route
with two values of the knob.

The shared `request(method, path, tags)` helper in `scripts/perf/workload.ts`
merges extra tags with the fixed request name `operation`; `http.batch`
callers take the same params from `operationParams(tags)`. Keep
`name: operation` on every measured request so the shared SQL still excludes
stats and repair traffic. Do not write a second request helper.

## 3. Wrap the core, not the runner

- Import the existing `core/` package; `knowledge/lesson-cores.md`
  lists each entry point and its result contract. Extract more only if the
  chosen operation is still coupled to the runner, and then reuse it in the
  runner too. A server calls the core; it never reruns the experiment entry point.
- The adapter owns clients (created once, closed on shutdown), a unique
  fixture per server (private schema, key prefix, temporary database), and
  readiness. Fixture resets, experiment loops, CSV, and panics on dependency
  errors stay out of request handlers. Use the Go helpers in
  `internal/lab/perf` (`Database`, `Warm`, `Int`, `Choice`, `Limit`,
  `Serve`). HTTP adapters are Go only. Deno/TypeScript stays in the k6
  lifecycle and workloads. Never a second lifecycle.
- Warm every pooled connection before the listener starts (`perf.Warm` runs
  a function on each connection the pool may open): pgx prepares statements
  per connection, and a lazy dial otherwise lands inside a timed request.
  Take the connection before starting the timer when the core accepts one,
  so pool waits stay outside `elapsed_ms` as in the base runner.
- `POST /operation` calls the core once per request with bounded, validated
  inputs and returns what the core returned, plus the variant that served it.
  If the base runner timed the core call, time it the same way here and
  return it as `elapsed_ms`: the analysis can then show the lesson's own
  measurement beside HTTP latency and the learner sees what HTTP adds.
- `GET /stats` returns the invariant's two numbers, not a boolean, per variant
  when state is per variant (`expected_sum` and `actual_sum`; `total` and
  `seen`). The runner saves this as `domain.json`; the lesson SQL and the
  teardown check both read the comparison from it.
- Map core outcomes honestly: a skipped duplicate is not an applied one, an
  empty queue is not a completed job, a simulated crash is an experiment
  outcome in a 200 response, a measured write failure is not a setup error.
  Use meaningful statuses, request timeouts, and orderly shutdown.

## 4. Build the workload around the variants

`perf/k6.ts` re-exports `options` and `handleSummary` from
`scripts/perf/workload.ts` and adds only the lesson (`optionsFor(defaults,
extra)` when the lesson needs its own default rate, p95 budget, or k6
options). Declare the adapter's response shapes as interfaces and pass them to
`assertResponse<T>`, which returns the parsed body when both checks pass so
metrics are recorded from it, not inside the predicate. Workload-only knobs
come from `number(name, fallback, min, max)`; a knob the server owns is read
once in `setup()` with `settings<T>()` (from `/health`), never validated a
second time from the environment.

- The default function picks the variant for this iteration (round-robin over
  the core's names, or `exec.scenario.iterationInTest % n`), sends the request
  tagged with it, and checks the status and the invariant the base runner
  checks per call.
- Record the lesson's domain signal as a custom metric **tagged by variant**:
  a `Trend` of the server's `elapsed_ms`, a `Rate` of applied duplicates or
  locked writes. Tagging is what lets the analysis split it; an untagged
  metric only reports a blended mean. Do not add a metric that only echoes
  what the workload itself chose (a crash flag, a constant batch size): a
  cross-check that cannot fail teaches nothing.
- `teardown` reads `/stats` and asserts the lesson's invariant on the numbers
  (`everyVariant(names, predicate)` when `/stats` is `{variants: [...]}`).
  Report the invariant's inputs, not arithmetic on them: a field that is
  always the sum of two others, or that no teardown, SQL, or README reads,
  is noise.
  For asynchronous work, drain with a bounded wait first and do not report
  submission latency as completion latency.
- Profiles come from `workload.ts`: `smoke` (one user, ten seconds), `load`
  (steady arrival rate, sixty seconds), `stress` (stepped rate with holds),
  `spike` (burst and recovery), `soak` (five-minute practice run, longer by
  `DURATION_S`). Choose the default `RATE` so the slower variant sits below
  saturation at `load` and say so; the learner raises it to find the knee.
  Arrival-rate executors report dropped iterations; keep that threshold.
- Thresholds: checks must all pass, operation failure rate under one percent,
  and a labelled p95 budget. Stress may exceed the budget on purpose: keep the
  failure and explain it. Rate and profile overrides fail before traffic.

## 5. Make the analysis answer the lesson's question

`scripts/perf/analyze.sql` is the shared floor: latency by scenario and
status, failure rate, dropped iterations, request count, ten-second buckets,
and every custom metric. The runner appends the lesson's `perf/analyze.sql`
after it. That file is required here, and it holds the lesson:

1. **Settings legend:** the shared file already prints `settings.json` as
   its first table; add one `.print` line saying what the lesson's knobs
   mean (`BATCH_SIZE is batchSize in the base main.go`).
2. **The base table, by variant:** the same columns as the base `analyze.sql`
   where they mean the same thing (`p50_batch_ms`, `overcount`, `p95_ms`),
   computed from k6 samples grouped by `tag(extra_tags, 'variant')` (the
   shared macro; extract each tag once in a view). Show the server's
   `elapsed_ms` trend beside `http_req_duration` when the adapter returns it.
3. **The question:** the lesson's final number in the named-group form
   (`median(x) filter (where variant = 'a')` beside the same for `b`, then
   the ratio), so a mistyped variant name is visible as a NULL beside a value.
4. **The invariant:** `expected` and `actual` from `read_json('domain.json')`
   with their difference, per variant when the server reports it that way.

Read [k6-duckdb.md](references/k6-duckdb.md) for the CSV shape, the tag
extraction, and the JSON reads. Filter by `metric_name` and `name` before
counting; never divide percentiles across buckets; keep failed requests
visible. Keep the base lesson's `measurements.csv` and `analyze.sql` untouched.

## 6. Write the walkthrough in the lesson's own terms

`perf/README.md` is a 10–20 minute session. Put the hypothesis first, then a
prediction prompt that quotes the baseline: "the base lesson measured 33 ms
sequential and 0.3 ms pipelined for one batch; write down your guess for each
at five batches per second." Then:

- Exact commands from the repository root: start services, the default run
  (smoke plus the one profile that answers the question, or the pair of
  server-level runs), and `analyze-k6-<lesson>` to repeat the SQL.
- Which table to look at first and which two numbers to compare, by the
  column names the SQL prints.
- One line per optional profile: what question it asks for this lesson, and
  its command. Do not run them all by default; keep default traffic under
  five minutes.
- Three interpretation questions that only this lesson can raise, and one
  knob for a second run with a predicted direction.
- Which measurements describe HTTP, which describe the domain, and what a
  laptop over loopback cannot establish about production capacity or uptime.

## 7. Verify against the lesson, then report

Check tools before installing anything (`k6`, `duckdb`, `go`, `deno`). Run:

- `make check`; `k6 inspect` on the workload; an invalid `PROFILE` must fail
  before traffic.
- `make lab-k6-<lesson>` smoke: every variant appears in the by-variant table
  with a plausible share of the requests; the per-variant counts sum to the
  shared operation count; the invariant table shows expected equal to actual;
  the custom metric splits by variant.
- The profile the walkthrough recommends, at its default rate, short enough
  to stay in budget (`DURATION_S`). Confirm the question's number has the same
  direction as the base lesson and note how far it moved.
- The base lesson (`make lab-<lesson>`) still runs when its fixtures are safe,
  and no server fixture (schema, keys, temporary file) remains afterwards.

Report the files changed, the exact next commands, the observed by-variant
numbers, which profiles were not run, and anything unexplained (a gap between
HTTP and core latency, a variant nearer saturation than expected). Then run
`update-trinkets-knowledge` (add the core row to `lesson-cores.md` and the
verification to `performance-labs.md`) and close the work log.

## Mechanics already in place

Read `knowledge/performance-labs.md` for detail; in brief:

- The Makefile discovers `lessons/go/*/perf/k6.ts` and adds `serve-`, `k6-`,
  `analyze-k6-`, and `lab-k6-` targets. Keep the base `main.go`
  so lesson discovery still works; the adapter is `perf/main.go`.
- `scripts/perf/run.ts` builds or starts the server, waits for its URL, runs
  k6 with CSV output, saves `settings.json` (from `/health`), `workload.json`
  (from `k6 inspect --include-system-env-vars`), `domain.json` (from
  `/stats`), and logs into `perf/results/<run-id>/`, runs the shared then the
  lesson SQL, and stops its own server. It keeps k6's exit status.
- The root `deno.json` maps `k6`, `k6/http`, `k6/execution`, `k6/metrics`, and
  `k6/options` to `@types/k6`; add a mapping before importing another module.
  `perf/results/` is excluded from `deno check`.
- Never clean a whole backing service to reset an experiment; fixtures are
  per server and cleaned on normal shutdown.
