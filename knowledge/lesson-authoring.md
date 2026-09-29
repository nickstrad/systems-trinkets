# Creating a lesson

Use `$create-lesson` to create a new lesson guide on demand. Optional
arguments can specify the topic, language, store, or output path.

- The canonical skill is `.claude/skills/create-lesson/SKILL.md`.
  `.agents/skills/create-lesson` is a relative symlink to that folder.
- The skill reads its own `lesson-spec.md` (the retired v3 prompt, folded in) as its detailed lesson
  specification, then inspects current lessons, helpers, and manifests to
  replace the prompt's static repository snapshot with actual source.
- Its default output is one `docs/lessons/<slug>.md`, readable with `glow`,
  with full code for the learner to type, terminal architecture diagrams,
  concise run steps, and an embedded `k6 build plan`. Keep prose minimal.
  It writes executable lesson files only when explicitly requested.
  A guide alone does not register Make targets: discovery requires
  `lessons/go/<slug>/main.go` or `lessons/deno/<slug>/main.ts`.
- Topic/slug selection checks both runtimes and saved guides, so lessons
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
[lease-reclaim.md](../docs/lessons/lease-reclaim.md) with aligned diagrams.
Extracted Go code compiled via `go test ./lessons/go/lease-reclaim/...`
(no test files), then `go run .` and `duckdb < analyze.sql` passed in the
scratch lesson directory. The three baseline trials stayed blocked; all
three leases recovered, with a 508.1 ms median and all invariants true.
The future k6 plan has not been implemented or exercised.
- **Configured software only (2026-09-28).** Lessons may use only rows marked
  `Configured: yes` in `software/software.md`; the skill reads that file and
  the spec says what to do when a topic needs something unconfigured (name
  the row, pick another idea). Adding software to the catalog with a compose
  file is a separate, deliberate step, not part of a lesson.
