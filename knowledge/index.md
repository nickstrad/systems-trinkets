# Agent knowledge

This directory is a short, source-grounded orientation to the repository. It
is intentionally narrower than the product documentation: use the code and
the linked design documents as the authority when they disagree.

## Writeups

- [Readability conventions](readability-conventions.md) — general naming and folder hygiene,
  reading guides, and artifact placement for human-readable projects.
- [Repository architecture](architecture.md) — CLI, harness, and counter reference Go modules, startup flow,
  user implementation versus harness ownership, and the implemented/planned boundary.
- [CLI, schema, and store pitfalls](cli-and-store.md) — SQLite constraints,
  migrations, curriculum reconciliation and read-only previews, JSON list
  fields, CRUD semantics, deletion/foreign-key behavior, and self-contained test setup.
- [HTTP invariant harness](harness.md) — the separate Go module: study layout and package
  contracts, source/output separation, results pipeline, harness-managed SUT lifecycle and crash tests,
  the counter reference on four engines, batch runs, signal/export races, and
  what is still backlog.

The schema companion pages are maintained separately:
[schema index](../cli/docs/schema/index.html) and [schema ERD](../cli/docs/schema/erd.html).

## Maintaining this index

When adding a knowledge writeup, add one concise entry here with its purpose.
Keep the list complete: each writeup `.md` file in `knowledge/` (with this
index itself as the obvious exception) must appear above, and each listed link
must remain a relative link that resolves from this file.
When supporting artifacts justify a topic folder, put the writeup and artifact
map in its `README.md`, then list the folder here with a link to that README
and its purpose. Use `update-trinkets-knowledge` while working to capture
verified reusable lessons and maintain this inventory.
Prefer updating an existing focused writeup over adding an inventory page.
Document behavior only after checking the current source; label design-plan or
README-only behavior as planned rather than implemented. Recheck this index
after moving or deleting a writeup.
