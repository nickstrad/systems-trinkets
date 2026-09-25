---
name: update-trinkets-knowledge
description: This skill should be used when asked to "update-trinkets-knowledge", "record this lesson", or "update the knowledge store", and during systems-trinkets work after solving a non-obvious problem, verifying a reusable discovery, or changing behavior documented in knowledge/.
---

# Update trinkets knowledge

Maintain the repository's `knowledge/` store so later agents can reuse verified
lessons without repeating the investigation. Resolve all paths below from the
repository root, not from the skill directory.

1. Read `knowledge/index.md` and the relevant linked entries. Check current
   source and any applicable local agent guidance before treating an old note
   as authoritative.
2. Identify the reusable finding from the current work: the observable symptom
   or question, its cause, the working resolution or decision, and the evidence
   that supports it. Distinguish observed results from hypotheses and planned
   behavior. If nothing reusable was learned and existing notes remain accurate,
   leave the store unchanged.
3. Update an existing focused entry when possible. Otherwise create a descriptive
   kebab-case `.md` file under `knowledge/`. Explain when the lesson applies,
   the source paths involved, how to resolve or avoid the problem, and relevant
   verification commands with their limits. Keep the account generalized and
   concise; omit session transcripts, personal paths, secrets, database records,
   and unneeded raw logs. Do not claim a command passed unless it was run.
4. Create `knowledge/<topic>/` only when a writeup needs supporting artifacts
   such as a minimal reproduction or diagram. Put the explanation and artifact
   map in its `README.md`; keep artifacts minimal and reviewable. Link existing
   repo documentation rather than copying it.
5. Update `knowledge/index.md` in the same change. List every top-level writeup
   other than the index itself, or each topic folder linked to its `README.md`,
   with a concise purpose. Keep each folder's README artifact map current.
   Correct or remove stale claims and broken links affected by the work.
6. Verify factual claims against source and check relative links and index
   coverage. Run a safe focused reproduction when needed; use a temporary
   database or scratch directory for experiments. Report what knowledge changed and what evidence
   supports it in the task handoff.

Keep updates within the active task's scope. This skill does not itself authorize
committing, pushing, or changing application behavior. Edit the canonical skill
at `.claude/skills/update-trinkets-knowledge/SKILL.md`; the Codex entrypoint at
`.agents/skills/update-trinkets-knowledge` is a relative symlink to that directory.
