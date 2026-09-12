# systems-trinkets

Notes on building the same systems patterns — queues, leases, rate limiters,
event logs — out of Valkey, SQLite, and PostgreSQL primitives, kept in a small
SQLite database so the notes stay as portable as the subject matter.

- [docs/systems-patterns.md](docs/systems-patterns.md) — the guide: the core map
  of pattern to primitives, and the questions to ask of each one.
- [docs/trinkets-cli.md](docs/trinkets-cli.md) — the `trinkets` CLI and its
  schema.

The active curriculum is an ordered catalog of 29 exercises, from atomic
counters through partition rebalancing. `seed_catalog.go` is the executable
source of truth; the guide mirrors its contracts and Valkey/SQLite/PostgreSQL
core sketches. The current harness has the counter concurrency suite; process
restart and operation-history capabilities are planned, while independently
failing multi-node support is deferred.

```sh
go build -o trinkets .
./trinkets seed      # load the patterns and engines from the guide
./trinkets matrix    # the core map, as stored
./trinkets pattern show fifo-queue
```

Use `./trinkets seed --update` to refresh seed-owned fields. The explicit
catalog replacement is `./trinkets seed --update --prune`; preview it with
`./trinkets --db /path/to/copy.db seed --update --prune --dry-run`. Ordinary
seeding does not remove retired rows or rename user data.
