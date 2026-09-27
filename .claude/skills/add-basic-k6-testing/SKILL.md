---
name: add-basic-k6-testing
description: Add a small HTTP server and local k6 performance experiments to a completed Go or Deno systems-trinkets lesson, with lightweight DuckDB analysis and a 10–20 minute learner walkthrough. Use when asked to add basic k6 testing, wrap a finished lesson for load testing, or add a performance follow-up. Does not implement the base lesson for the learner.
---

# Add basic k6 testing

The learner writes the base lesson; the agent builds its performance extension.
Deliver working local scaffolding and a short guided experiment that helps the
learner predict, run, inspect, and explain behavior. Budget 10–20 minutes of
learner time after setup, not 10–20 minutes for every possible test profile.

## Establish the experiment

Read the repository instructions, `knowledge/index.md`, the selected lesson,
its analysis, and the relevant runtime helpers. Read `knowledge/lesson-cores.md`
for the existing callable boundaries and result semantics. Use the repository work-log and
knowledge skills. If the target is unclear, ask which lesson; otherwise proceed.
Respect unfinished exercises and existing edits.

State one hypothesis tied to the lesson and choose one core operation to expose.
Examples: a cache miss burst increases database work; arrivals exceeding worker
capacity grow the queue; duplicate events must not inflate a counter.

Adding the adapter and extracting shared logic are authorized by this workflow.
If you find a defect in the learner's existing code, follow `AGENTS.md`: explain
the location, cause, and concept, and wait for explicit permission to fix that
defect. Do not silently solve it while extracting functions or adding concurrency.
Correct defects introduced in your own scaffolding. If a base defect blocks the
experiment, finish independent scaffolding and explain the blocker.

## Wrap one operation

- The current Go lessons already expose importable `core/` packages; the Deno
  lesson exposes `createOperations` from `core.ts`. Use those entry points first.
  Extract additional logic only when the selected operation is still coupled to
  a runner. Importing `main.ts` executes setup, resets, and experiments; a server
  must import `core.ts` instead.
- Reuse the core operation in the original runner and HTTP handler. Preserve the
  original run/analyze commands and measured behavior. Keep fixtures, destructive
  resets, experiment loops, and report generation outside request handling.
- Prefer Go `net/http` or `Deno.serve`, a localhost listener with configurable
  port, a readiness route, and one meaningful operation route. Use meaningful
  HTTP statuses, bounded inputs, request timeouts, and orderly shutdown.
- Create clients/pools once, close them on shutdown, and use concurrency-safe
  access. A sequential lesson's single connection or CSV writer may need a small
  server-specific adapter. Avoid a global lock that accidentally serializes the
  measured operation. Keep business semantics explicit.
- For Go, use a small server entry point importing the existing core, or an
  opt-in serve mode that branches before the runner's fixture setup. Keep the
  existing `main.go` so Make still discovers the lesson. For Deno, add a small
  server entry point that constructs `createOperations` with its own clients.
  The server owns schema/bucket initialization and client shutdown explicitly.
- Map core outcomes deliberately: counter `total` is meaningful only when
  `applied` is true; queue `pgx.ErrNoRows` means no job was claimed; SQLite's
  `WriteErr` is a measured write failure distinct from setup errors. Deno's
  simulated crash returns early without throwing. Use these contracts when
  designing responses and k6 checks; do not report a skipped operation as a
  completed one. See `knowledge/lesson-cores.md` for concurrency and scope limits.
- Use lesson-owned test data and an explicit bounded seed/reset command. Never
  clean an entire backing service to prepare a performance run. State whether
  each profile expects a warm cache, cold cache, or fresh queue.
- Measure one domain signal when it explains the hypothesis: cache misses,
  completed jobs, duplicate applications, or repair backlog. For asynchronous
  work, distinguish HTTP acceptance from completion; include a bounded drain and
  final invariant check. Do not claim job completion latency from submission
  response times. Add a tiny domain CSV only if necessary for the experiment.

## Build a small local workload

Keep the workload in `perf/k6.js` under the lesson. k6 is a separate executable;
its imports do not belong in the Deno import map or a Node package install.
Use one script with a validated `PROFILE` selection, defaulting to smoke.
Include these selectable profiles with configurable, conservative local rates:

| Profile | Suggested duration | Learning question |
|---|---|---|
| smoke | 10–20 seconds, one virtual user | Are the requests and correctness checks valid? |
| load | 60 seconds at a steady arrival rate | Does the system keep up with the chosen demand? |
| stress | 2–3 minutes, increasing rate with holds | Where do latency, failures, or backlog rise? |
| spike | 60–90 seconds, low/burst/low | How does it recover after a sudden burst? |
| soak | 5 minutes by default, duration configurable | Does a chosen signal drift during sustained use? |

Call the short soak a practice run; offer 30–60 minutes as an optional extension.
Run smoke plus one hypothesis-relevant profile in the default walkthrough.
Keep total default load generation under five minutes. Do not automatically run
every profile or a long soak. Do not manufacture overload by silently increasing
load until the machine freezes: bound virtual users, requests, payloads, fixture
growth, and duration. Report if the chosen range did not reach saturation.

Use arrival-rate executors for rate experiments. Explain iterations/second and
requests/second separately if one iteration makes multiple requests. Preallocate
adequate virtual users, set a finite maximum, and report dropped iterations.
Use short transitions and explicit holds for a spike; a long ramp is a different
experiment. Rate overrides must reject invalid values before traffic starts.

Check the expected status and a meaningful response invariant. Configure expected
statuses consistently with failure metrics. Add thresholds for correctness and
clearly labelled, configurable latency/error budgets; checks alone do not fail a
k6 run. Stress may deliberately exceed a budget: retain the failure and explain
it rather than weakening the threshold to make it pass. Use stable request names
so polling/setup traffic can be excluded from operation measurements.

## Keep analysis lightweight

Read [k6-duckdb.md](references/k6-duckdb.md) when generating the analysis.
Default to native k6 CSV output and one `perf/analyze.sql` with two or three
labelled queries. Keep the original lesson's `measurements.csv` and `analyze.sql`
separate. No metrics database, dashboard server, or conversion pipeline is needed.

Keep run artifacts in an ignored `perf/results/<run-id>/` directory. Record the
profile, rate schedule, durations, relevant server settings, data preparation,
and tool versions beside the raw output. Make the working directory and input
path unambiguous in runnable commands. If using a runner, preserve k6's exit code
while still permitting analysis of failed runs, and clean up only its own server.

If useful DuckDB analysis would require substantial telemetry plumbing, keep the
client CSV summary and explain its limits. Defer richer correlation. If DuckDB
is unavailable, still provide the SQL and installation guidance; identify the
analysis as unexecuted instead of replacing missing results with invented data.

## Deliver and verify

Provide a short `perf/README.md` with:

1. The hypothesis and a prediction prompt.
2. Exact commands to start dependencies, seed, serve, run smoke and one selected
   profile, analyze, and stop; give their working directories. Reuse repository
   Make targets where available, adding only small explicit targets if helpful.
3. A plain-language description and runnable command for each optional profile.
4. Three interpretation questions and one knob to change for a second run.
5. Which measurements describe HTTP, which describe domain work, and what this
   local experiment cannot establish about production capacity or long uptime.

Check available tools before installing anything. Run a focused build/type check,
readiness check, and smoke profile against isolated lesson data. Run the SQL over
the resulting CSV and confirm its counts agree with the k6 summary, allowing for
documented filters. Exercise one short comparison if needed to substantiate the
chosen signal. Verify the original lesson entry path is preserved; run it when
its fixture effects are safe. Do not claim an unrun command passed.

Finish with changed files, exact next commands, observed results, and any blocked
validation. Explain the hypothesis without claiming unmeasured outcomes. Follow
the repository knowledge workflow. Keep the learner's reading focused on the
handler, the workload scenario, and the analysis; avoid a generalized framework.
