# Lesson specification

The detailed contract for one systems-trinkets lesson. `SKILL.md` says how to
choose a topic, what to inspect, and what to output; this file says what a
lesson is. It replaces the retired `docs/prompts/daily_lesson/v3.txt`.

## Spirit and scope

Build one small system around one useful, common systems concept. Choose
exactly one main systems property to teach, such as reliability, durability,
latency, availability, or consistency. State a concrete question and an
observable invariant or outcome that will answer it. Prefer a small controlled
experiment, with a baseline and one changed mechanism when useful. Keep the
typing, running, and analysis realistic for about 10 minutes, and the whole
lesson for about 20; simplify the experiment if necessary.

Use AI cloud environments as the north star: the lessons should help someone
building or operating platforms like Daytona, E2B, Vercel Sandbox, Modal,
Inngest, Temporal, or Vercel Workflows. Think about isolation, cold start,
leases and reaping, durable steps, retries and idempotency, event delivery,
placement, secrets that expire, quotas, metering, and file and artifact
movement. Small local runs demonstrate a mechanism, not production capacity.

## Software: only what is configured

`software/software.md` is the catalog. A lesson may use only software whose
row says `Configured: yes`. That covers the runtimes, the compose-backed
services started with `make up-<service>`, and built-in features of those
services. Do not use software marked `no`, do not ask the learner to install
anything, and do not add compose files, module manifests, or Docker
configuration as part of a lesson. If the best idea needs unconfigured
software, either choose another idea or say plainly which catalog row would
need to be configured first, then choose another idea.

Use Go or TypeScript with Deno. Choose one or at most two primary services
from the configured rows. Choose storage to fit the problem: PostgreSQL when a
relational database is needed; Valkey-only, object-storage-only, or
queue-only systems are welcome. Never use SQLite in a new lesson, even though
the repository contains an older SQLite lesson. Always use DuckDB to analyze
measurements; analysis-only DuckDB does not count toward the primary limit.

Whenever connection details appear, copy them from the catalog's `Setup`
column with full credentials; they are fixed local development values.

Vary the language, primary service, architecture, mechanism, and measured
property across the lessons that exist in the repo. Favor fresh topics or
clearly explain the new angle.

## Repository layout and commands

Choose a short, descriptive, lowercase kebab-case slug, unique across both
runtimes and `docs/lessons/`. It becomes the directory name and the Make
target suffix. There are no lesson numbers.

Go lessons:

    lessons/go/<slug>/main.go
    lessons/go/<slug>/core/core.go
    lessons/go/<slug>/analyze.sql

Deno lessons:

    lessons/deno/<slug>/main.ts
    lessons/deno/<slug>/core.ts
    lessons/deno/<slug>/analyze.sql

Use only additional files essential to the concept. The repo has one root Go
module, `github.com/nickstrad/systems-trinkets`, and one Deno workspace with
`lessons/deno/deno.json` as its member import map. Do not create per-lesson
`go.mod`, `deno.json`, lockfiles, Compose files, or Makefiles. Prefer drivers
already in `go.mod` or the import map; if a new dependency is essential, name
the shared dependency change explicitly instead of assuming a helper exists.

All Make commands run from the repo root. New `main.go` and `main.ts` files
are discovered automatically:

    make run-<slug>       runs the experiment
    make analyze-<slug>   runs DuckDB over analyze.sql
    make lab-<slug>       runs the experiment and then the analysis

The recipes change into the lesson directory before running `go run .` or
`deno run -A main.ts`, and `duckdb < analyze.sql`. Write `measurements.csv` in
that directory, overwriting the previous run. Analyze it with relative paths.
Show concrete commands with the chosen slug, never placeholders. Start only
the services the lesson needs, with their `make up-<service>` targets.

## Existing helpers: import them, do not reimplement them

The current source is authoritative; inspect `internal/lab/` and
`lessons/deno/lab/` on every invocation. As of this writing they provide:

Go, under `github.com/nickstrad/systems-trinkets/internal/lab`:

    lab.Env(key, fallback string) string
    lab.Check(err error)                    // panics on error; runner only
    lab.Ms(d time.Duration) float64
    lab.NewMeasurements(columns ...string) *lab.Measurements
    m.Write(fields ...string)               // format numbers yourself
    m.Close()                               // flush, close, print file name
    postgres.URL() string
    postgres.Connect(ctx) *pgx.Conn         // pgx/v5
    valkey.URL() string
    valkey.Connect(ctx) *redis.Client       // go-redis/v9, pings

There is no shared Go SeaweedFS helper; choose Deno for a compact S3 lesson
or supply the client setup explicitly. Import a Go core as
`github.com/nickstrad/systems-trinkets/lessons/go/<slug>/core`.

Deno, from the `lab/` import map entry:

    import { env, sleep, mapConcurrent, Measurements } from "lab/lab.ts";
    import * as postgres from "lab/postgres.ts";   // url(), connect(), pool(max, options?)
    import * as valkey from "lab/valkey.ts";       // url(), connect()
    import * as seaweedfs from "lab/seaweedfs.ts"; // url(), connect(), ensureBucket,
                                                   // listObjects, listKeys, exists, deleteKeys

The import map supplies `pg`, `redis`, and `@aws-sdk/client-s3`. Direct
imports from `pg` need `// @ts-types="npm:@types/pg@^8"` above the import.
Use top-level await in `main.ts`, import the core from `./core.ts`, and close
resources in `finally`. Time with `performance.now()`.

Services without a helper yet (for example NATS, etcd, Temporal, Toxiproxy,
OpenBao, PgBouncer, the registry) are still usable when configured: name the
client package, show the connection setup in the runner, and state the
`go.mod` or import-map addition if one is needed.

## Core and experiment: keep the future k6 adapter easy

The standalone lesson must finish and teach its concept with
`make lab-<slug>`, without an HTTP server or k6. Separate the reusable
operation from its small experiment runner:

- `core/core.go` uses `package core`; `core.ts` exports functions or a small
  factory. Export shared DDL as `Schema` (Go) or `schema` (Deno) when needed.
- The core accepts caller-owned clients and explicit inputs and settings,
  performs the actual operation, and returns a small useful result plus
  errors. Distinguish expected outcomes (duplicate, miss, empty queue,
  injected failure) from unexpected dependency errors, and say which result
  fields are meaningful for each outcome.
- The runner owns connections, fixtures, resets, experiment loops, timing,
  invariant checks, console output, CSV writing, and cleanup. Importing the
  core must not connect, run the experiment, or change storage. Core
  operations must not reset fixtures, print, write CSV, or exit.
- Keep transactions inside the operation when they express the mechanism.
  In Go, a small interface over the needed pgx methods lets a connection or
  pool be passed. In Deno, keep each transaction on one checked-out client.
  Concurrent transactions need separate connections.
- Use lesson-specific tables, keys, subjects, buckets, and object prefixes.
  Reset only this lesson's fixtures. Make key and prefix construction
  reusable. Avoid global mutable state and assumptions that IDs arrive in
  order. Keep concurrency and fixture growth bounded. No service-wide
  flushes or volume deletion.

Use ordinary functions and minimal types; do not build a framework. A later
HTTP request should call one operation or a defined unit of work, not rerun
`main` or reset the whole experiment.

## Measure and explain

Measure one meaningful property tied to the lesson's question, with the
supporting columns needed to interpret it. Record units, variant or mode,
operation identity, and outcome. Check a domain invariant as well as timing:
fast but incorrect is not success. Fail on unexpected setup or dependency
errors; record deliberate failure scenarios explicitly.

State what the measurement includes and excludes. Keep fixture setup and CSV
writes outside timed regions. Warm up when measuring steady-state latency,
then restore the intended starting state; if cold start is the subject, say
so. For concurrency lessons, use a controlled overlap rather than lucky
scheduling. Distinguish a simulated interruption from a real process or
storage crash, and do not claim durability the experiment did not test.

`analyze.sql` holds two or three short DuckDB queries over `measurements.csv`
that answer the question. Explain each result briefly, including the expected
relationship or invariant. Label illustrative output as illustrative; never
invent measured results or claim code was run when it was not.
