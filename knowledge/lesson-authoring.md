# Creating a daily lesson

Use `$create-daily-lesson` to create a new lesson guide on demand. Optional
arguments can specify the topic, language, store, or output path.

- The canonical skill is `.claude/skills/create-daily-lesson/SKILL.md`.
  `.agents/skills/create-daily-lesson` is a relative symlink to that folder.
- The skill reads `docs/prompts/daily_lesson/v3.txt` as its detailed lesson
  specification, then inspects current lessons, helpers, and manifests to
  replace the prompt's static repository snapshot with actual source.
- Its default output is `docs/lessons/<slug>.md`, with full code for the
  learner to type. It writes executable lesson files only when requested.
  A guide alone does not register Make targets: discovery requires
  `lessons/go/<slug>/main.go` or `lessons/deno/<slug>/main.ts`.
- Topic/slug selection checks both runtimes and saved guides, so lessons
  that have been authored but not yet typed count toward variety.
- New lessons exclude SQLite and keep the HTTP/k6 implementation as the
  later `$add-basic-k6-testing` step after the base lesson is completed.

Verified 2026-09-28 by reading v3, the skill, and Makefile discovery rules.
The skill-creator `quick_validate.py` check passed. No generated lesson was
executed as part of creating this skill.
