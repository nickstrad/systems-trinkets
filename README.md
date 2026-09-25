# systems-trinkets

Small, self-contained lessons for building systems engineering skills. Each lesson
takes one systems or distributed-systems concept and works it out in a small
project, written in **Go** or **Deno** (TypeScript). The repo grows one lesson at a time.

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
