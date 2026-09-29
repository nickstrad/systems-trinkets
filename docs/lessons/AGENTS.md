# Lesson docs

Follow the repository's learning rules and the [lesson lifecycle](README.md).

## Local runtime boundary

Every lesson idea and complete project must run locally on macOS using Docker
Desktop/Compose and Go. Deno/TypeScript is reserved for k6 tooling; lesson cores,
runners, HTTP adapters and platform components are Go only. Existing DuckDB
analysis and k6 load tooling stay in scope. Run Linux-specific harnesses inside Docker. Exclude Firecracker and
all KVM-dependent tools, nested virtualization, separately managed Linux VMs,
Apple container and alternate VM runtimes from project paths for now.
Containerd/nerdctl is allowed when useful to a specific lesson, with its Linux
engine running locally on the Mac; Docker remains the backing-service default.
Excluded tools may appear in provider research only.
Use ARM64-compatible images on Apple silicon. Keep unimplemented Docker harnesses
blocked until configured; portability alone does not make a lesson ready.

## Organize ideas by platform aspect

`ideas.md` is a collection of Markdown tables, one per core aspect of an agentic
platform: execution, lifecycle, scheduling, storage, networking, durable work,
and other abstractions identified in `docs/agentic-platforms/`. Organize by the
mechanism being learned rather than by vendor. Read the relevant research notes
before adding or revising a category or idea; keep links to the evidence.

Each aspect has a short description of its abstractions, relevant research
links, and the same table columns:

- **Order:** unique global impact rank across every aspect; 1 is highest.
- **Idea / question:** stable kebab-case slug and the concrete question.
- **Mechanism / comparison:** the behavior to build and its baseline or knob.
- **Invariant:** the observable correctness condition.
- **Measurement:** the outcome or metric, including units where applicable.
- **Software / readiness:** required software and `ready` or `blocked`, naming
  missing prerequisites. Recheck `software/software.md`; only `Configured: yes`
  dependencies are eligible for a working plan.
- **Research:** a relevant platform note or primary source supporting the
  motivation; distinguish a local analogy from the provider's implementation.
- **Status / plan:** `idea`, `planned`, or `completed`, linking the guide once
  one exists. Readiness and lifecycle status are separate.

Keep cells concise. Put an idea under its main aspect once; cross-reference
related aspects instead of copying rows. Split an aspect only when it improves
discovery. Ideas may involve unconfigured software if the blocker is explicit.

When asked for structure only, leave tables with headers and separators but no
body rows: no sample ideas, placeholder rows, or automatic backlog seeding.
Populate them only when asked to add ideas. Category descriptions name
abstractions, not disguised project proposals.

## Maintain one total impact order

Rank every lesson idea across the aspect tables, including planned/completed and blocked
entries, with unique consecutive integers from 1 through the total row count.
Never restart numbering per aspect, use ties, or substitute priority buckets.
Sort each table by Order. Slugs are stable identifiers; ranks can change.

Judge impact by how much knowing the lesson improves the learner's ability to
build and operate these platforms compared with the other ideas. Consider:

1. Prevention of tenant-boundary failures, incorrect effects, and lost work.
2. Reuse across sandbox and workflow platforms and across their components.
3. Foundational value: how many other mechanisms become easier to understand.
4. Operational value for recovery, diagnosis, capacity, and performance.

Use those considerations as editorial judgment, not an invented numeric score.
For close choices, favor the concept that unlocks more later lessons, then use
lexicographic slug order as a deterministic final tie-break. Explain the ranking
rationale briefly in ideas.md and reconsider the whole ordering when adding,
removing, or materially changing ideas. Renumber as needed to retain 1..N.

Impact is independent of software readiness, difficulty, and lifecycle status.
Do not demote an important idea just because its runtime is not configured.
Order is not a strict prerequisite sequence. By default, select the smallest
Order among eligible unplanned ideas; honor an explicitly selected idea and
check its prerequisites. Do not recreate already planned/completed work.

Validate that every row has one unique rank and slug and that the combined
ranks equal 1..N. The initial pass used three rows per aspect; later research
may add rows without changing the global ordering rule.

## Plans and completion

`$create-lesson` turns an existing eligible idea into a working guide in
`planned/`. The learner writes the base source unless explicitly requesting AI
implementation. Move a plan to `completed/` when the base work is finished,
following the completion evidence rules in README.md; k6 remains optional.
Update a matching idea row's status/link when present. An empty backlog does
not invalidate existing plans or require creating retrospective idea rows.

## Potential projects and Other side quests

Keep potential projects at the bottom of ideas.md. Each proposal states its
value and small first scope, suggests an ordered set of existing idea slugs,
shows its architecture, and names the remaining integration work. A project
sequence can differ from global impact order because of dependencies/readiness.
Do not promote a proposal to planned or claim the platform is implemented.

Use the repo-local `.claude/skills/draw-visual/SKILL.md` for project diagrams;
keep Mermaid sources in `diagrams/`, regenerate embeds, and visually inspect
70/80-column previews. Explain every diagram component in one or two sentences,
marking its coverage as Idea (existing slug), Gap (Other ID), or Fixture. A
component may need both a lesson mechanism and unplanned integration.

The Other table holds tentative integration side quests with stable Oxx IDs,
project association, required connections, completion condition, and status.
It is not a ranked idea table. Promote a side quest into the ranked backlog
only when deliberately defining it as a lesson; retain the cross-reference.

CLI/UI and client presentation are outside the learning scope and assumed to
be fully AI-generated. Their backend contracts, authenticated identities,
authorization and recovery remain in scope. Distinguish a runnable model from
a deployed isolation/runtime boundary; configured services alone do not supply
the missing integration. Never call glue implemented just because a related
lesson exists.
