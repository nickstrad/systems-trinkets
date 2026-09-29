# Repository-local terminal diagrams

Use `.claude/skills/draw-visual/SKILL.md` for regenerable Unicode architecture
embeds. `.agents/skills/draw-visual` is a relative symlink to that canonical
folder, so Claude and Codex share the repository copy. The source bundle includes
render/setup helpers and the Go snapshot helper's module and checksums; binaries
and preview PNGs stay outside the repo. The global skill is independent.

Project sources live in `docs/lessons/diagrams/ideas-*.mmd`, embedded in
`docs/lessons/ideas.md`. Regenerate and check from the repo root:

```sh
bash .claude/skills/draw-visual/scripts/setup.sh
python3 .claude/skills/draw-visual/scripts/render.py --update docs/lessons/ideas.md
DRAW_VISUAL_MAX_WIDTH=70 python3 .claude/skills/draw-visual/scripts/render.py --check docs/lessons/ideas.md
```

Setup builds the pinned Mermaid tool with Go; rendering needs Python 3. On a
new device initial setup needs network access. Snapshot generation needs a
monospaced TrueType font; pass `--font` when the Linux default does not exist.
The optional PlantUML JRE installer targets Linux x64. Mermaid source portability
is provided, but this task did not verify execution on macOS.

The snapshot launcher rebuilds its helper and therefore needs writable Go
caches and binary destination. In a restricted session, an already installed
compatible `diagram-snapshot` binary can generate previews directly with
`--input docs/lessons/ideas.md --out <temporary-directory> --columns 70,80`.
This is a preview of terminal geometry, not a screenshot of the user's app.

Verified 2026-09-29: skill validation, helper-source comparison with the copied
original, symlink resolution, and five fresh embeds passed. Used the existing
snapshot binary after a cache-write restriction prevented rebuilding; all ten
final PNGs were opened and reviewed at 70/80 columns, without clipping/wrapping.
Shared return lines made two initial diagrams ambiguous: separate read/write
views of the same store, explained in prose, made their paths easier to follow.
Source regeneration and width checks alone do not establish visual clarity.
