---
name: trinkets-work-log
description: This skill should be used when asked to "trinkets-work-log", "start a work log", "log this work", "resume from the state file", "pick up where we left off", or "hand the state file to subagents", and at the start of any non-trivial systems-trinkets task, when dispatching subagents, and when finishing a task. Keeps an append-only `.state/` event log so a fresh session can resume after a context clear, then folds lessons into knowledge/ and deletes the log.
---

# Trinkets work log

Keep one append-only state file per non-trivial task so any session can resume
after a context clear. Resolve all paths from the repository root. `.state/` is
gitignored; never commit its files.

## 1. At session start: resume before starting

List `.state/`. If a file matches the current task, read it fully and continue
from its last "next" entry instead of starting over. Append a `resume` entry.
Leave unrelated in-progress files alone.

## 2. Start the file

For new non-trivial work, run `mkdir -p .state` and create
`.state/<YYYY-MM-DD>-<short-task-slug>.md`. Begin it with a short header:

- **Goal:** one or two sentences.
- **Plan:** a checklist (`- [ ]` items).
- **Coordinator:** which agent/session owns the task.
- **Log:** heading below which entries are appended.

## 3. Append events as the work happens

Append a timestamped entry (`date '+%Y-%m-%d %H:%M'`) for each decision,
command run and its outcome, file changed, blocker, and next step. Write enough
that a reader with no other context can continue. Never rewrite or delete past
entries; correct them with a new entry. Tick plan items by appending a line
such as `plan: [x] item`, not by editing the header, unless you are the
coordinator and no other agent is writing.

```markdown
### 2026-09-24 14:05 [coordinator] decision
Store lesson results in Postgres; analyze exported runs with DuckDB.
### 2026-09-24 14:12 [sub:builder] command
`go test -race ./...` -> PASS (3 packages). Changed: lessons/example/main.go.
### 2026-09-24 14:20 [coordinator] next
Dispatch reviewer; blocker: none.
```

Entry format: `### <timestamp> [<author>] <kind>` where kind is one of
`decision`, `command`, `change`, `dispatch`, `result`, `blocker`, `resume`,
`next`.

## 4. Coordinate subagents

Choose one approach per dispatch and note it in a `dispatch` entry:

- **Coordinator logs:** record the dispatch (agent, brief summary) and, on
  return, a `result` entry with the outcome and files changed.
- **Subagent logs:** pass the state file path in the brief and instruct the
  subagent to append its own entries tagged `[sub:<agent-name>]`, append-only,
  never editing other entries or the header. Use `>>` or an append edit.

## 5. Finish the work

1. Reread the whole log. Pick out reusable lessons, non-obvious fixes, and
   verified decisions.
2. Add them to `knowledge/`: start at `knowledge/index.md`, follow
   `knowledge/AGENTS.md`, and use the `update-trinkets-knowledge` skill for the
   mechanics. Keep `knowledge/index.md` updated in the same change. Skip if
   nothing reusable was learned.
3. Delete only this task's state file (`rm .state/<this-file>.md`). Leave other
   in-progress state files in place.

This skill does not authorize committing or pushing. Edit the canonical skill at
`.claude/skills/trinkets-work-log/SKILL.md`; the Codex entrypoint at
`.agents/skills/trinkets-work-log` is a relative symlink to that directory.
