# Working in systems-trinkets

Read `knowledge/index.md` first when planning work or researching this codebase.
Follow its links for the relevant subsystem, then verify details against current
source. Read `harness/docs/architecture.md` before changing the harness; distinguish
implemented behavior from `harness/docs/backlog.md`. Historical decisions and
verification are in `harness/docs/work-log.md`.

Optimize names and folder structure for a human encountering the project. Follow
[readability conventions](knowledge/readability-conventions.md) when adding,
moving, or renaming code, docs, and artifacts. Use descriptive domain names,
group related material by responsibility, and give substantial components a
README with a reading order. Update navigation and references in the
same change. Apply these conventions to the work at hand; avoid unrelated
reorganizations.

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

The user builds correct pattern implementations. Keep harness mechanics,
deliberate faults, and bug-only flags/interfaces under `harness/`; examples
and user projects contain correct application logic and ordinary unit tests.

When database schema, migrations, relationships, or column semantics change,
read `cli/docs/schema/AGENTS.md` and update both `cli/docs/schema/index.html` and
`cli/docs/schema/erd.html` in the same change. Keep applicable knowledge current too.

The metadata CLI in `cli/`, test toolkit in `harness/`, and counter reference
in `examples/counter/` are separate Go modules. From the repository root, run
`go -C cli test ./...`, `go -C harness test ./...`, and
`go -C examples/counter test ./...`; there is no root Go module. Use a temporary database via `--db` for CLI experiments:
the repository's `cli/trinkets.db` is shared data tracked intentionally.

`AGENTS.md` is a relative symlink to this file. Edit this file to maintain both
agent entrypoints together.
