# systems-trinkets

Notes on building the same systems patterns — queues, leases, rate limiters,
event logs — out of Valkey, SQLite, and PostgreSQL primitives, kept in a small
SQLite database so the notes stay as portable as the subject matter.

- [docs/systems-patterns.md](docs/systems-patterns.md) — the guide: the core map
  of pattern to primitives, and the questions to ask of each one.
- [cli/docs/trinkets-cli.md](cli/docs/trinkets-cli.md) — the `trinkets` CLI and its
  schema.
- [cli/](cli/README.md) — the supporting metadata tool, its source and tests.
- [examples/counter/](examples/counter/README.md) — standalone Go counter reference on four engines.
- [harness/](harness/README.md) — HTTP invariant tests for pattern implementations.

The active curriculum is an ordered catalog of 29 exercises, from atomic
counters through partition rebalancing. `cli/seed_catalog.go` is the executable
source of truth; the guide mirrors its contracts and Valkey/SQLite/PostgreSQL
core sketches. The current harness tests counter concurrency and crash/restart behavior.
Operation-history checking is planned; independently failing multi-node support
is deferred.

The metadata tool, database, and CLI documentation are colocated in `cli/`.
See [cli/README.md](cli/README.md) for its layout and full usage.

When adding code or notes, follow the
[readability conventions](knowledge/readability-conventions.md) for names,
folder placement, and reading guides.

```sh
cd cli
go build -o bin/trinkets .
./bin/trinkets matrix
./bin/trinkets pattern show fifo-queue
```
