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
  `k6.ts` files re-export its `options` and `handleSummary` and add only the
  operation, its invariant, and any custom metric. Both are TypeScript: k6
  runs them as-is and `deno check .` types them through the root `deno.json`
  (see [deno-lessons.md](deno-lessons.md)). A lesson declares its adapter's
  JSON shapes as interfaces and passes them to `assertResponse<T>` and
  `stats<T>`; `body<T>` casts rather than validates, so the invariant
  predicates remain the runtime check. k6 has no exported type for the
  `handleSummary` argument; `workload.ts` declares the minimal `SummaryData`
  it reads. `RATE` describes iterations;
  the counter workload makes two requests per iteration.
- [analyze.sql](../scripts/perf/analyze.sql) is the one DuckDB analysis: it casts
  the CSV once at load and ends with a query over every custom metric, so a
  lesson needs its own `perf/analyze.sql` only for an extra query; the runner
  appends it after the shared file.
- [Go lifecycle helpers](../internal/lab/perf/perf.go) provide a private schema,
  pgx pool, bounded settings (`Int`, `Choice`), a request cap (`Limit`), error
  responses (`Fail`), HTTP timeouts, and graceful shutdown. Every setting read
  through `Int`/`Choice` is reported by `GET /health` as `settings`, which is
  where `settings.json` comes from. Adapters call the
  [existing cores](lesson-cores.md) and own initialization and HTTP mapping.
- The Deno adapter uses `postgres.pool(max, options)` from
  [lab/postgres.ts](../lessons/deno/lab/postgres.ts): `options` are Postgres
  startup parameters (`-c search_path=... -c statement_timeout=5000`), so every
  pooled connection starts in the private schema without a `set` round trip.

Run artifacts live in ignored `perf/results/<run-id>/`: CSV, JSON summary,
effective workload options, settings, final domain state, and logs. The newest
run directory is the default for `analyze-k6-*` (run ids start with a
timestamp); `RESULTS_DIR` selects another directory. `workload.json` must be
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

The queue adapter is synchronous: one HTTP operation inserts a job and calls the
transactional worker, then returns after completion. It measures contention,
not asynchronous submission latency. The counter omits total for skipped events.
Cross-store simulated crashes are explicit experiment outcomes in successful HTTP
responses; teardown reports state before repair and verifies state after repair.

Custom tags on a request or a custom metric land in the CSV `extra_tags`
column as `key=value&key=value`; system tags such as `name` keep their own
columns. `regexp_extract(extra_tags, 'variant=([^&]+)', 1)` splits samples by
variant, and `select fixtures, unnest(server) from read_json('settings.json')`
prints the server's settings as one row (verified 2026-09-28 with k6 v2.3.0
and DuckDB against a real run directory). The
[add-basic-k6-testing](../.claude/skills/add-basic-k6-testing/SKILL.md) skill
(rewritten 2026-09-28) now expects the lesson's contrast to be a tagged
per-request variant where the core allows it, a required lesson
`perf/analyze.sql` that reproduces the base lesson's table by variant, and
the shared `request` helper extended to accept extra tags. The six existing
`perf/` directories predate that shape: they use one fixed `MODE` per run
and no lesson SQL, so their tables look alike across lessons and the mode is
only visible in `settings.json` and `domain.json`.

k6 CSV is a metric-sample table. Filter by metric name and request name before
counting requests. Stats/repair requests explain why total HTTP counts exceed
operation counts. SQL separates statuses, reports dropped iterations, and shows
ten-second completion buckets including empty buckets; boundary buckets can be
partial. Do not treat these counts as exact per-second throughput.

Threshold failures remain failures even when SQL succeeds. The runner always
analyzes, then returns k6's exit status (99 for the tested threshold failure);
Make reports that error and itself exits 2. SQLite `MODE=DELETE` deliberately
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

Do not compare the smoke numbers as benchmarks; they verify wiring and invariants.
