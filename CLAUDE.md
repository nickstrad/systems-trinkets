# systems-trinkets

Small systems-engineering lessons in Go or Deno, backed by PostgreSQL, Valkey,
SeaweedFS (S3), and DuckDB. See `README.md` for the overview.

## This repo is for learning

The user is building systems-engineering skills here. When you find an error
(a failing test, a bug, a crash, a wrong result, a misconfiguration) in the
user's code or lessons:

1. **Explain it, don't fix it.** Say what fails, where (`file:line`), the cause,
   and the concept behind it. Point toward how to fix it without writing the fix.
2. **Fix only when told to.** Change the code only after you have stated the issue
   and the user explicitly asks you to fix it. Approval to fix one issue does not
   cover the next one.

## Start here

1. Read `knowledge/index.md` first. `knowledge/` holds tips from earlier work:
   where things live, gotchas, and verified commands. Follow `knowledge/AGENTS.md`
   and keep the index updated when you learn something reusable.
2. Backing services live in `services/` (`services/index.md`); run them with
   `make up-<service>` / `down-` / `clean-` from the repo root.
3. For non-trivial work, use the `trinkets-work-log` skill: keep an event log in
   `.state/` (gitignored) so context can be cleared, then move lasting lessons
   into `knowledge/` and delete the log.

Skills live in `.claude/skills/`; `.agents/skills/` holds symlinks for Codex.
