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

## Idea entries and projects (2026-09-29)

`docs/lessons/AGENTS.md` defines the backlog layout: `docs/lessons/ideas.md`
holds one `###` entry per idea (slug heading, bold question, Compare,
Invariant, Measure, Software, Options, optional Builds on and Plan lines)
under one `##` section per platform aspect, organized by abstraction rather
than vendor. There is no ranking or Order column; earlier versions carried a
global 1..42 impact rank and per-aspect tables, removed on 2026-09-29 because
the tables were hard to read on small screens and the rank added upkeep
without guiding selection. The Options line lists combinations of configured
software that could implement the idea and names any catalog row that still
needs setup. Structure-only requests leave sections with their description and
Research line and no entries. Existing plans remain valid when no matching
entry exists. The current backlog has 47 ideas: 42 from the research passes
plus five added from configured-but-unused catalog rows (`idle-sandbox-reaper`,
`partitioned-owner-lease`, `pooled-connection-modes`, `image-layer-pull-cost`,
`wal-change-feed`).

Readiness corrections made the same day: Compose alone expresses shared or
private PID namespaces (`pid: "service:<name>"`) and a fixture process tree,
so `container-namespace-boundary` and `exec-cancel-reap` do not need the
Docker Engine API; the usage ledger is lesson tables on the configured
PostgreSQL, so its catalog row is `yes`; `surrogate-credential-broker` is ready
with OpenBao and a plain Go worker, with only the container worker blocked.

`docs/lessons/projects.md` holds five draft platforms. Each walks from the
architecture diagram to Components (box, role, idea slugs, work chunk), Lessons
needed in build order, Follow-up lessons, Work to bring it together (named
chunks with Done-when conditions) and an Exit criterion. A Shared prerequisites
section lists the configuration chunks blocked ideas point at (Go Docker client
and isolation harness, Go S3 helper, Unix peer-identity harness, Unix-socket
egress broker). The former Other table of O01–O12 side quests was folded into
those per-project work chunks. The file marks the Durable automation service as
"Start here" because all of its lessons are ready; `$create-lesson` prefers its
Lessons needed when no idea is selected. Client CLI/UI work stays fully
AI-generated and outside learning scope. Check cross-references after editing:
every slug in projects.md must be a `###` heading in ideas.md, and every idea
should appear in at least one project.

Muse research lives in `docs/agentic-platforms/meta-muse.md`; that note separates
published architecture from local experiments and unconfigured runtime needs.

Verified 2026-09-29: the five diagram embeds pass
`render.py --check docs/lessons/projects.md` at default and 70-column widths
after moving from ideas.md (marker paths are relative to the Markdown file, so
a move within `docs/lessons/` needs no source change). See
[terminal diagrams](terminal-diagrams.md).

## Go-only local baseline (2026-09-29)

Lesson cores, runners, HTTP adapters and project code are Go only. Deno and
TypeScript remain for k6 tooling. The canonical builder/spec and k6 skill now
follow this boundary. Docker runs backing services; containerd/nerdctl is
allowed for a specific lesson benefit after local integration is configured.
KVM and separate VM paths are excluded. See [mac-portability.md](mac-portability.md)
for replacement ideas and remaining runtime/client readiness blockers.
