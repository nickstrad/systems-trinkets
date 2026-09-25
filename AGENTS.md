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

## Credentials are not secret here

Every service runs locally with fixed dev credentials (see `services/index.md`).
Whenever you mention how to connect — in chat, docs, code, or error explanations —
write the full connection string with username and password, e.g.
`postgres://trinkets:trinkets@localhost:5432/trinkets`. Never redact or
abbreviate credentials, and never replace them with placeholders.

## Start here

1. Read `knowledge/index.md` first. `knowledge/` holds tips from earlier work:
   where things live, gotchas, and verified commands. Follow `knowledge/AGENTS.md`
   and keep the index updated when you learn something reusable.
2. Backing services live in `services/` (`services/index.md`); run them with
   `make up-<service>` / `down-` / `clean-` from the repo root. Lessons run
   with `make run-<lesson>` / `analyze-` / `lab-`; the repo is one Go module
   with shared helpers in `internal/lab` (see `knowledge/go-modules.md`).
3. For non-trivial work, use the `trinkets-work-log` skill: keep an event log in
   `.state/` (gitignored) so context can be cleared, then move lasting lessons
   into `knowledge/` and delete the log.

## Before you commit or finish

Reflect before every commit and every final summary of a task: did this work
find a gotcha, verify a command or behavior, or prove an earlier claim wrong
(yours, a note's, or the code's)? Examples: a pragma that applies per
connection, a driver behavior you tested, a measurement that was skewed.

- **Yes:** run the `update-trinkets-knowledge` skill first, and put the
  `knowledge/` change in the same commit as the work.
- **No:** say so in one line in your summary ("knowledge: nothing new").

Do this even when the task was a review, a cleanup (`/simplify`), or a commit
request; those surface lessons too.

Skills live in `.claude/skills/`; `.agents/skills/` holds symlinks for Codex.
Instructions live in `AGENTS.md`; each `CLAUDE.md` only imports it
(`@AGENTS.md`). Edit `AGENTS.md`. See `knowledge/agent-instructions.md`.
