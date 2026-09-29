---
name: create-lesson
description: Turn an existing idea in docs/lessons/ideas.md into a complete working lesson plan in docs/lessons/planned, using only configured software. Use when asked to create, build, or plan the next systems-trinkets lesson. Includes code for the learner to type, DuckDB analysis, and a later k6 build plan; does not implement the learner's source by default.
---

# Create lesson

Turn one existing idea from `docs/lessons/ideas.md` into a complete working
plan at `docs/lessons/planned/<slug>.md`. The learner opens it with `glow`
and types the code beside their editor. A bare `$create-lesson` selects an
eligible unplanned idea; it does not invent a new topic. "Implement the idea"
means authoring this runnable guide, not writing the learner's source files.
Honor a supplied idea, language, or service within the configured-software gate.

Ideas, plans, and completed work have separate homes. Read
`docs/lessons/AGENTS.md` for the table schema and global impact order, and
`docs/lessons/README.md` for lifecycle and completion criteria. Keep the
idea entry and its plan link current; do not move a freshly authored guide to
`completed/`. Adding potential projects to `ideas.md` is a separate ideation
task, optionally informed by `docs/agentic-platforms/`.

## Read the specification and current context

Resolve repository paths from the repo root. Read
[`lesson-spec.md`](lesson-spec.md) in this skill folder in full on each
invocation. It is the contract for scope, software, layout, helpers, core
boundaries, and measurement. Then gather the real context:

- Read `AGENTS.md`, `knowledge/index.md`, and relevant knowledge entries;
  use `trinkets-work-log` for the task.
- Read `docs/lessons/ideas.md` and inspect both `docs/lessons/planned/` and
  `docs/lessons/completed/`. Read relevant platform research linked by the
  selected idea; distinguish sourced platform behavior from local analogies.
- Read `software/software.md`. Only rows with `Configured: yes` may appear
  in a lesson. Note each candidate's `Setup` column: it holds the
  `make up-<service>` name, the connection string with full credentials, and
  any client package the lesson will need.
- Inspect `git status --short`, the lesson directories under
  `lessons/go/`, and existing guides in `docs/lessons/`.
  Read enough nearby runners, cores, and analysis to identify topics already
  covered. Guides count as prior topics even before their code has been
  typed. Do not overwrite existing work.
- Check `Makefile`, `go.mod`, and the k6-only `deno.json`.
  Read the Go shared helpers in `internal/lab/`. Current source determines available APIs and
  dependencies; the spec's helper list is a snapshot. Reuse helpers rather
  than inventing aliases or reimplementing them.
- When importing a URL or supplied lesson, read the actual content first.
  Preserve its question and mechanism while adapting to this repository.
  If the source is inaccessible, request its contents and continue
  independent work; do not substitute a guessed lesson.
- Use only observed lessons to explain topic variety. Do not imply that
  unseen conversations or other lessons were inspected.

## Runtime boundary

Lesson and platform implementation is Go only. Deno/TypeScript remains for
k6 workloads and orchestration. Follow the repository's local Mac policy:
Docker services and container harnesses, optional configured containerd/nerdctl
for a specific learning benefit, no KVM or separately managed Linux VM.
A goroutine is not a permissions sandbox; use a configured process/container
harness for enforced isolation and retain its readiness blocker until then.

## Choose one bounded experiment

Follow the spec's scope: one main systems property, one concrete question,
and an observable invariant or outcome. Aim for about 20 minutes overall,
with roughly 10 minutes for typing, running, and analysis. Simplify the
mechanism if the code would exceed that budget.

Use Go only and one or at most two primary services from the configured
rows of `software/software.md`. Always analyze measurements with DuckDB;
analysis-only DuckDB does not count toward that limit. Exclude SQLite from
new lessons even though an older lesson uses it. If an idea needs
unconfigured software, leave it in ideas and record the missing catalog rows
(or missing entries). For an explicitly selected blocked idea, explain the
blocker without substituting a different topic. For a bare invocation, select
another eligible idea; if none exists, report that no idea is ready. Never ask
the learner to install software or add Docker configuration as part of a lesson.

Select the requested idea by its stable slug or title. Without a selection,
choose the unplanned idea with the smallest global `Order` whose entire
required stack is configured. Reuse its stable slug, check for existing guides,
and explain its distinct angle if related lessons already exist. Recheck the catalog even if ideas.md says
"ready". If a supplied topic or imported lesson is absent from ideas.md,
record it there first because the user has explicitly supplied the idea;
then apply the same readiness check. Do not silently add invented topics.

Choose a lowercase kebab-case slug without a lesson number, unique across
existing Go lessons and guides. Record the chosen question, invariant,
baseline or knob, services, and slug in the work log, then write
the Markdown file and link it from the idea entry with status `planned`.
Topic selection does not require a separate approval step.

## Write one Markdown lesson

Keep the deliverable self-contained and readable with
`glow docs/lessons/planned/<slug>.md`. Include these parts:

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
  `analyze.sql`. No TODOs
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

Name the files the AI will build: `perf/main.go`,
`perf/k6.ts`, `perf/analyze.sql`, and a concise `perf/README.md`. Reuse the
shared runner, workload helpers, and analysis. End the plan with
`$add-basic-k6-testing lessons/go/<slug>` and the Markdown guide path,
using concrete paths. The follow-up reads this plan after the learner has
completed the base lesson.

Keep the core callable with caller-owned clients and explicit settings;
the runner owns fixtures, resets, timing, CSV, checks, and cleanup. Preserve
the spec's transaction boundaries, expected-outcome/error distinction,
bounded concurrency, and lesson-specific fixtures. No per-lesson module
manifests, lockfiles, Compose files, or Makefiles. Include full local
development credentials wherever connection details appear.

## Complete a plan when its base work is done

When authorized base lesson work is finished, or the learner asks to mark a
plan complete, follow the completion criteria
in `docs/lessons/README.md`: inspect the base implementation and record actual
validation or the learner's completion report. Move the existing guide to
`docs/lessons/completed/<slug>.md`, update its self-references and inbound
links (including ideas.md), and mark that idea completed. Never overwrite an
existing destination or infer completion from scratch verification alone.
Optional k6 work does not hold a completed base plan in planned.

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
an isolated scratch workspace with the same Go module layout and
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
$create-lesson from the outbox-redelivery idea in docs/lessons/ideas.md
$create-lesson from <shared lesson URL>
```
