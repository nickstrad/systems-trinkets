# systems-trinkets

Small systems-engineering lessons in Go, backed by PostgreSQL, Valkey,
Redis, SeaweedFS (S3), and DuckDB. See `README.md` for the overview.

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

Every service runs locally with fixed dev credentials (see `software/software.md`).
Whenever you mention how to connect — in chat, docs, code, or error explanations —
write the full connection string with username and password, e.g.
`postgres://trinkets:trinkets@localhost:5432/trinkets`. Never redact or
abbreviate credentials, and never replace them with placeholders.

## Start here

1. Read `knowledge/index.md` first. `knowledge/` holds tips from earlier work:
   where things live, gotchas, and verified commands. Follow `knowledge/AGENTS.md`
   and keep the index updated when you learn something reusable.
2. Backing services live in `software/` (`software/software.md`); run them with
   `make up-<service>` / `down-` / `clean-` from the repo root. Lessons run
   with `make run-<lesson>` / `analyze-` / `lab-`; the repo is one Go module
   with shared helpers in `internal/lab` (see `knowledge/go-modules.md`).
   For a Redis-protocol store, choose Valkey or Redis by which feature set suits
   the lesson and default to Redis when it does not matter (see
   `software/software.md`); existing Valkey lessons stay on Valkey.
3. For non-trivial work, use the `trinkets-work-log` skill: keep an event log in
   `.state/` (gitignored) so context can be cleared, then move lasting lessons
   into `knowledge/` and delete the log.

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

## Lesson lifecycle

Lesson ideas live in `docs/lessons/ideas.md`, grouped by platform aspect with
no ranking; `docs/lessons/projects.md` groups them into potential platforms
and names the integration work between them. `$create-lesson` takes an
existing idea and writes a complete working guide in `docs/lessons/planned/`,
using only rows marked `Configured: yes` in `software/software.md`. Leave the
base source for the learner unless explicitly asked to implement it.
When the base work is complete, move its guide to `docs/lessons/completed/`
and update the idea status and links. Follow `docs/lessons/README.md` for
completion evidence; optional k6 work can come later. Platform research in
`docs/agentic-platforms/` informs ideas and future software suggestions, but
does not make unconfigured dependencies ready. See `docs/skills.md`.

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
