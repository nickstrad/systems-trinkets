# knowledge/

Helpful tips for future agents: how to move around this codebase, gotchas already
hit, and decisions already made. Read it before digging in so you do not repeat
an investigation. Update it as work is done.

## Start here

1. Read `index.md` first. It lists every item in this folder and what it covers.
   Open only the items relevant to your task.
2. When your work teaches something reusable (a non-obvious fix, a pitfall, a
   verified command, where something lives), add or update an item here.
3. Always update `index.md` in the same change. An item missing from the index
   is invisible to the next agent.

## Item format

- **Single file:** `knowledge/<topic>.md` (kebab-case) when prose is enough.
- **Folder:** `knowledge/<topic>/` when the item needs artifacts (scripts,
  docs, SQLite files, sample data, diagrams). The folder must have a `README.md`
  that explains the concept and lists each artifact and what it is for.

Each item should say when it applies, the relevant source paths, what to do,
and how it was verified. Keep items short and factual; mark anything unverified.
Fix or delete items that become wrong. Use the `update-trinkets-knowledge` skill
for the detailed procedure.
