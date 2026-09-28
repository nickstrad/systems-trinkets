# cache-aside: the same experiment under concurrent traffic

Spend 10–20 minutes here after the base lesson. The base runner read one profile
21 times in a row with a 30-second TTL: one miss, then hits. Here k6 sends the
same `core.ReadProfile` call as HTTP requests over several profiles, and the TTL
becomes the knob you turn between two runs.

**Hypothesis:** under concurrent traffic a miss (`postgres`) is still slower
than a hit (`cache`), so `miss_to_hit_ratio` stays above 1. With a TTL that
outlasts the run, only the first read of each profile misses. With a TTL shorter
than the gap between reads of a profile, misses recur on every expiry and the
hit rate falls. Every read returns the name Postgres stores.

**Predict first.** Your base lesson measured **1312 µs for the miss** and about
**381 µs per hit** (ratio 3.4, hit rate 95.2 % over 21 reads). Here k6 sends 5
reads per second, round-robin over 10 profiles, so each profile is read every 2
seconds. Write down your guess for `hit_rate_pct`, `miss_us`, `hit_us`, and
`miss_to_hit_ratio` at `TTL_MS=30000` and at `TTL_MS=3000`.

## What the adapter does

- `GET /operation?id=<1..PROFILE_COUNT>` calls `core.ReadProfile` once for that
  profile and returns `id`, `name`, `source` (`cache` or `postgres`), and
  `elapsed_ms`. Any other id answers 400.
- `elapsed_ms` times only the core call, the way `main.go` times `latency_us`.
  k6 records it as `core_ms`, tagged by `source`; the SQL shows it in
  microseconds beside the base table's columns.
- Two server settings, reported by `/health` and printed in the settings header
  at the top of the SQL output: `TTL_MS` (default 30000, the base `cacheTTL`)
  and `PROFILE_COUNT` (default 10). The workload reads `PROFILE_COUNT` from
  `/health`, so you set it once, on the make command.
- At startup the server creates a private `perf_*` schema in
  `postgres://trinkets:trinkets@localhost:5432/trinkets` and seeds profiles
  `Ada 1` … `Ada N` in one insert. pgx prepares a statement per connection on
  first use, so the server runs one untimed miss on every pool connection
  (`POOL_SIZE`, default 8), then deletes the keys so the cache starts cold. No
  timed miss pays for a new connection or a prepare.
- `core.ProfileKey(id)` always builds `profile:<id>`, so the server gives its
  profiles random ids far above the base lesson's `42` (the server log prints
  the key range). Its keys in `redis://localhost:6379` cannot collide with the
  base lesson's or another server's, and shutdown deletes them.
- `GET /stats` returns `served`, `hits`, `misses`, `failed`, `redundant_misses`,
  and, read at that moment, `stored` (rows in Postgres), `cached` (keys still in
  Valkey), and `cached_agree` (cached values equal to the stored name).
- `redundant_misses` counts misses that started before another miss of the same
  profile had returned: two requests both found the key missing and both read
  Postgres. That is the unit of a cache stampede.

## Run from the repository root

```sh
make up-postgres up-valkey
make lab-k6-cache-aside                                   # 10-second smoke: wiring and invariant
make lab-k6-cache-aside PROFILE=load TTL_MS=30000         # 60 s at 5 reads/s, TTL outlasts the run
make lab-k6-cache-aside PROFILE=load TTL_MS=3000          # same traffic, TTL expires ~20 times
make analyze-k6-cache-aside                               # repeat the SQL on the latest run
RESULTS_DIR=lessons/go/cache-aside/perf/results/<run-id> make analyze-k6-cache-aside
```

Add `DURATION_S=20` to both load runs for a quicker pair. `RATE=5` keeps the
server far below saturation (each read takes a few milliseconds); raise it to
look for the knee.

Why 3000: each profile is read every `PROFILE_COUNT / RATE` = 2 seconds. A
3-second TTL expires between every second read, so about half the reads miss. A
TTL of an exact multiple of 2 seconds would put reads right on the expiry and
make the result depend on timing jitter.

## Read the tables

The shared tables print first. The lesson's tables follow the
`== cache-aside ==` banner:

The settings header at the very top shows `TTL_MS` and `PROFILE_COUNT` for this
run; check them before any number below.

1. **Base lesson table under load**: one row per source, the base table's
   `avg_us`, `p50_us`, `p95_us` (core time), plus the same request's HTTP time
   in `*_http_ms`. `requests` sums to the shared "Operation HTTP count"; an
   `error` row would hold failed requests.
2. **Summary**: start here. Compare `hit_rate_pct` between the two runs, then
   `miss_us` and `hit_us` with your base 1312 µs and 381 µs. `miss_to_hit_ratio`
   is the base lesson's number; `http_miss_to_hit_ratio` is what a client over
   HTTP sees.
3. **Misses over time**: with `TTL_MS=30000` every miss is in the first 5-second
   bucket; with `TTL_MS=3000` misses appear in every bucket.
4. **Invariant**: `served` must equal `k6_served` (reads whose name k6 checked
   against the stored `Ada <id>`), `failed` must be 0, and `cached_agree` must
   equal `cached`.

A 20-second pair on a laptop measured: `TTL_MS=30000` hit rate 90.1 % (10 misses
of 101), `TTL_MS=3000` 49.5 % (51 of 101); `miss_us` about 2500 to 2700 and
`hit_us` about 1170, a ratio of 2.1 to 2.3.

## Optional profiles

One command each; the default traffic above stays under five minutes.

- `make lab-k6-cache-aside PROFILE=stress TTL_MS=3000`: about 2.5 minutes
  stepping from 5 to 20 reads/s. With a fixed TTL, what happens to the hit rate
  as the rate rises, and why?
- `make lab-k6-cache-aside PROFILE=spike RATE=100 PROFILE_COUNT=1 TTL_MS=100`:
  about 1 minute with a burst toward 800 reads/s of one profile whose key
  expires ten times a second. Does `redundant_misses` rise above 0? Dropped
  iterations are likely at this rate; they mean k6 ran out of virtual users.
- `make lab-k6-cache-aside PROFILE=soak TTL_MS=3000`: 5 minutes. Do the medians
  drift as the run goes on?
- `RATE`, `MAX_VUS`, and `DURATION_S` apply to any profile; `PROFILE_COUNT`
  changes the gap between reads of one profile.

## Questions only this lesson raises

1. Your base run measured hits at about 0.38 ms. Here `hit_us` is likely two to
   three times that, although each hit is still one Valkey GET. The base runner
   reads back to back; k6 at 5 reads/s leaves 200 ms between reads. What could
   make an idle connection's next round trip slower? (The pipelining-work
   follow-up saw the same effect.)
2. `redundant_misses` probably stayed 0 in both load runs. Two reads of one
   profile would have to arrive within one miss (a few milliseconds) of each
   other. With round-robin over 10 profiles at 5 reads/s, how far apart are
   they? What `RATE` and `PROFILE_COUNT` would make a stampede likely?
3. With `TTL_MS=3000`, `cached` in the invariant table is below `PROFILE_COUNT`
   while with `TTL_MS=30000` it equals it. Why, and does a lower `cached` mean
   the cache and Postgres disagreed?

**Knob for a second run:** keep `TTL_MS=3000` and add `PROFILE_COUNT=4`.
Predict: each profile is read every 0.8 seconds, so one miss is followed by
three hits before the key expires, and the hit rate rises from about 50 % toward
75 %. Then try `PROFILE_COUNT=20`: reads 4 seconds apart, longer than the TTL,
so almost every read misses (hit rate near 0 %).

## What these numbers describe

- HTTP: `*_http_ms` (from each request's `http_req_duration`, re-recorded as
  `operation_ms` with its source), the failure rate, and dropped iterations.
- Domain: `avg_us` / `p50_us` / `p95_us` (the server's own timing of
  `core.ReadProfile`), `hit_rate_pct`, and the invariant table from
  `domain.json`.
- Limits: k6, the server, Postgres, and Valkey (both in Docker) share one
  laptop, so a miss costs two extra loopback round trips (the Postgres query and
  the SET). On a real network the miss penalty and the ratio would be larger.
  Profiles never change here, so this run cannot show stale reads; it shows only
  how often the cache is cold. A 60-second run says nothing about production
  capacity or uptime, and a smoke run's p95 is a wiring check, not an estimate.

See `perf/main.go` (adapter), `perf/k6.ts` (workload), and `perf/analyze.sql`
(lesson tables); the root [README](../../../../README.md) explains the `serve-`
/ `k6-` targets and what each run saves under `perf/results/`.
