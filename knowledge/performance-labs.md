# HTTP and k6 performance follow-ups

Applies after completing a base lesson. Each existing lesson has a `perf/`
directory containing an HTTP entry point, `k6.ts`, and a short walkthrough.
Base lesson runners, measurements, and analysis remain separate.
See [README](../README.md) for the two learning stages and command table.

## Shared pieces and commands

- [Makefile](../Makefile) discovers `lessons/*/*/perf/k6.ts` and adds
  `serve-`, `k6-`, `analyze-k6-`, and `lab-k6-` targets; `make help` lists them.
  Start backing services with their existing `up-` targets first. `lab-k6-*`
  defaults to a ten-second smoke.
- [run.ts](../scripts/perf/run.ts) (Deno, type-checked by `make check`) builds
  or starts a server, waits for its actual localhost URL, runs k6, captures
  artifacts, analyzes the CSV, and shuts down its own server. Combined runs use
  an available port. Standalone `serve-*` uses port 8080 or `PORT`; standalone
  `k6-*` uses `BASE_URL` and reuses existing server state. Go adapters build to
  one `perf/results/server` binary per lesson, not one per run.
- [workload.ts](../scripts/perf/workload.ts) holds bounded smoke/load/stress/spike/
  soak profiles, request helpers, checks, thresholds, and the summary. Lesson
  `k6.ts` files re-export its `options` and `handleSummary` (or call
  `optionsFor(defaults, extra)` for a lesson default rate, p95 budget, or k6
  option such as `batch`) and add only the operation, its invariant, and the
  custom metric. Both are TypeScript: k6 runs them as-is and `deno check .`
  types them through the root `deno.json` (see [deno-lessons.md](deno-lessons.md)).
  Helpers: `request(method, path, tags)` merges the tags with
  `name: operation` (`operationParams(tags)` gives the same params to
  `http.batch`); `assertResponse<T>` runs the two checks and returns the
  parsed body on success so metrics are recorded from it; `number(name,
  fallback, min, max)` reads a workload knob; `settings<T>()` reads the
  server's `/health` settings in `setup()` so a server-owned knob is not
  validated twice; `everyVariant(names, predicate)` is the teardown for
  `/stats` shaped `{variants: [...]}`. `body<T>` casts rather than
  validates, so the invariant predicates remain the runtime check. k6 has no
  exported type for the `handleSummary` argument; `workload.ts` declares the
  minimal `SummaryData` it reads. `RATE` describes iterations; the counter
  workload makes `DELIVERIES` requests per iteration and the queue workload
  `WORKERS`.
- [analyze.sql](../scripts/perf/analyze.sql) is the shared floor: it prints
  `settings.json` first, defines the `tag(extra, key)` macro (an anchored
  `regexp_extract` over `extra_tags`), casts the CSV once at load, and ends
  with a query over every custom metric. The runner appends the lesson's
  required `perf/analyze.sql`, which reproduces the base lesson's table by
  variant, the question in named-group form, and the invariant from
  `domain.json`.
- [Go lifecycle helpers](../internal/lab/perf/perf.go) provide a private schema,
  pgx pool, a pool warm-up (`Warm` runs a function on every connection the
  pool may open, because pgx prepares statements per connection and a lazy
  dial otherwise lands inside a timed request), bounded settings (`Int`,
  `Choice`), a request cap (`Limit`), error responses (`Fail`), HTTP
  timeouts, and graceful shutdown. A core whose queries name a table without
  a schema (the queue) gets one `Database` call, so one schema and pool, per
  variant. Every setting read
  through `Int`/`Choice` is reported by `GET /health` as `settings`, which is
  where `settings.json` comes from. Adapters call the
  [existing cores](lesson-cores.md) and own initialization and HTTP mapping.
- The Deno adapter uses `postgres.pool(max, options)` from
  [lab/postgres.ts](../lessons/deno/lab/postgres.ts): `options` are Postgres
  startup parameters (`-c search_path=... -c statement_timeout=5000`), so every
  pooled connection starts in the private schema without a `set` round trip.

Run artifacts live in ignored `perf/results/<run-id>/`: CSV, JSON summary,
effective workload options, settings, final domain state, and logs. The newest
run directory that holds `metrics.csv` is the default for `analyze-k6-*` (run
ids start with a timestamp; a run that failed before traffic, such as
`PROFILE=bogus`, keeps its directory and `server.log` but is skipped);
`RESULTS_DIR` selects another directory. `workload.json` must be
generated with `k6 inspect --include-system-env-vars`: plain `inspect` does not
receive environment-selected PROFILE/RATE overrides (with plain `inspect`,
`PROFILE=bogus` silently inspects the smoke profile).

## Make passes settings through without `export`

GNU Make (3.81 verified) forwards both `make lab-k6-x PROFILE=load` command-line
variables and inherited environment variables (`RATE=10 make ...`) to recipe
processes as-is. Do not add `export PROFILE RATE ...` lines for optional
settings: `export` of an undefined variable exports it as an empty string,
which then has to be stripped downstream and shows up as `"MAX_VUS": ""` in
artifacts. Defaults belong in the consumer (`workload.ts`, `perf.Int`,
`env()`), which all treat empty as unset.

## Isolation and interpretation

Go Postgres adapters set pool search_path to their random `perf_*` schema; the
Deno adapter does the same through pool startup options. Cleanup drops only
that schema. Cache profiles use a unique key; S3 adapters use a unique prefix;
SQLite uses a temporary database. Normal shutdown cleans these fixtures. A
forced kill can leave them behind; server logs identify Postgres/S3 fixtures.
Never clean the entire backing service to reset a performance experiment.

Every lesson's contrast is a tagged per-request variant in one run, except
cache-aside, whose knob (`TTL_MS`) is server-level and compared across a pair
of runs (remade 2026-09-28 with the
[add-basic-k6-testing](../.claude/skills/add-basic-k6-testing/SKILL.md) skill):

- **pipelining-work:** `?variant=sequential|pipeline`, one key set per
  variant, `core_ms` trend; `/stats` per variant (`expected_sum` vs MGET).
- **completed-job-counter:** `?variant=naive|idempotent&id&attempt`; the event
  id is the k6 iteration number, so an event's only duplicates are its own
  `DELIVERIES` attempts (read from `/health` in `setup()`). The connection is
  acquired before the timer so pool waits stay outside `elapsed_ms`.
- **background-job-queue:** `?variant=blocking|skip_locked` with one schema,
  table, and pool per variant, so blocking claims never hold rows a
  skip_locked claim wants. One iteration is one base-lesson round of
  `WORKERS` simultaneous requests via `http.batch`: evenly spaced single
  arrivals show no lock wait until the queue saturates. `elapsed_ms` is
  `Claim.Took`; HTTP time also holds the insert and the `WORK_MS` sleep.
- **sqlite-wal-lab:** `?variant=DELETE|WAL`, two temporary databases in one
  server, each with a reader snapshot held for the server's life (a waiting
  DELETE writer holds PENDING, which blocks new readers, so a per-request
  snapshot would fail as a setup error). Every pool connection is opened and
  warmed at startup because a new connection needs SHARED. `BUSY_MS`
  defaults to 250 so the default run passes the p95 budget; `BUSY_MS=2000` is
  the documented failing run (k6 exit 99, Make exit 2). A locked write is a
  200 with `outcome: locked`, the lesson's measured outcome.
- **cache-aside:** `GET /operation?id` over `PROFILE_COUNT` seeded profiles
  (read from `/health` in `setup()`), ids shifted by a random base so
  `core.ProfileKey` keys are unique per server. The source is known only
  from the response, so k6 re-records `response.timings.duration` as
  `operation_ms` tagged by `source`; a request cannot be tagged after it is
  sent. `redundant_misses` counts stampedes; the default round-robin never
  produces one. Avoid a TTL that is a multiple of the per-key read interval.
- **cross-store-failure (Deno):** `?variant=<write order>&crash=0|1`, one
  schema, S3 sub-prefix, and `createOperations` per variant, because
  metadata rows are not prefix-scoped and another variant's rows would look
  dangling to the reconciler. `/repair` answers 409 while uploads are in
  flight and keeps the pre-repair measurement so `domain.json` holds both
  states. The base lesson's grace experiment is deliberately not repeated.

Custom tags on a request or a custom metric land in the CSV `extra_tags`
column as `key=value&key=value`; system tags such as `name` keep their own
columns. `tag(extra_tags, 'variant')` splits samples by variant (verified
2026-09-28 with k6 v2.3.0 and DuckDB against real run directories, including
`variant=…&crash=…` pairs). Adapters report the invariant's inputs per
variant, not booleans, and every base lesson still runs afterwards; running
one rewrites its `measurements.csv`, so copy it aside first.

k6 CSV is a metric-sample table. Filter by metric name and request name before
counting requests. Stats/repair requests explain why total HTTP counts exceed
operation counts. SQL separates statuses, reports dropped iterations, and shows
ten-second completion buckets including empty buckets; boundary buckets can be
partial. Do not treat these counts as exact per-second throughput.

Threshold failures remain failures even when SQL succeeds. The runner always
analyzes, then returns k6's exit status (99 for the tested threshold failure);
Make reports that error and itself exits 2. SQLite `BUSY_MS=2000` deliberately
demonstrates this path. Guard this when editing the runner: `status || analyze()`
would skip the analysis exactly when it is most useful.
The 1500 ms p95 budget is illustrative. Short smoke/soak runs and shared laptop
resources do not establish production capacity or long-term reliability.

## Verification, 2026-09-27

- `make check` passed with all Go adapters, the Deno server, and `run.ts`.
- After the `.js` to `.ts` conversion (same day): `make check` passed from the
  root, `k6 inspect` parsed all five `k6.ts`, `PROFILE=bogus` still fails
  before traffic, and three-second `lab-k6-cache-aside` and
  `lab-k6-cross-store-failure` smokes passed with all checks.
- Three-second `lab-k6-*` smokes passed for all five lessons with real local
  services and DuckDB, including `MODE=blocking` for the queue.
- SQLite `MODE=DELETE` with `DURATION_S=2` produced failed writes; the shared
  SQL still ran and Make exited with k6's status 99.
- Manual flow: `PORT=8091 make serve-cache-aside`, then
  `BASE_URL=http://127.0.0.1:8091 make k6-cache-aside`; `settings.json` recorded
  the server's reported settings and "existing server" fixtures; SIGINT stopped
  the server and no `perf_*` schema remained.
- A run directory holds about 72 KB; before the shared binary it held 16 MB.
- Full stress, spike, and soak durations have not been executed.

## Verification, 2026-09-28 (pipelining-work)

- `make check` passed with the new Go adapter and `k6.ts`; `k6 inspect` parsed
  the workload and `PROFILE=bogus` failed before traffic.
- Ten-second `lab-k6-pipelining-work` smoke passed; SQL operation count (97)
  matched the k6 summary, and the `increments` custom metric (97 samples of
  200) matched `expected_sum` and `actual_sum` (19400) in `domain.json`.
- Twenty-second `PROFILE=load` runs at 5 batches/s: `MODE=pipeline` p50 2.3 ms,
  `MODE=sequential` p50 59 ms, both with zero failures, zero dropped
  iterations, and matching sums. Neither mode reached saturation at that rate.
- Sequential p50 over HTTP (59 ms) was well above the base lesson's single
  batch (33 ms) at the same 200 keys; the cause was not investigated.
- No `perf:pipeline:*` keys remained in Valkey after the server shut down.
- Stress, spike, and soak were not executed for this lesson.

## Verification, 2026-09-28 (all six perf/ directories remade)

Each lesson ran `make check`, `k6 inspect`, a `PROFILE=bogus` failure before
traffic, a ten-second smoke, and a twenty-second `PROFILE=load` at the default
rate against real services; the base lesson ran afterwards and no fixture
remained. Load numbers (p50 of the server's core timing unless noted):

- pipelining-work: sequential 61 ms vs pipeline 1.6 ms (ratio 38 on core
  time, 30 over HTTP; the base lesson's single-batch ratio is 125). Sums
  matched for both variants.
- completed-job-counter: naive overcount 2.0 per event, idempotent 0; the
  invariant difference was 0 for both.
- background-job-queue at `WORKERS=2`: blocking max claim 110 ms vs
  skip_locked 1.8 ms (slowdown 60; the base row is 121 because its skip_locked
  max was 0.9 ms); every claim got its own job and finished. `WORKERS=1` at
  the same rate gave slowdown 1.0: no in-round contention.
- sqlite-wal-lab at `BUSY_MS=250`: DELETE 100% locked at 260 ms, WAL 0%
  locked at 0.8 ms; rows equalled seed plus successful writes. Before the
  pool warm-up, `BUSY_MS=2000` produced 503s from connection setup.
- cache-aside, pair of runs: `TTL_MS=30000` hit rate 90%, `TTL_MS=3000` 50%;
  miss-to-hit ratio 2.1 and 2.3 (base 3.2).
- cross-store-failure: each order left only its own failure class and all
  three were consistent after repair; `intent_then_put` kept every upload,
  the others `uploads - crashed`; the pending repair checked 8x fewer items
  but cost more per item.

**Idle gaps inflate core timing.** Every lesson's core time under k6 was above
the base runner's: sequential batches 62 ms at one VU vs 35 ms back to back,
cache hits 1.2 ms vs 0.4 ms, first deliveries 3.2 ms vs retries 1.5 ms in the
naive counter (identical work). Requests spaced 100–200 ms apart pay a
wake-up cost the base runner's back-to-back loop never does; the mechanism
(CPU or scheduler wake-up, Docker Desktop VM) was not isolated. This explains
the earlier "59 ms, cause not investigated" note. Compare k6 runs with k6
runs, and the base CSV with the base CSV.

Stress, spike, and soak were not executed for any lesson. Do not compare the
smoke numbers as benchmarks; they verify wiring and invariants.
