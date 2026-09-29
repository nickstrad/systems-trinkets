# Lesson skills

| Task | Skill | Result |
|---|---|---|
| Turn an existing idea into a working guide | [`$create-lesson`](../.claude/skills/create-lesson/SKILL.md) | `docs/lessons/planned/<slug>.md`, linked from ideas.md |
| Add concurrent traffic to a finished base lesson | [`$add-basic-k6-testing`](../.claude/skills/add-basic-k6-testing/SKILL.md) | HTTP adapter, k6 workload, and DuckDB analysis under the lesson's `perf/` |
| Draw terminal architecture diagrams | [`draw-visual`](../.claude/skills/draw-visual/SKILL.md) | Mermaid sources, regenerable Unicode embeds, and terminal-width previews |
| Track a substantial task across sessions | [`trinkets-work-log`](../.claude/skills/trinkets-work-log/SKILL.md) | Temporary append-only `.state/` log |
| Preserve a verified repository finding | [`update-trinkets-knowledge`](../.claude/skills/update-trinkets-knowledge/SKILL.md) | Focused knowledge note and index entry |

Example requests:

```text
Update docs/lessons/ideas.md with projects inspired by docs/agentic-platforms/.
$create-lesson from the outbox-redelivery idea in docs/lessons/ideas.md
I've finished lease-reclaim; review the completion evidence and move its plan to completed.
$add-basic-k6-testing lessons/go/lease-reclaim
```

The [lesson lifecycle](lessons/README.md) defines planning and completion.
The lesson builder writes Go-only guides; Deno/TypeScript remains for k6 tooling.
It uses only software marked `Configured: yes` in
[the catalog](../software/software.md), and leaves the base code for the learner
to type. A completed guide stays in completed even while k6 work is pending.
Research and backlog updates may propose future software, but do not configure it.

Edit canonical skills in `.claude/skills/`; `.agents/skills/` exposes them to
Codex through symlinks.

The repository includes draw-visual's sources and helpers for both Claude and
Codex. On a new device, run
`bash .claude/skills/draw-visual/scripts/setup.sh`; see its skill for prerequisites
and font selection. Generated renderer binaries and review PNGs are not vendored.
