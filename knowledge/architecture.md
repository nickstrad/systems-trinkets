# Repository architecture

## Two Go modules

The repository has two intentionally separate modules:

- `cli/` is the module `systems-trinkets/cli` (`cli/go.mod`). Its package is
  `main` and builds the `trinkets` metadata CLI. Its database dependency is SQLite through
  `modernc.org/sqlite`.
- `harness/` is the module `systems-trinkets/harness` (`harness/go.mod`). It
  owns the HTTP test toolkit and DuckDB/Parquet result path. Keeping its module
  boundary prevents harness dependencies from becoming CLI dependencies.

There is no root Go module. From the repository root, use
`go -C cli test ./...` and `go -C harness test ./...` to check each module
separately. Build the metadata tool with `go -C cli build -o bin/trinkets .`
and run `./cli/bin/trinkets --db cli/trinkets.db matrix` from the repository
root. The shared database and SQLite sidecars live in `cli/`; CLI docs live
in `cli/docs/`. Database defaults remain relative to the working directory,
not the binary: from `cli/`, `./bin/trinkets matrix` uses the shared database.
Tests live alongside the source in `cli/` and create their own temporary
databases; no catalog dump or `testdata/` directory is required.

## Metadata CLI flow

`main.go` parses global `--db`, resolves the database from the flag, then
`TRINKETS_DB`, then `./trinkets.db`, and dispatches the resource command. The
`help` and `version` paths return before opening SQLite. The seed dry-run path copies a read-only source through the SQLite backup API
and migrates only that temporary copy. Other database commands call
`openDB` first, so schema creation and migrations happen on first use before a
command handler runs.

All CLI source paths below are relative to `cli/`. The files share one
`package main`; command, storage, and seed concerns remain separated by file.
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
on-ramp; `cli/docs/trinkets-cli.md` describes the CLI surface.

The harness has working package code for HTTP, load shapes, checks, result
collection, target loading, SUT process lifecycle (start/kill/restart/stop),
a `run`/`report`/`sql`/`new-suite`/`targets` CLI, query templates, and the
counter suite (including a crash test) with a Go reference implementation on
memory, SQLite, Valkey and PostgreSQL. `harness/test-plan.md` is the design
authority and work log; its §7 backlog items (open-loop load, performance
thresholds, history/linearizability checking, further pattern suites) are
plans, not current guarantees.
