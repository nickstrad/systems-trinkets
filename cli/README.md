# Metadata CLI

`trinkets` is the supporting tool for browsing and editing the repository's
patterns, engines, approaches, and attempts. The systems exercises and HTTP
testing toolkit live outside this module, in the repository docs and `harness/`.

## Build and use

From `cli/`:

```sh
go build -o bin/trinkets .
./bin/trinkets matrix
./bin/trinkets pattern show counter
go test ./...
```

The database defaults to `./trinkets.db` relative to your working directory.
Run from `cli/` to use the shared database, for example `go run . matrix`.
From the repository root, use `./cli/bin/trinkets --db cli/trinkets.db matrix`.
For experiments,
pass `--db` with a temporary database path. `TRINKETS_DB` can also set the path.

See the [command reference](docs/trinkets-cli.md),
[pattern guide](../docs/systems-patterns.md), and
[schema reference](docs/schema/index.html).

## Layout

This small executable uses one `package main`, organized by concern:

| Files | Responsibility |
| --- | --- |
| `main.go` | Startup, database selection, command dispatch |
| `cmd_*.go` | Pattern, engine, approach, attempt, and matrix commands |
| `output.go` | Flag and display helpers |
| `db.go`, `store.go` | SQLite schema/migrations, domain types, data access |
| `seed_catalog.go` | Built-in curriculum content |
| `seed.go`, `reconcile.go` | Load, refresh, reconcile, and preview the catalog |
| `*_test.go` | Tests with temporary databases |
| `go.mod`, `go.sum` | This module's dependencies |
| `bin/` | Ignored build output |
| `docs/` | CLI reference and schema diagrams |
| `trinkets.db` | Shared metadata database |

`trinkets.db` is the shared metadata database, tracked here in `cli/`.
SQLite's `trinkets.db-wal` and `trinkets.db-shm` sidecars also live here and
are ignored by Git.
Harness results are managed separately by `harness/`.
