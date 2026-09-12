# Repository architecture

## Two Go modules

The repository has two intentionally separate modules:

- The root module is `systems-trinkets` (`go.mod`). Its package is `main` and
  builds the `trinkets` CLI. The root dependency is SQLite through
  `modernc.org/sqlite`.
- `harness/` is the module `systems-trinkets/harness` (`harness/go.mod`). It
  owns the HTTP test toolkit and DuckDB/Parquet result path. Keeping its module
  boundary prevents harness dependencies from becoming root CLI dependencies.

Run Go commands from the module being changed. The harness's imports such as
`systems-trinkets/harness/results` are not root-package imports.

## Root CLI flow

`main.go` parses global `--db`, resolves the database from the flag, then
`TRINKETS_DB`, then `./trinkets.db`, and dispatches the resource command. The
`help` and `version` paths return before opening SQLite. The seed dry-run path copies a read-only source through the SQLite backup API
and migrates only that temporary copy. Other database commands call
`openDB` first, so schema creation and migrations happen on first use before a
command handler runs.

The source ownership is deliberately plain:

- `db.go`: current SQLite DDL, connection pragmas, and idempotent migrations.
- `store.go`: row types, JSON/list conversion, parameterized queries, partial
  update builder, and matrix assembly.
- `cmd_pattern.go`, `cmd_engine.go`, `cmd_approach.go`, and `cmd_attempt.go`:
  flag parsing and CRUD behavior for the four resources.
- `cmd_matrix.go`: the pattern × engine view.
- `seed_catalog.go`: the ordered in-code curriculum with three core-map
  approaches per pattern; `docs/systems-patterns.md` is its readable guide.
- `seed.go` and `reconcile.go`: transactional seed upserts, explicit legacy
  identity reconciliation, conflict reports, and read-only-source previews.
- `output.go`: custom flags, JSON output, and tabular formatting.

The conceptual relationship is:

```text
patterns ──< approaches >── engines
    └──────< attempts >──────┘
                         (optional approach link)
```

`pattern show` includes its approaches and attempts; `engine show` includes
its approaches. `matrix` loads all rows and reports the first non-empty
primitive string it sees for each pattern/engine cell, plus approach and
attempt counts.

## What is implemented vs planned

Implemented in the current source are the four-table SQLite store, migrations,
CRUD handlers, `matrix`, transactional `seed`, JSON output for list/show
commands, and the root docs under `docs/`. The root `README.md` is the short
on-ramp; `docs/trinkets-cli.md` describes the CLI surface.

The harness has working package code for HTTP, load shapes, checks, result
collection, target loading, a `run`/`report`/`sql`/`new-suite`/`targets` CLI,
query templates, and a counter suite. `harness/test-plan.md` is still the
design authority for work beyond that current surface; its phase-2/3 items
(for example FIFO/crash suites, process lifecycle, open-loop load, and
linearizability/history reporting) are plans, not current guarantees.
