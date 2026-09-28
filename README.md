# systems-trinkets

Small, self-contained lessons for building systems engineering skills. Each lesson
takes one systems or distributed-systems concept and works it out in a small
program, written in **Go** or **Deno** (TypeScript). The repo grows one lesson at a time.

## Two ways to learn each lesson

Start with the **base lesson**: implement the mechanism, run its small controlled
experiment, and inspect the results with DuckDB. This is the manual learning
stage, where you work through the systems concept and its correctness checks.

Then try the **HTTP/k6 follow-up**: an AI-assisted adapter calls the same core
logic while k6 supplies concurrent traffic. Use smoke, load, stress, spike, and
short soak experiments to learn how to define a workload, read latency and error
measurements, and explain behavior under pressure. Each lesson's `perf/README.md`
provides a 10–20 minute starting walkthrough. DuckDB reads the k6 CSV directly.

```sh
make up-postgres up-valkey
make lab-cache-aside                         # base lesson and its small experiment
make lab-k6-cache-aside                      # HTTP server + smoke test + analysis
make lab-k6-cache-aside PROFILE=load RATE=10  # steady arrivals against fresh fixtures
```

Available walkthroughs:

- [Cache aside](lessons/go/cache-aside/perf/README.md): expiry and cache misses.
- [Job queue](lessons/go/background-job-queue/perf/README.md): synchronous worker requests and lock contention.
- [Completed job counter](lessons/go/completed-job-counter/perf/README.md): duplicates and idempotency.
- [SQLite WAL](lessons/go/sqlite-wal-lab/perf/README.md): writes while a reader holds a snapshot.
- [Pipelining work](lessons/go/pipelining-work/perf/README.md): round trips amortized by a Valkey pipeline.
- [Cross-store failure](lessons/deno/cross-store-failure/perf/README.md): injected failures and repair.

For future completed lessons, invoke `$add-basic-k6-testing` with the lesson path
to add the same follow-up workflow.

## Makefile commands

Every `make` command runs from the repo root. The Makefile discovers lessons
from the filesystem, so a new lesson directory needs no Makefile edit:

- `lessons/go/<name>/main.go` or `lessons/deno/<name>/main.ts` gives the lesson
  its `run-`, `analyze-`, and `lab-` targets.
- `lessons/<runtime>/<name>/perf/k6.ts` adds the `serve-`, `k6-`,
  `analyze-k6-`, and `lab-k6-` targets.

`make help` prints the target list plus the lesson, k6, and service names that
are currently discovered. Use it whenever you are unsure what a name is.

### Tools you need

| Stage | Required tools |
|---|---|
| Services | Docker with `docker compose` |
| Base lesson | Go (Go lessons), Deno (Deno lessons), `duckdb` CLI for `analyze-` |
| HTTP/k6 follow-up | Everything above plus `k6` and Deno (the runner is `scripts/perf/run.ts`) |
| `make check` | Go and Deno |

On macOS: `brew install k6 duckdb deno go`.

### 1. Services: `up-`, `down-`, `clean-`, `logs-`, `ps`

The backing services run in Docker and are defined in [`services/`](services/index.md).
Start only what a lesson needs; the table in the next section says which.

| Command | What it runs | When to use it |
|---|---|---|
| `make up-<service>` | `docker compose -f services/<service>.compose.yaml up -d --wait` | Before any lesson that uses the service. Returns once the health check passes, so the next command can connect immediately. Safe to repeat; a running service is left alone. |
| `make down-<service>` | `docker compose ... down` | Stop the container but keep its data volume. Use between sessions. |
| `make clean-<service>` | `docker compose ... down -v --remove-orphans` | Stop and delete the data volume. Use when you want a fresh database, cache, or bucket. Lessons reset their own tables and keys, so this is rarely necessary. |
| `make logs-<service>` | `docker compose ... logs -f` | Follow the service log while a lesson runs. Ctrl-C stops following; the service keeps running. |
| `make ps` | `docker ps` filtered to `trinkets-*` | See which services are up, healthy, and on which ports. |

The services are `postgres`, `valkey`, and `seaweedfs`. Several can be started
in one command: `make up-postgres up-valkey`. Connection details and dev
credentials are in [`services/index.md`](services/index.md); lessons use them
by default (`postgres://trinkets:trinkets@localhost:5432/trinkets`,
`redis://localhost:6379`, S3 at `http://localhost:8333` with access key
`trinkets` and secret key `trinkets-secret`).

If `up-` fails with a port conflict, something else is already listening on
that port; see [knowledge/local-services.md](knowledge/local-services.md).

### 2. Base lesson: `run-`, `analyze-`, `lab-`

These run the original lesson: its experiment and its DuckDB analysis. Using
`cache-aside` as the example:

| Command | What it runs | When to use it |
|---|---|---|
| `make run-cache-aside` | `cd lessons/go/cache-aside && go run .` (a Deno lesson runs `deno run -A main.ts` from `lessons/deno/<name>`) | Execute the experiment. Prints progress and writes `measurements.csv` in the lesson directory, overwriting the previous run. |
| `make analyze-cache-aside` | `cd lessons/go/cache-aside && duckdb < analyze.sql` | Load that CSV into DuckDB and print one labelled table per query. Needs a `measurements.csv` from an earlier run. Rerun it as often as you like without repeating the experiment. |
| `make lab-cache-aside` | `run-` then `analyze-` | The normal way to work a lesson: fresh numbers in one command. |

Both recipes run inside the lesson directory because `measurements.csv` and
`analyze.sql` use relative paths. `measurements.csv` is committed as the
lesson's reference result, so `git diff` after a run shows how your numbers
moved.

To point a lesson at another instance, set `DATABASE_URL`, `CACHE_URL`, or
`OBJECT_STORE_URL`:

```sh
DATABASE_URL=postgres://trinkets:trinkets@localhost:15432/trinkets make run-cache-aside
```

### 3. HTTP/k6 follow-up: `lab-k6-`, `serve-`, `k6-`, `analyze-k6-`

These wrap a completed lesson's core logic in a small HTTP adapter
(`perf/main.go` or `perf/server.ts`) and drive it with k6. All four targets call
`scripts/perf/run.ts`, a Deno script that builds or starts the server, waits
for it, runs k6, saves artifacts, analyzes the CSV, and stops its own server.

| Command | What it runs | When to use it |
|---|---|---|
| `make lab-k6-<lesson>` | `run.ts lab`: build/start the adapter on a free port with fresh isolated fixtures, run the k6 workload, capture `/stats`, run DuckDB over the CSV, stop the adapter | The default. One command gives a complete, reproducible run. Start here and for every comparison between settings. |
| `make serve-<lesson>` | `run.ts serve`: run the adapter in the foreground on `localhost:8080` (or `PORT`) until Ctrl-C | When you want to keep a server up between several k6 runs, watch its log live, or hit it with `curl`. Fixtures persist until you stop it. |
| `make k6-<lesson>` | `run.ts run`: run the workload against `BASE_URL` (default `http://127.0.0.1:8080`), capture artifacts | Against a server started with `serve-`. State carries over between runs, so use this for warm-cache or backlog experiments. |
| `make analyze-k6-<lesson>` | `run.ts analyze`: DuckDB over the newest `perf/results/<run-id>/metrics.csv` | Re-read a run without generating traffic, or after editing `analyze.sql`. `RESULTS_DIR=<path>` selects an older run. |

A typical session:

```sh
make up-postgres up-valkey                       # 1. services the lesson needs
make lab-k6-cache-aside                          # 2. smoke: are requests and checks valid?
make lab-k6-cache-aside PROFILE=load             # 3. one hypothesis-relevant profile
make lab-k6-cache-aside PROFILE=load TTL_MS=3000   # 4. change one knob, rerun
make analyze-k6-cache-aside                      # 5. re-read the latest run's tables
```

Settings are passed as `make` variables or environment variables; both reach
the recipe unchanged. Workload settings are read by `scripts/perf/workload.ts`,
adapter settings by the adapter, which reports the values it used on
`GET /health`.

| Setting | Default | Meaning |
|---|---|---|
| `PROFILE` | `smoke` | `smoke` (one virtual user, 10 s), `load` (steady arrival rate, 60 s), `stress` (rate ×1, ×2, ×4, back, 135 s), `spike` (burst to ×8, 62 s), `soak` (steady, 300 s) |
| `RATE` | `5` | Iterations per second for the arrival-rate profiles. The counter lesson sends two requests per iteration. |
| `DURATION_S` | per profile | Overrides the smoke, load, or soak duration (1–3600). |
| `MAX_VUS` | `32` | Upper bound on concurrent virtual users (1–128). |
| `P95_MS` | `1500` | Illustrative latency budget; the run fails its threshold above it. |
| `PORT` | `8080` | Listening port for `serve-`. `lab-k6-` always picks a free port. |
| `BASE_URL` | `http://127.0.0.1:8080` | Where `k6-` sends traffic. Must be a local listener. |
| `RESULTS_DIR` | newest run | Run directory for `analyze-k6-`. |
| `MODE` | per lesson | Adapter variant: queue `skip_locked`/`blocking`, counter `idempotent`/`naive`, SQLite `WAL`/`DELETE`, cross-store `intent_then_put`/`put_then_insert`/`insert_then_put`. |
| `CACHE_TTL_MS`, `WORK_MS`, `POOL_SIZE` | `1000`, `100`, `8` | Cache-aside expiry, queue job duration, and pool size for the queue, SQLite, and cross-store adapters. Each walkthrough names its knob. |

The short soak is practice; longer runs are optional and must respect the
lesson's fixture limits. Intentional overload can fail a run: the runner still
analyzes the CSV, then exits with k6's status, so `make` reports an error.
That is the expected outcome, not a broken setup.

Each run saves raw metrics, the JSON summary, the effective workload options,
the settings the server reported, final domain state, and both logs in the
lesson's ignored `perf/results/<run-id>/` directory. DuckDB runs the shared
`scripts/perf/analyze.sql` over the CSV, then the lesson's optional
`perf/analyze.sql`. Base lesson measurements remain separate.

Performance adapters use private Postgres schemas, unique cache keys and S3
prefixes, or temporary SQLite files, and clean them up on normal shutdown.
Backing services stay running. A forced kill can leave temporary fixtures;
inspect `server.log` in the run directory if cleanup is interrupted. Your
laptop runs both the server and the generator, so these experiments show
mechanisms and relative changes, not production capacity.

### 4. Checks: `check`

| Command | What it runs | When to use it |
|---|---|---|
| `make check` | `go vet ./...` and `deno check .` | Compile-check every Go lesson and adapter, every Deno lesson, the perf runner, and every k6 workload without starting a service. Run it before committing and after editing shared helpers. |

Deno type-checks the k6 workloads against `@types/k6` through the root
`deno.json`; k6 itself runs the same `.ts` files.

## What to run for each lesson

| Lesson | Services to start | Base lesson | Follow-up | Knob to try |
|---|---|---|---|---|
| `cache-aside` | `make up-postgres up-valkey` | `make lab-cache-aside` | `make lab-k6-cache-aside` | `CACHE_TTL_MS=100` |
| `background-job-queue` | `make up-postgres` | `make lab-background-job-queue` | `make lab-k6-background-job-queue` | `MODE=blocking`, `WORK_MS`, `POOL_SIZE` |
| `completed-job-counter` | `make up-postgres` | `make lab-completed-job-counter` | `make lab-k6-completed-job-counter` | `MODE=naive` |
| `sqlite-wal-lab` | none | `make lab-sqlite-wal-lab` | `make lab-k6-sqlite-wal-lab` | `MODE=DELETE` (fails on purpose) |
| `pipelining-work` | `make up-valkey` | `make lab-pipelining-work` | `make lab-k6-pipelining-work` | `MODE=sequential`, `BATCH_SIZE=1000` |
| `cross-store-failure` | `make up-postgres up-seaweedfs` | `make lab-cross-store-failure` | `make lab-k6-cross-store-failure` | `MODE=put_then_insert` |

`make help` lists the current lesson names.

## Layout

Lessons live under `lessons/<runtime>/<name>/`: Go lessons in `lessons/go/`,
Deno lessons in `lessons/deno/`. Each lesson directory has a `main.go` or
`main.ts` that writes `measurements.csv`, and an `analyze.sql` that DuckDB
runs over it. The few helpers every lesson needs (service URLs with env
overrides, the CSV writer) exist once per runtime with the same shape:

| | Go | Deno |
|---|---|---|
| Toolchain root | `go.mod` (repo root, one module) | `deno.json` (repo root: workspace and k6 types, one lockfile) with `lessons/deno/deno.json` as the lesson import map |
| Shared helpers | `internal/lab` (`lab.Env`, `lab.Check`, `lab.NewMeasurements`) | `lessons/deno/lab/lab.ts` (`env`, `sleep`, `mapConcurrent`, `Measurements`) |
| Postgres | `internal/lab/postgres` (`postgres.Connect`) | `lab/postgres.ts` (`postgres.connect`) |
| Valkey | `internal/lab/valkey` (`valkey.Connect`) | `lab/valkey.ts` (`valkey.connect`) |
| SeaweedFS (S3) | not yet needed by a Go lesson | `lab/seaweedfs.ts` (`connect`, `ensureBucket`, `listObjects`, `listKeys`, `exists`, `deleteKeys`) |

Each current lesson separates reusable operations from its experiment runner:
Go lessons expose a `core/` package, and the Deno lesson exposes `core.ts`.
The runner owns connections, fixtures, resets, and CSV reporting; the
`perf/` adapter calls the same operations with its own clients. See
[lesson core entry points](knowledge/lesson-cores.md).

## Stack

Lessons build on a shared set of local services:

- **PostgreSQL** — relational storage
- **Valkey** (Redis-compatible) — caching, queues, pub/sub
- **SeaweedFS** — S3-compatible object storage
- **DuckDB** — analysis of run data (results, timings, traces) produced by lessons

PostgreSQL, Valkey, and SeaweedFS run in Docker, defined in [`services/`](services/index.md).
DuckDB runs in-process or from its CLI, so it needs no service.
