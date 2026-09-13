# CLI, schema, and store pitfalls

Read [db.go](../cli/db.go), [store.go](../cli/store.go), and the command handlers
in `cli/` before changing persistence behavior. Unqualified source paths below
are relative to that module. These are the invariants most likely to
be accidentally weakened by a seemingly small edit.

## Schema and connection behavior

`openDB` uses a SQLite DSN with a 5-second busy timeout, foreign keys enabled,
and WAL journal mode, then sets `MaxOpenConns(1)`. This is a one-process,
short-lived CLI design: do not assume the same connection settings if adding a
new database entry point.

The current tables are:

- `patterns`: unique `slug`, descriptive text, JSON-array `invariants` and
  `readings`, nullable positive `curriculum_order`, and timestamps.
- `engines`: unique `slug`, notes, and timestamps.
- `approaches`: pattern/engine foreign keys, unique `(pattern_id, engine_id,
  title)`, primitives, writeup, and timestamps.
- `attempts`: pattern/engine foreign keys, optional `approach_id`, a checked
  status (`planned`, `in_progress`, `done`, `abandoned`), lessons, and
  timestamps. Its composite foreign key requires a cited approach to belong to
  the same pattern and engine.

The JSON columns are text with SQLite `CHECK (json_type(...) = 'array')`.
`jsonText(nil)` deliberately emits `[]`, not JSON `null`, so writes continue to
satisfy that check.

## Migrations are real code

`schemaSQL` is idempotent, but `CREATE TABLE IF NOT EXISTS` does not add new
columns to an existing database. `openDB` therefore runs `migrations` after
the DDL. Current migrations add newer text/list columns, convert a legacy
line-based `invariant` column to the `invariants` JSON array, convert old
line-based readings, and drop the legacy invariant column.

Migration functions are not wrapped in one shared transaction by `openDB`.
The curriculum-order migration wraps its column addition and partial unique
index creation in its own transaction. The index must be created after the
column exists, because legacy databases reach migrations only after initial
DDL succeeds. Existing order column/index definitions are validated; incompatible
definitions fail instead of silently weakening the constraint. The column guard
compares actual SQL tokens, ignoring comments and keeping string literals
separate, because table-info pragmas do not expose CHECK constraints.
`rewriteColumn` also reads all rows and updates them one by one. A future
migration that can fail after a partial write should add its own transaction
and be tested against a copy of an older database; do not infer atomic upgrade
behavior from the idempotent DDL comment.

## Store and editing semantics

The `fields` builder only adds flags that were supplied, and `update` always
adds `updated_at`. This is what makes edits non-destructive to omitted fields.
An edit with no field flags returns `nothing to update`; preserve that guard.

`--invariant` and `--reading` are repeatable. On edit, supplying either flag
replaces the entire list, rather than appending. `parseReading` splits on the
last `" - "` (so a title may contain dashes) and accepts a bare `http://` or
`https://` URL as both title and URL. Preserve this format when adding CLI
helpers.

Pattern and engine identifiers are slugs; approach and attempt identifiers are
numeric IDs. The command-layer `verb`/`hoistPositional` handling exists because
Go's `flag` parser stops at the first positional argument: `edit
fifo-queue --notes ...` must be rearranged before parsing.

Pattern add/edit accepts one positive `--curriculum-order` or the explicit
`--clear-curriculum-order`; supplying both is rejected. An omitted order flag
is not included in the update, so partial edits preserve the current nullable
order. Pattern list, show, and matrix text views render an unassigned order as
`-`; their JSON views use `Pattern.CurriculumOrder` (and the matrix row's
matching pointer) so the value is a JSON number or `null`.

## Seeding and deletes

`seed` performs all engine, pattern, and approach upserts inside one
transaction. Without `--update`, existing rows are left alone. With
`--update`, doc-derived fields and approach `primitives` are refreshed, while
the approach `writeup` is deliberately preserved. Do not make seed overwrite
that prose unless the ownership model changes.

The executable curriculum data lives in `seed_catalog.go`; `seed.go` owns the
transactional upserts. `seed --update --prune` is the explicit catalog
reconciliation in `reconcile.go`. It uses seven named renames and nine explicit
retirements, preserving surviving pattern/approach IDs and creation timestamps.
It rejects destination collisions, unknown patterns/engines, and notes,
writeups, non-core approaches, or attempts on renamed/retired rows. Retained
patterns keep authored approaches and history; a report lists those extra
approaches rather than claiming exactly 87 total rows. Ordinary seed/update
never rename or delete legacy rows.

`seed --update --prune --dry-run` is routed before `openDB` in `main.go`.
It opens the source read-only and uses the driver's SQLite backup API to a
temporary database, then migrates and plans only there. Copying the main file
alone would miss committed WAL data. Apply rebuilds its conflict report inside
the write transaction, checks canonical content and foreign keys, then commits.
Schema upgrade is separate and can remain after content rollback.

Order refresh clears only managed positions that differ, within the same
transaction, before assigning new values; a custom row occupying a required
position causes rollback. Conditional upserts preserve `updated_at` when
seed-owned fields are unchanged, so a second reconciliation changes no rows.
Pattern lists and the final matrix sort place ordered rows first, then NULL
custom rows by family/slug. `Pattern.CurriculumOrder` is `*int`, exposing a JSON
number or null rather than a nullable-wrapper object.

`go -C cli test ./...` from the repository root covers fresh/legacy migration, negative SQL constraints,
identity/history preservation, injected rollback after rename/upsert/delete,
repeat-run write rejection, source-byte-preserving dry runs, and backup of a
live committed WAL. These checks create temporary databases directly in test code; they do not
modify the shared database. Reconciliation tests use minimal synthetic rows
for retained, renamed, and retired patterns instead of a historical catalog
dump. Migration tests define the old table shape explicitly, independently
of production DDL, so the missing-column condition cannot disappear when
the current schema changes.

Pattern/engine deletion pre-counts dependent approaches and attempts and
requires `--force` when dependents exist; SQLite cascades the dependent rows
when force is used. Approach deletion is different: it explicitly unlinks
referencing attempts in a transaction, then deletes the approach, preserving
the historical attempt. The composite attempt foreign key is why an invalid
`--approach` is annotated with a more useful message by the command handler.
