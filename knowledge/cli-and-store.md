# CLI, schema, and store pitfalls

Read [db.go](../db.go), [store.go](../store.go), and the command handlers
before changing persistence behavior. These are the invariants most likely to
be accidentally weakened by a seemingly small edit.

## Schema and connection behavior

`openDB` uses a SQLite DSN with a 5-second busy timeout, foreign keys enabled,
and WAL journal mode, then sets `MaxOpenConns(1)`. This is a one-process,
short-lived CLI design: do not assume the same connection settings if adding a
new database entry point.

The current tables are:

- `patterns`: unique `slug`, descriptive text, JSON-array `invariants` and
  `readings`, and timestamps.
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

Migration functions are not wrapped in one transaction by `openDB`.
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

## Seeding and deletes

`seed` performs all engine, pattern, and approach upserts inside one
transaction. Without `--update`, existing rows are left alone. With
`--update`, doc-derived fields and approach `primitives` are refreshed, while
the approach `writeup` is deliberately preserved. Do not make seed overwrite
that prose unless the ownership model changes.

Pattern/engine deletion pre-counts dependent approaches and attempts and
requires `--force` when dependents exist; SQLite cascades the dependent rows
when force is used. Approach deletion is different: it explicitly unlinks
referencing attempts in a transaction, then deletes the approach, preserving
the historical attempt. The composite attempt foreign key is why an invalid
`--approach` is annotated with a more useful message by the command handler.
