# Working in systems-trinkets

Read `knowledge/index.md` first when planning work or researching this codebase.
Follow its links for the relevant subsystem, then verify details against current
source. Read `harness/test-plan.md` before changing the harness; distinguish its
design from what is implemented.

Use the repository skill `update-trinkets-knowledge` while working whenever a
solved problem, verified discovery, or changed behavior yields a reusable lesson.
Codex discovers it at `.agents/skills/update-trinkets-knowledge/SKILL.md`; Claude
discovers it at `.claude/skills/update-trinkets-knowledge/SKILL.md`. Both paths
share one skill. If automatic discovery is unavailable, read that file directly.
Capture the lesson before handing off the task; do not manufacture a writeup for
routine work that produced no new knowledge.

Keep knowledge as focused `.md` writeups. Use a folder only when supporting
artifacts are needed, with a `README.md` explaining them. Update
`knowledge/index.md` whenever entries are added, renamed, removed, or change
purpose. Prefer updating an existing entry over creating a duplicate.

When database schema, migrations, relationships, or column semantics change,
read `cli/docs/schema/AGENTS.md` and update both `cli/docs/schema/index.html` and
`cli/docs/schema/erd.html` in the same change. Keep applicable knowledge current too.

The metadata CLI in `cli/` and the test toolkit in `harness/` are separate Go
modules. Run `go -C cli test ./...` and `go -C harness test ./...` from the
repository root; there is no root Go module. Use a temporary database via `--db` for CLI experiments:
the repository's `cli/trinkets.db` is shared data tracked intentionally.

`AGENTS.md` is a relative symlink to this file. Edit this file to maintain both
agent entrypoints together.
