# Agent instruction files (AGENTS.md / CLAUDE.md)

Applies when adding or editing agent instructions for any folder in this repo.

- **Layout.** In each folder with instructions (repo root, `knowledge/`,
  `services/`), `AGENTS.md` holds the real text and `CLAUDE.md` contains only
  `@AGENTS.md`. Codex and other tools read `AGENTS.md`; Claude Code reads
  `CLAUDE.md` and pulls `AGENTS.md` in through the import. Edit `AGENTS.md`;
  never put content in `CLAUDE.md` unless it is Claude-only.
- **Adding a folder's instructions:** write `<dir>/AGENTS.md`, then
  `printf '@AGENTS.md\n' > <dir>/CLAUDE.md`. Claude loads nested ones when it
  reads a file in that folder.
- **Why not a symlink:** Claude Code's Edit/Write tools refuse to write through
  symlinks, and symlinks break on Windows checkouts. The import is the pattern
  the Claude Code docs recommend.
- **Why keep `CLAUDE.md` at all:** since v2.1.277 Claude Code reads `AGENTS.md`
  natively, but by default only when no `CLAUDE.md` or `CLAUDE.local.md` exists
  in the working directory or above. Relying on that fallback breaks as soon as
  someone adds a `CLAUDE.local.md`. The import works in every mode, so no
  `/config` "Project instructions" setting is needed.

Source: https://code.claude.com/docs/en/memory.md (AGENTS.md sections).
Verified 2026-09-24 on Claude Code 2.1.282 by reading the docs; the switch from
symlinks to imports was made the same day.
