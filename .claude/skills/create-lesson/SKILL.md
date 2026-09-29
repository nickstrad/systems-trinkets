---
name: create-lesson
description: Create or import a systems-trinkets lesson as one Markdown file readable with glow, with complete Go or Deno code to type, terminal architecture visuals, run steps, DuckDB analysis, and a concrete k6 build plan. Lessons use only software marked configured in software/software.md. Use when asked for a new lesson, the next systems lesson, or to bring a lesson into this repo. k6 implementation is a subsequent step.
---

# Create lesson

Create one lesson in `docs/lessons/<slug>.md` for the learner to open with
`glow` and type beside their editor. A bare `$create-lesson` is enough:
choose a fresh topic and create the Markdown file.
Honor a supplied lesson, topic, language, service, or output path. Keep prose
to the commands and interpretation needed to run the code.

## Read the specification and current context

Resolve repository paths from the repo root. Read
[`lesson-spec.md`](lesson-spec.md) in this skill folder in full on each
invocation. It is the contract for scope, software, layout, helpers, core
boundaries, and measurement. Then gather the real context:

- Read `AGENTS.md`, `knowledge/index.md`, and relevant knowledge entries;
  use `trinkets-work-log` for the task.
- Read `software/software.md`. Only rows with `Configured: yes` may appear
  in a lesson. Note each candidate's `Setup` column: it holds the
  `make up-<service>` name, the connection string with full credentials, and
  any client package the lesson will need.
- Inspect `git status --short`, the lesson directories under both
  `lessons/go/` and `lessons/deno/`, and existing guides in `docs/lessons/`.
  Read enough nearby runners, cores, and analysis to identify topics already
  covered. Guides count as prior topics even before their code has been
  typed. Do not overwrite existing work.
- Check `Makefile`, `go.mod`, `deno.json`, and `lessons/deno/deno.json`.
  Read the chosen runtime's shared helpers in `internal/lab/` or
  `lessons/deno/lab/`. Current source determines available APIs and
  dependencies; the spec's helper list is a snapshot. Reuse helpers rather
  than inventing aliases or reimplementing them.
- When importing a URL or supplied lesson, read the actual content first.
  Preserve its question and mechanism while adapting to this repository.
  If the source is inaccessible, request its contents and continue
  independent work; do not substitute a guessed lesson.
- Use only observed lessons to explain topic variety. Do not imply that
  unseen conversations or other lessons were inspected.

## Choose one bounded experiment

Follow the spec's scope: one main systems property, one concrete question,
and an observable invariant or outcome. Aim for about 20 minutes overall,
with roughly 10 minutes for typing, running, and analysis. Simplify the
mechanism if the code would exceed that budget.

Use Go or Deno and one or at most two primary services from the configured
rows of `software/software.md`. Always analyze measurements with DuckDB;
analysis-only DuckDB does not count toward that limit. Exclude SQLite from
new lessons even though an older lesson uses it. If a requested topic needs
software that is not configured, say which catalog row would need to be
configured first, then pick the nearest idea that works with configured
software. Never ask the learner to install software or add Docker
configuration as part of a lesson.

If no topic was requested, pick a fresh mechanism or explain a distinct
angle on an existing topic. Vary language, service, architecture, and
property across the visible lessons where useful. Favor concepts relevant
to AI sandbox and workflow platforms, as the spec describes.

Choose a lowercase kebab-case slug without a lesson number, unique across
both runtimes and existing guides. Record the chosen question, invariant,
baseline or knob, language, services, and slug in the work log, then write
the Markdown file. Topic selection does not require a separate approval step.

## Write one Markdown lesson

Keep the deliverable self-contained and readable with
`glow docs/lessons/<slug>.md`. Include these parts:

- A title and one or two sentences naming the property and question.
  Cite an imported lesson's URL.
- A compact architecture diagram and, when useful, a timeline or state
  transition visual. Apply an available drawing/visualization skill's
  relevant design guidance, while respecting its output scope. The required
  visuals must render in glow: use aligned ASCII or Unicode diagrams in
  fenced `text` blocks. Use labelled paths and arrows; keep diagrams within
  about 76 columns. Image links, HTML, and Mermaid source cannot be the only
  way to understand the architecture in a terminal.
- Complete source in language-tagged code fences, each under its exact
  repo-relative destination path. Go needs `main.go`, `core/core.go`, and
  `analyze.sql`; Deno needs `main.ts`, `core.ts`, and `analyze.sql`. No TODOs
  or missing implementation. Include a small file tree.
- Exact directory creation, the `make up-<service>` commands for the
  services used, and `make lab-<slug>` / `make analyze-<slug>` commands.
  Fill in the real slug. Explain that lesson Make targets appear after the
  learner creates the source files. Briefly name expected invariants and
  measurement boundaries.
- A `## k6 build plan` section containing the actionable handoff below.

Keep explanatory prose short; use code comments for mechanism details.
By default, write only the Markdown lesson, leaving the source for the
learner to type. Create executable source too only if explicitly requested.
Do not create a separate README or K6_PLAN.md alongside the guide.

## Define the k6 follow-up before handing off

Read `.claude/skills/add-basic-k6-testing/SKILL.md` and the shared performance
conventions to make the plan implementable. Plan only; create `perf/` when
the user later requests the follow-up. The lesson's k6 build plan must specify:

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
`$add-basic-k6-testing lessons/<runtime>/<slug>` and the Markdown guide path,
using concrete paths. The follow-up reads this plan after the learner has
completed the base lesson.

Keep the core callable with caller-owned clients and explicit settings;
the runner owns fixtures, resets, timing, CSV, checks, and cleanup. Preserve
the spec's transaction boundaries, expected-outcome/error distinction,
bounded concurrency, and lesson-specific fixtures. No per-lesson module
manifests, lockfiles, Compose files, or Makefiles. Include full local
development credentials wherever connection details appear.

## Review and hand off

Before delivering, check that the code agrees with the inspected
helper signatures and shared dependencies, and that every service the lesson
touches is a configured row in `software/software.md`. Trace the invariant
through the operation, runner checks, CSV columns, and SQL results. Check
units, variant names, warm-up and reset behavior, timing boundaries, and
controlled overlap where concurrency matters. Distinguish simulated
interruptions from actual crashes and keep production claims within what the
experiment demonstrates.

Do not invent measured results or claim execution based on inspection.
Label example output as illustrative. For verification, extract code into
an isolated scratch workspace with the same module/import-map layout and
shared helpers, then run focused compilation/type checks and the base
experiment when local services are available. Keep scratch source and CSV
out of the learner's lesson directories. Report exact checks and outcomes,
including checks that could not run. Use isolated fixtures and preserve
existing lesson work. Errors discovered
in the user's existing code follow `AGENTS.md`: explain their location,
cause, and concept, and wait for an explicit request before fixing them.

Apply `update-trinkets-knowledge` when there is a reusable finding, then
close this task's work log. The final response links the Markdown lesson,
gives its glow command, and states verification limits.

Example invocations:

```text
$create-lesson
$create-lesson about expiring worker leases in Go with PostgreSQL
$create-lesson from <shared lesson URL>
```
