# sqlite-wal-lab: the same experiment under concurrent traffic

Spend 10–20 minutes here after the base lesson. This is the base lesson's own
comparison, journal mode `DELETE` vs `WAL` with a reader snapshot held open,
sent as HTTP requests from k6 instead of three trials per mode from `main.go`.

**Hypothesis:** with a reader holding a snapshot on each database, every
`DELETE` write still waits out the busy timeout and fails with `SQLITE_BUSY`
(locked share about 100 %), every `WAL` write still succeeds in well under a
millisecond (locked share 0 %), and in each database the rows equal the seed row
plus the successful writes.

**Predict first.** Your base lesson measured **DELETE: 3 of 3 writes locked
after about 2027 ms** (its busy timeout was 2000 ms) and **WAL: 3 of 3 ok in
about 0.13 ms**. Here the busy timeout is 250 ms, and k6 sends 5 writes per
second, alternating the two modes. Write down your guess for each mode's locked
share, `p50_ms` write time, and HTTP `p50_http_ms`. Does a second writer
arriving while a `DELETE` writer is still waiting change anything?

## What the adapter does

- At startup the server creates two databases in one temporary directory,
  `DELETE.db` and `WAL.db`, with `core.DSN(path, mode, BUSY_MS)` and
  `core.Schema`, then opens a reader with `core.OpenSnapshot` on each and
  **holds it for the server's lifetime**. Every write therefore lands while a
  reader snapshot is open, as the base runner's one write does. A snapshot per
  request would also work in `WAL`, but in `DELETE` a writer waiting for its
  exclusive lock blocks new readers, so the next request's snapshot would fail
  as a setup error instead of the write failing as the outcome.
- `POST /operation?variant=DELETE|WAL` calls `core.WriteEvent` once on that
  mode's database. Any other or missing variant answers 400; there is no
  default. A locked write is the lesson's result, not a broken request: it
  answers **200** with `outcome: "locked"` and `error` set to the SQLite error
  text (the `result` column of `measurements.csv`); a successful write answers
  `outcome: "ok"`. Other write errors answer 500, setup errors 503.
- `elapsed_ms` is `WriteResult.Elapsed`: lock wait plus the insert, timed the
  way `main.go` times it. Waiting for a pool connection and preparing the
  statement stay outside it, and inside HTTP time. k6 records it as `write_ms`,
  tagged by variant and outcome; the SQL counts the locked share from the
  `outcome` tag.
- Each database's pool has `POOL_SIZE` (8) writer connections plus one for the
  held snapshot, all opened and warmed at startup. A new SQLite connection needs
  a shared lock to run its pragmas and read the schema, and a waiting `DELETE`
  writer blocks new shared locks; without the warm-up, overlapping `DELETE`
  requests failed inside `WriteEvent`'s setup instead of in the write.
- `GET /stats` reports per mode: `journal_mode` (what SQLite says it is),
  `writes_ok`, `writes_locked`, `writes_failed` (every write counts in exactly
  one), `seed_rows`, `expected_rows` (seed + ok), `rows` (a fresh count),
  `snapshot_rows` (what the held reader still sees), and `wal_bytes`.
- The server removes its temporary directory on shutdown; its log prints the
  path and `removed <path>`. Requests are capped at 10000 per server because the
  held reader stops checkpoints, so the `WAL` file only grows.

## Run from the repository root

No backing service is needed.

```sh
make lab-k6-sqlite-wal-lab                  # 10-second smoke: wiring and invariant
make lab-k6-sqlite-wal-lab PROFILE=load     # 60 s at 5 writes/s: answers the question
make analyze-k6-sqlite-wal-lab              # repeat the SQL on the latest run
```

`BUSY_MS=250` is the default so the run passes the shared 1500 ms p95 budget and
the `DELETE` lock stays below saturation at `RATE=5`: about 2.5 `DELETE` writes
per second, each holding the write lock for about 0.25 s, so one arrives every
400 ms and each has finished before the next.

## Read the tables

The shared tables print first (the server settings, with `BUSY_MS` and
`POOL_SIZE` for this run, then latency, failures, dropped iterations, 10-second
buckets, custom metrics). The shared p50 mixes both modes, so skip it. The
lesson's tables follow the `== sqlite-wal-lab ==` banner:

1. **Base lesson table under load**: start here. It has the base table's shape:
   `variant` is `mode`, `outcome` is `result`, `writes` is `trials`. Compare
   `DELETE locked p50_ms` with `BUSY_MS`, and `WAL ok p50_ms` with your base
   0.13 ms.
2. **HTTP time beside write time**: `share_pct` should be about 50 for each
   mode, and `requests` should sum to the shared "Operation HTTP count".
   `p50_http_minus_write_ms` is what HTTP, JSON, and pool wait add.
3. **The question**: `delete_locked_pct` beside `wal_locked_pct`, and
   `delete_p50_write_ms` beside `wal_p50_write_ms`.
4. **Invariant per mode**: `difference` must be 0 (`rows` = `expected_rows`),
   `writes_failed` must be 0, `writes_locked` should match the locked `writes`
   in the base table, and `snapshot_rows` must still be the seed count, 1.

The `DELETE` locked writes are the finding, not a broken run: they answer 200,
so `http_req_failed` stays 0 and the thresholds pass.

## Optional runs

One command each; the default traffic above stays under five minutes.

- `make lab-k6-sqlite-wal-lab PROFILE=load BUSY_MS=2000`: the base lesson's busy
  timeout. Expect every `DELETE` write locked at about 2020 ms, the shared p95
  over the 1500 ms budget, `k6` exit 99, and Make exit 2 (the SQL still runs).
  Add `P95_MS=2500` to make it pass.
- `make lab-k6-sqlite-wal-lab PROFILE=stress`: about 2.5 minutes stepping from 5
  to 20 writes/s. From 10 writes/s, a `DELETE` write arrives every 200 ms,
  before the previous one has given up. Does `DELETE p95_ms` rise above
  `BUSY_MS`, and does `WAL p95_ms` move at all?
- `make lab-k6-sqlite-wal-lab PROFILE=spike`: about 1 minute with a 10-second
  burst at 40 writes/s: 20 `DELETE` writers per second, each waiting 250 ms,
  against 8 pool connections. Watch `p50_http_minus_write_ms` for `DELETE`.
- `make lab-k6-sqlite-wal-lab PROFILE=soak`: 5 minutes at 5 writes/s. Watch
  `wal_bytes` in the invariant table; add `DURATION_S=1800` for 30 minutes.
- `RATE=<n>` sets the arrival rate for any profile, `MAX_VUS` caps in-flight
  requests, `DURATION_S` shortens or lengthens a run.

## Questions only this lesson raises

1. Eight `DELETE` writers can overlap while one reader holds its snapshot. Does
   each wait its own `BUSY_MS`, or do they queue behind each other and wait
   longer? Use `max_ms` for `DELETE` in the base table, and explain it with the
   lock each writer is waiting for.
2. `snapshot_rows` stays 1 while `WAL` `rows` grows past 100. Which file do the
   new rows live in, and why can SQLite not copy them into `WAL.db` (checkpoint)
   while the reader is open? Compare `wal_bytes` / `writes_ok` with the
   4096-byte page size.
3. Your base `WAL` write took about 0.13 ms; here `WAL p50_ms` is probably
   higher even with nothing else writing. The base runner used a fresh database
   per trial and wrote once. What differs here: the pool connection, the growing
   `WAL` file, or the idle gap between writes (see the pipelining-work
   follow-up)?

**Knob for a second run:**
`make lab-k6-sqlite-wal-lab PROFILE=load
BUSY_MS=1000`. Predict: `DELETE p50_ms`
moves to about 1000 ms and the locked share stays 100 %; `WAL` does not change,
because a `WAL` writer never waits for a reader.

## What these numbers describe

- HTTP: `p50_http_ms` / `p95_http_ms` (from `http_req_duration`), the failure
  rate, and dropped iterations. Dropped iterations mean k6 ran out of virtual
  users, not that the server failed.
- Domain: `write_ms` (the server's timing of the write), the locked share, and
  the invariant table from `domain.json`.
- Limits: k6 and the server share one laptop, and SQLite runs in the server
  process on a local disk. Here the reader never lets go; a real reader does,
  and then `DELETE` writers succeed. That window is what this run cannot show. A
  60-second run says nothing about production capacity, and a smoke run's p95 is
  a wiring check, not an estimate.

See `perf/main.go` (adapter), `perf/k6.ts` (workload), and `perf/analyze.sql`
(lesson tables); the root [README](../../../../README.md) explains `serve-` /
`k6-` targets and what each run saves under `perf/results/`.
