# Atomic counter reference

A small HTTP service implementing atomic counters on memory, SQLite, Valkey,
or PostgreSQL. This is a standalone Go project you can read, run, and use as
a starting point for your own implementations.

## Project layout

```text
counter/
  go.mod, go.sum          independent module and database driver dependencies
  cmd/counter/main.go     flags, server startup and shutdown
  store.go               Store interface: the operations the HTTP layer needs
  handler.go             routes, validation, JSON responses, error handling
  memory.go              map + mutex implementation
  counter_test.go        memory and HTTP contract tests
  store/open.go          engine selection and default connection settings
  internal/
    store/
      sqlite/            atomic SQL upsert, WAL, connection setup
      valkey/            INCRBY and other native commands
      postgres/          atomic SQL upsert with RETURNING
    storetest/           shared behavioral tests for every engine
  bin/                   optional ignored build output
```

Start with `store.go`, then `memory.go`, then `handler.go`. Follow one request
from the HTTP handler into a store. Compare the engine adapters to see which
database primitive supplies atomicity. Read `cmd/counter/main.go` last to see
how those pieces are assembled. This project contains correct application
logic and ordinary unit tests. Harness orchestration and deliberate faults
are maintained separately under `harness/`; a user implementing a pattern
is not expected to write them.

The module root is package `counter`, a public HTTP library used by the
harness's in-process tests. Engine adapters stay in `internal/` because they
are implementation details of this application. `cmd/` holds the executable.
There is no additional `src/` or `pkg/` layer: the module root already holds
Go source, and the package names describe their roles.

## Run it

From this directory:

```sh
go run ./cmd/counter --engine memory
```

In another terminal:

```sh
curl -X POST http://127.0.0.1:8080/counters/page.home/incr
curl http://127.0.0.1:8080/counters/page.home
```

For a persistent local implementation:

```sh
go run ./cmd/counter --engine sqlite --addr 127.0.0.1:8081
```

SQLite defaults to a stable file under the operating system's temporary
directory, derived from the listen address so restarts reopen the same file.
Pass `--dsn /path/to/counter.db` to choose one yourself.

Valkey and PostgreSQL use the backing services in
[`../../harness/infra/`](../../harness/infra/). Their default connection settings
match those Compose files. Choose `--engine valkey` or `--engine postgres`;
`--dsn` overrides the connection string.

## Build and test

```sh
go build -o bin/counter ./cmd/counter
go test -race ./...
```

Memory and SQLite tests are self-contained. Valkey and PostgreSQL store tests
skip if their services are unreachable; `COUNTER_VALKEY_DSN` and
`COUNTER_POSTGRES_DSN` select test instances. Store conformance tests reset
the counter data, so use dedicated test databases.

The HTTP [contract](../../harness/suites/counter/CONTRACT.md) and
[invariants](../../harness/suites/counter/INVARIANTS.md) belong to the harness.
From the repository root, run its configured SQLite target, including the
crash/restart test:

```sh
go -C harness run ./cmd/harness run counter --target targets/counter-go-sqlite.toml
```

The harness starts and stops the server for this command. Its four counter
target files point here. The counter module does not import the harness;
the harness imports the public counter package through a local module
replacement to preserve its in-process fallback.

The normal command has no fault-injection flags. Harness maintainers use
`harness/cmd/counter-fault` to validate failure detection. The agreed HTTP
contract includes `POST /_reset` to clear counters between runs; that small
contract endpoint does not require importing or implementing the harness.
