---
name: create-lesson
description: Create or import a short systems-trinkets lesson as runnable Go or Deno code, concise run steps, DuckDB analysis, and a concrete K6_PLAN.md for later AI implementation. Use when asked for a new lesson, the next systems lesson, or to bring a lesson into this repo. Creates the base experiment; k6 implementation is a subsequent step.
---

# Create lesson

Create one small runnable experiment in this repository. A bare
`$create-lesson` is enough: choose a fresh topic and create its files.
Honor a supplied lesson, topic, language, store, or output path. Keep prose
to the commands and interpretation needed to run the code.

## Read the specification and current context

Resolve repository paths from the repo root. Read
[`docs/prompts/daily_lesson/v3.txt`](../../../docs/prompts/daily_lesson/v3.txt)
in full on each invocation. It is the detailed specification for scope,
mechanism, measurements, and code boundaries. This skill's local output
format replaces v3's long writeup, code fences, and short k6 paragraph.
Keep that specification in one place rather than copying it into this skill.
The following adaptations make the ChatGPT prompt work inside the repository:

- Use actual repository context instead of the prompt's assumption that the
  author cannot access the repo. Read `AGENTS.md`, `knowledge/index.md`, and
  relevant knowledge entries; use `trinkets-work-log` for the task.
- Inspect `git status --short`, the lesson directories under both
  `lessons/go/` and `lessons/deno/`, and existing guides in `docs/lessons/`
  if present. Read enough nearby runners, cores, and analysis to identify
  topics already covered. Guides count as prior topics even before their
  code has been typed. Do not overwrite existing work.
- Check `Makefile`, `services/index.md`, `go.mod`, `deno.json`, and
  `lessons/deno/deno.json`. Read the chosen runtime's shared helpers:
  `internal/lab/` or `lessons/deno/lab/`, including the relevant service
  helper. Current source determines available APIs and dependencies; v3's
  helper list is a snapshot. Reuse helpers rather than inventing aliases or
  reimplementing their functionality.
- When importing a URL or supplied lesson, read the actual content first.
  Preserve its question and mechanism while adapting to this repository.
  If the source is inaccessible, request its contents and continue independent
  work; do not substitute a guessed lesson.
- Use only observed lessons to explain topic variety. Do not imply that
  unseen ChatGPT conversations or other daily lessons were inspected.

## Choose one bounded experiment

Follow v3's scope: one main systems property, one concrete question, and an
observable invariant or outcome. Aim for about 20 minutes overall, with
roughly 10 minutes for typing, running, and analysis. Simplify the mechanism
if the code would exceed that budget.

Use Go or Deno and one or at most two primary stores from PostgreSQL,
DuckDB, Valkey, and SeaweedFS. Always analyze measurements with DuckDB;
analysis-only DuckDB does not count toward that limit. Exclude SQLite from
new lessons even though an older lesson uses it.

If no topic was requested, pick a fresh mechanism or explain a distinct
angle on an existing topic. Vary language, storage, architecture, and
property across the visible lessons where useful. Favor concepts relevant
to AI sandbox and workflow platforms. Firecracker, Kata, or similar runtime
technology belongs only when its prerequisites and experiment fit the small
lesson; state Linux/KVM or other requirements explicitly.

Choose a lowercase kebab-case slug without a lesson number, unique across
both runtimes and existing guides. Record the chosen question, invariant,
baseline or knob, language, stores, and slug in the work log, then write the
files. Topic selection does not require a separate approval step.

## Create the local files

Write complete source, with no TODOs or missing implementation:

- Go: `lessons/go/<slug>/main.go`, `core/core.go`, and `analyze.sql`.
- Deno: `lessons/deno/<slug>/main.ts`, `core.ts`, and `analyze.sql`.
- Both: a short `README.md` with the question, exact service startup and
  `make lab-<slug>` / `make analyze-<slug>` commands, expected invariants,
  measurement boundaries, and a link to `K6_PLAN.md`. Fill in the real slug.
  Cite an imported lesson's URL. Skip the long overview, architecture
  diagram, duplicated source blocks, and extended tutorial prose.
- Both: `K6_PLAN.md`, an actionable handoff for the later AI build.

## Define the k6 follow-up before handing off

Read `.claude/skills/add-basic-k6-testing/SKILL.md` and the shared performance
conventions to make the plan implementable. Plan only; create `perf/` when
the user later requests the follow-up. `K6_PLAN.md` must specify:

1. The base question, exact variant names, invariant, and hypothesis under
   concurrent traffic. Explain how one run compares variants, or why a
   startup setting requires a pair of runs.
2. The actual exported core operations and result/error meanings. Specify
   the operation endpoint, validated inputs, example JSON, and whether a
   successful response means accepted, applied to storage, or completed.
   Name `/health` settings and `/stats` domain counters used by checks.
3. The workload lifecycle: setup, coordinated overlap, interruption or
   failure injection if relevant, bounded waits/drain, teardown, and cleanup.
   Identify which calls are setup and which are measured. Each request
   calls a core operation or a defined unit of work, never the base runner.
4. Unique fixture namespaces and owner/operation IDs, concurrency and
   growth caps, and how parallel runs avoid collisions. Specify an
   observable correctness check instead of assuming success replies prove
   the invariant. Separate intentional baseline failures from test failures.
5. Domain metrics with units and variant tags, how to obtain them, and the
   final-state check when HTTP latency misses the lesson's property. Name
   the DuckDB tables/columns that reproduce the base comparison. Exclude
   unobserved/censored latencies from percentiles and show their count.
6. Concrete smoke and one useful load experiment, starting settings,
   one knob and comparison values, correctness thresholds, and a clearly
   labelled latency budget if useful. Keep future commands labelled as
   available only after the adapter exists. State acceptance criteria and
   what the experiment cannot establish.

Name the files the AI will build: `perf/main.go` or `perf/server.ts`,
`perf/k6.ts`, `perf/analyze.sql`, and a concise `perf/README.md`. Reuse the
shared runner, workload helpers, and analysis. End the plan with
`$add-basic-k6-testing lessons/<runtime>/<slug>` using the concrete path.

Keep the core callable with caller-owned clients and explicit settings;
the runner owns fixtures, resets, timing, CSV, checks, and cleanup. Preserve
v3's transaction boundaries, expected-outcome/error distinction, bounded
concurrency, and lesson-specific fixtures. No per-lesson module manifests,
lockfiles, Compose files, or Makefiles. Include full local development
credentials wherever connection details appear.

## Review and hand off

Before delivering, check that the code agrees with the inspected
helper signatures and shared dependencies. Trace the invariant through the
operation, runner checks, CSV columns, and SQL results. Check units, variant
names, warm-up and reset behavior, timing boundaries, and controlled overlap
where concurrency matters. Distinguish simulated interruptions from actual
crashes and keep production claims within what the experiment demonstrates.

Do not invent measured results or claim execution based on inspection.
Label example output as illustrative. Run focused compilation/type checks
and the base experiment when local services are available; report any
checks that could not run. Use isolated fixtures and preserve existing
lesson work. Errors discovered
in the user's existing code follow `AGENTS.md`: explain their location,
cause, and concept, and wait for an explicit request before fixing them.

Apply `update-trinkets-knowledge` when there is a reusable finding, then
close this task's work log. The final response links the code/run steps and
k6 plan, with the exact next command and verification limits.

Example invocations:

```text
$create-lesson
$create-lesson about expiring worker leases in Go with PostgreSQL
$create-lesson from <shared lesson URL>
```
