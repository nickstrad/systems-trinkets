# systems-trinkets

Small, self-contained lessons for building systems engineering skills. Each lesson
takes one systems or distributed-systems concept and works it out in a small
program, written in **Go** or **Deno** (TypeScript). The repo grows one lesson at a time.

## Layout

Lessons live under `lessons/<runtime>/<name>/`: Go lessons in `lessons/go/`,
Deno lessons in `lessons/deno/`. Each lesson directory has a `main.go` or
`main.ts` that writes `measurements.csv`, and an `analyze.sql` that DuckDB
runs over it. The few helpers every lesson needs (service URLs with env
overrides, the CSV writer) exist once per runtime with the same shape:

| | Go | Deno |
|---|---|---|
| Toolchain root | `go.mod` (repo root, one module) | `lessons/deno/deno.json` (import map, one lockfile) |
| Shared helpers | `internal/lab` (`lab.Env`, `lab.Check`, `lab.NewMeasurements`) | `lessons/deno/lab/lab.ts` (`env`, `sleep`, `mapConcurrent`, `Measurements`) |
| Postgres | `internal/lab/postgres` (`postgres.Connect`) | `lab/postgres.ts` (`postgres.connect`) |
| Valkey | `internal/lab/valkey` (`valkey.Connect`) | `lab/valkey.ts` (`valkey.connect`) |
| SeaweedFS (S3) | not yet needed by a Go lesson | `lab/seaweedfs.ts` (`connect`, `ensureBucket`, `listObjects`, `listKeys`, `exists`, `deleteKeys`) |

A new lesson is a new directory under `lessons/go/` or `lessons/deno/` with
its entry file and `analyze.sql`; the Makefile discovers it, so no Makefile
edit is needed.

Each current lesson separates reusable operations from its experiment runner:
Go lessons expose a `core/` package, and the Deno lesson exposes `core.ts`.
The runner owns connections, fixtures, resets, and CSV reporting. A future HTTP
handler can call the same operations with its own clients. See
[lesson core entry points](knowledge/lesson-cores.md).

## Running a lesson

Every `make` command runs from the repo root. A lesson's name is its directory
name under `lessons/go/` or `lessons/deno/`; `make help` lists them.

```sh
make help                        # lists lessons and services
make up-postgres up-valkey       # start the services this lesson needs
make lab-cache-aside             # run the lesson, then analyze its measurements
make check                       # go vet ./... and deno check, for every lesson
```

The three per-lesson targets, using `cache-aside` as the example:

| Command | What it does |
|---|---|
| `make run-cache-aside` | `cd lessons/go/cache-aside && go run .` (a Deno lesson runs `deno run -A main.ts` from `lessons/deno/<name>`). Prints progress and writes `measurements.csv` in that directory. |
| `make analyze-cache-aside` | `cd lessons/go/cache-aside && duckdb < analyze.sql`. Loads that CSV into DuckDB and prints one labelled table per query. Needs a `measurements.csv` from an earlier run. |
| `make lab-cache-aside` | `run-` then `analyze-`, so one command gives fresh numbers. |

The Makefile discovers lessons with a wildcard over `lessons/go/*/main.go`
and `lessons/deno/*/main.ts`, so a new lesson directory gets these three
targets with no Makefile edit.

Each lesson's entry file says which services it needs; start them first with
the commands below. Lessons connect with the local dev credentials by default
(`postgres://trinkets:trinkets@localhost:5432/trinkets`,
`redis://localhost:6379`, S3 at `http://localhost:8333` with access key
`trinkets` and secret key `trinkets-secret`). To point a lesson elsewhere, set
`DATABASE_URL`, `CACHE_URL`, or `OBJECT_STORE_URL`, for example:

```sh
DATABASE_URL=postgres://trinkets:trinkets@localhost:15432/trinkets make run-cache-aside
```

## Stack

Lessons build on a shared set of local services:

- **PostgreSQL** — relational storage
- **Valkey** (Redis-compatible) — caching, queues, pub/sub
- **SeaweedFS** — S3-compatible object storage
- **DuckDB** — analysis of run data (results, timings, traces) produced by lessons

PostgreSQL, Valkey, and SeaweedFS run in Docker, defined in [`services/`](services/index.md).
DuckDB runs in-process or from its CLI, so it needs no service.

## Running services

Start only what a lesson needs:

```sh
make up-postgres      # also: up-valkey, up-seaweedfs
make down-postgres    # stop, keep data
make clean-postgres   # stop and delete data
make logs-postgres
make ps               # list running services
```

Connection details are in [`services/index.md`](services/index.md).
