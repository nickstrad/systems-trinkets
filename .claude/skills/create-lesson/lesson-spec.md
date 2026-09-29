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
configuration as part of a lesson. If an idea needs unconfigured
software, keep it in `docs/lessons/ideas.md` with its missing prerequisites.
For an explicitly requested idea, report the blocker; for an automatic
selection, choose another eligible existing idea. Do not substitute silently.

Use Go only for lesson cores, runners and HTTP adapters. Deno/TypeScript is
reserved for k6 tooling. Follow the repository local Mac/container boundary. Choose one or at most two primary services
from the configured rows. Choose storage to fit the problem: PostgreSQL when a
relational database is needed; Valkey- or Redis-only, object-storage-only, or
queue-only systems are welcome. When a lesson needs a Redis-protocol store,
pick Valkey or Redis by which feature set suits it: Redis for JSON, search,
vector sets, time series, probabilistic structures (Bloom, Cuckoo, CMS,
Top-K) or `DELEX`; Valkey for Valkey-specific behavior (`DELIFEQ`,
cluster-mode multi-DB) or an explicit Valkey-versus-Redis comparison. Default
to Redis when it does not matter, and name the choice and the reason in the
guide. Existing Valkey lessons stay on Valkey.

Never use SQLite in a new lesson, even though
the repository contains an older SQLite lesson. Always use DuckDB to analyze
measurements; analysis-only DuckDB does not count toward the primary limit.

Whenever connection details appear, copy them from the catalog's `Setup`
column with full credentials; they are fixed local development values.

Vary the primary service, architecture, mechanism, and measured
property across the lessons that exist in the repo. Favor fresh topics or
clearly explain the new angle.

## Repository layout and commands

Choose a short, descriptive, lowercase kebab-case slug, unique across existing Go lessons
and the planned/completed guides in `docs/lessons/`. It becomes the directory name and the Make
target suffix. There are no lesson numbers.

Author the complete guide at `docs/lessons/planned/<slug>.md`; link its
existing idea in `docs/lessons/ideas.md`. Follow `docs/lessons/README.md` for
completion and archival. The source paths below are for the learner to type.

Go lessons:

    lessons/go/<slug>/main.go
    lessons/go/<slug>/core/core.go
    lessons/go/<slug>/analyze.sql

Use only additional files essential to the concept. The repo has one root Go
module, `github.com/nickstrad/systems-trinkets`. Do not create per-lesson
module manifests, lockfiles, Compose files or Makefiles. Prefer drivers already
in `go.mod`; explicitly name any essential shared dependency change.

All Make commands run from the repo root. New `main.go` files
are discovered automatically:

    make run-<slug>       runs the experiment
    make analyze-<slug>   runs DuckDB over analyze.sql
    make lab-<slug>       runs the experiment and then the analysis

The recipes change into the lesson directory before running `go run .` and `duckdb < analyze.sql`. Write `measurements.csv` in
that directory, overwriting the previous run. Analyze it with relative paths.
Show concrete commands with the chosen slug, never placeholders. Start only
the services the lesson needs, with their `make up-<service>` targets.

## Existing helpers: import them, do not reimplement them

The current source is authoritative; inspect `internal/lab/` on every invocation. As of this writing they provide:

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
    redis.URL() string                      // env REDIS_URL, default redis://localhost:6380
    redis.Connect(ctx) *goredis.Client      // go-redis/v9, pings

The `internal/lab/redis` package is named `redis`, so import go-redis as
`goredis` when both appear in one file. Confirm both helpers exist in
`internal/lab/` before relying on them.

There is no configured Go S3 client/helper yet. Keep S3 ideas blocked on that
catalog entry until it is configured; do not silently substitute another
language. Import a Go core as
`github.com/nickstrad/systems-trinkets/lessons/go/<slug>/core`.

Services without a helper yet (for example NATS, etcd, Temporal, Toxiproxy,
OpenBao, PgBouncer, the registry) are still usable when configured: name the
client package, show the connection setup in the runner, and state the
`go.mod` addition if one is needed.

## Core and experiment: keep the future k6 adapter easy

The standalone lesson must finish and teach its concept with
`make lab-<slug>`, without an HTTP server or k6. Separate the reusable
operation from its small experiment runner:

- `core/core.go` uses `package core`. Export shared DDL as `Schema` when needed.
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
  pool be passed.
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
