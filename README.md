# systems-trinkets

Small, self-contained lessons for building systems engineering skills. Each lesson
takes one systems or distributed-systems concept and works it out in a small
program, written in **Go** or **Deno** (TypeScript). The repo grows one lesson at a time.

## Layout

The repo is one Go module. Each lesson is a directory under `lessons/` with a
`main.go` (or `main.ts`) that writes `measurements.csv`, and an `analyze.sql`
that DuckDB runs over it. The few helpers every lesson needs (service URLs,
fail-fast `Check`, the CSV writer) live in `internal/lab`.

## Running a lesson

Every `make` command runs from the repo root. A lesson's name is its directory
name under `lessons/`; `make help` lists them.

```sh
make help                        # lists lessons and services
make up-postgres up-valkey       # start the services this lesson needs
make lab-cache-aside             # run the lesson, then analyze its measurements
```

The three per-lesson targets, using `cache-aside` as the example:

| Command | What it does |
|---|---|
| `make run-cache-aside` | `cd lessons/cache-aside && go run .` (or `deno run -A main.ts` if the lesson has a `main.ts`). Prints progress and writes `lessons/cache-aside/measurements.csv`. |
| `make analyze-cache-aside` | `cd lessons/cache-aside && duckdb < analyze.sql`. Loads that CSV into DuckDB and prints one labelled table per query. Needs a `measurements.csv` from an earlier run. |
| `make lab-cache-aside` | `run-` then `analyze-`, so one command gives fresh numbers. |

The Makefile discovers lessons with a wildcard, so a new directory under
`lessons/` gets these three targets with no Makefile edit.

Each lesson's `main.go` says which services it needs; start them first with the
commands below. Lessons connect with the local dev credentials by default
(`postgres://trinkets:trinkets@localhost:5432/trinkets`,
`redis://localhost:6379`). To point a lesson elsewhere, set `DATABASE_URL` or
`CACHE_URL`, for example:

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
