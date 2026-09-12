# Agent knowledge

This directory is a short, source-grounded orientation to the repository. It
is intentionally narrower than the product documentation: use the code and
the linked design documents as the authority when they disagree.

## Writeups

- [Repository architecture](architecture.md) — root Go module, startup flow,
  ownership of the main packages, and the implemented/planned boundary.
- [CLI, schema, and store pitfalls](cli-and-store.md) — SQLite constraints,
  migrations, curriculum reconciliation and read-only previews, JSON list
  fields, CRUD semantics, and deletion/foreign-key behavior.
- [HTTP invariant harness](harness.md) — the separate Go module, current
  package contracts, result pipeline, target lifecycle, and the pieces still
  described only by the harness plan.

The schema companion pages are maintained separately:
[schema index](../docs/schema/index.html) and [schema ERD](../docs/schema/erd.html).

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
