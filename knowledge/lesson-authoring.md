# Creating a lesson

Use `$create-lesson` to turn an existing idea in `docs/lessons/ideas.md` into
a working lesson guide. Optional arguments select an idea or store.

- The canonical skill is `.claude/skills/create-lesson/SKILL.md`.
  `.agents/skills/create-lesson` is a relative symlink to that folder.
- The skill reads its own `lesson-spec.md` (the retired v3 prompt, folded in) as its detailed lesson
  specification, then inspects current lessons, helpers, and manifests to
  replace the prompt's static repository snapshot with actual source.
- Its default output is one `docs/lessons/planned/<slug>.md`, readable with `glow`,
  with full code for the learner to type, terminal architecture diagrams,
  concise run steps, and an embedded `k6 build plan`. Keep prose minimal.
  It writes executable lesson files only when explicitly requested.
  A guide alone does not register Make targets: discovery requires
  `lessons/go/<slug>/main.go`.
- Topic/slug selection checks Go lessons and saved guides, so lessons
  that have been authored but not yet typed count toward variety.
- New lessons exclude SQLite and keep the HTTP/k6 implementation as the
  later `$add-basic-k6-testing` step after the base lesson is completed.
- The k6 skill reads the matching Markdown guide's build plan. Plans specify
  the actual core API, variant comparison, workload phases, bounded fixtures,
  domain metrics, SQL output, and acceptance criteria. The plan is part of
  every new guide; a separate K6_PLAN.md is unnecessary.
- Verify a guide by extracting its code into a temporary workspace with
  the same module layout and copied shared helpers. Source and generated
  measurements stay outside the learner's lesson directories.

Verified 2026-09-28: skill-creator `quick_validate.py` passed; glow rendered
[lease-reclaim.md](../docs/lessons/planned/lease-reclaim.md) with aligned diagrams.
Extracted Go code compiled via `go test ./lessons/go/lease-reclaim/...`
(no test files), then `go run .` and `duckdb < analyze.sql` passed in the
scratch lesson directory. The three baseline trials stayed blocked; all
three leases recovered, with a 508.1 ms median and all invariants true.
The future k6 plan has not been implemented or exercised.
- **Configured software only (2026-09-28).** Lessons may use only rows marked
  `Configured: yes` in `software/software.md`; the skill reads that file and
  the spec says what to do when an idea needs something unconfigured: name
  the missing prerequisites, leave it in ideas, and do not silently substitute
  for an explicitly selected topic. Adding software to the catalog with a compose
  file is a separate, deliberate step, not part of a lesson.

## Ideas, planned, completed (2026-09-29)

`docs/lessons/README.md` defines the lifecycle; `docs/skills.md` maps tasks to
skills. Bare `$create-lesson` chooses an eligible existing idea, rechecking
`Configured: yes` in the software catalog. Ideas may list blocked prerequisites;
research in `docs/agentic-platforms/` is inspiration, not software readiness.
A supplied new topic can be recorded as an idea before planning.

A complete guide stays in planned while the learner implements the base source.
Once that work is done, move the guide to completed, record validation or the
learner's completion report with its limits, and update idea/inbound/self links.
Scratch verification alone does not complete the learner's work. Optional k6
work can follow later; its skill looks in completed first, then planned. Keep
older code-only lessons as they are without manufacturing retrospective guides.

Verified against the updated builder/spec, k6 skill, and lifecycle docs. The
lease guide moved to `docs/lessons/planned/lease-reclaim.md`; its executable
code blocks were preserved. This migration did not rerun the historical base
experiment described above.

## Idea tables (2026-09-29)

`docs/lessons/AGENTS.md` defines the backlog schema: one empty or populated
table per platform aspect, organized by abstraction rather than vendor. Columns
cover global impact Order, question/slug, comparison, invariant, measurement, software/readiness,
research, and status/plan. Structure-only requests leave all tables without
body rows. Existing plans remain valid when no corresponding idea row exists.
The initial seeded ideas were removed at the user's request; the lease plan
remains in planned. Verified by inspecting the new instructions and checking
the table structure and local links.


The initial backlog had three ideas per aspect (36 total); the Muse research
adds six ideas, bringing the current total to 42. Order is a unique,
consecutive global rank of platform-building learning impact, independent of
readiness or status; each aspect displays rows in ascending order. The builder
selects the smallest Order among eligible unplanned ideas unless directed
otherwise. The lease entry links its existing plan. Blocked runtime and ledger
ideas remain visible without changing catalog readiness. Verified row counts,
unique slugs/ranks, contiguous ranking, links, and builder validation;
these are candidate designs, not executed experiments.


## Project compositions and side quests

The bottom of `docs/lessons/ideas.md` contains five draft platforms with value,
suggested lesson sequences, architecture diagrams, and component tables. Every
node maps to existing idea slugs, Other gap IDs, or a fixture. Project build
sequences may differ from global learning-impact order. The Other table holds
12 tentative integration tasks (O01–O12), outside the lesson ranking until
explicitly promoted to lessons. Client CLI/UI work is fully AI-generated and
outside learning scope; backend contracts and authorization remain in scope.

Muse research lives in `docs/agentic-platforms/meta-muse.md`; that note separates
published architecture from local experiments and unconfigured runtime needs.
The software catalog was not changed by the research or project proposals.

Verified 2026-09-29: all 42 lesson ranks are unique/contiguous; five project
sources render into fresh embeds; final diagrams were visually inspected at
70 and 80 columns with widths 54, 50, 43, 61 and 43. See
[terminal diagrams](terminal-diagrams.md) for repository-local tooling and limits.

## Go-only local baseline (2026-09-29)

Lesson cores, runners, HTTP adapters and project code are Go only. Deno and
TypeScript remain for k6 tooling. The canonical builder/spec and k6 skill now
follow this boundary. Docker runs backing services; containerd/nerdctl is
allowed for a specific lesson benefit after local integration is configured.
KVM and separate VM paths are excluded. See [mac-portability.md](mac-portability.md)
for replacement ideas and remaining runtime/client readiness blockers.
