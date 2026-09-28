# pipelining-work: the same experiment under concurrent traffic

Spend 10–20 minutes here after the base lesson. This is the base lesson's own
comparison, `sequential` vs `pipeline`, sent as HTTP requests from k6 instead of
five back-to-back runs from `main.go`.

**Hypothesis:** under concurrent traffic, a sequential batch of 200 INCRs is
still at least an order of magnitude slower than a pipelined one (the base
lesson's `sequential_to_pipeline_ratio`), HTTP adds about the same fixed cost to
both, so the HTTP ratio is smaller than the core ratio, and each variant's
counters still sum to its confirmed batches times the batch size.

**Predict first.** Your base lesson measured a median batch of **35.4 ms
sequential** and **0.28 ms pipelined** (ratio about 126) with nothing else
running. Here k6 sends 5 batches per second, alternating the two variants, with
up to 32 in flight. Write down your guess for each variant's `p50_core_ms`, its
`p50_batch_ms` over HTTP, and the two ratios.

## What the adapter does

- `POST /operation?variant=sequential|pipeline` calls `core.IncrementSequential`
  or `core.IncrementPipelined` once on 200 counters. Any other or missing
  variant answers 400; there is no default.
- `elapsed_ms` in the response times only the core call, the way `main.go` times
  `variant.op`. k6 records it as the `core_ms` metric, tagged by variant.
- Each variant owns its own key set (`perf:pipeline:<token>:<variant>:<i>`), so
  the invariant is checked per variant. `GET /stats` reads each set back with
  `core.Sum` (one MGET) and reports `batches`, `failed_batches`, `expected_sum`
  (batches × `BATCH_SIZE`), and `actual_sum`. Counters accumulate over the run
  instead of being reset per batch as in `main.go`.
- The server deletes its keys on shutdown.

## Run from the repository root

```sh
make up-valkey                                        # redis://localhost:6379
make lab-k6-pipelining-work                           # 10-second smoke: wiring and invariant
make lab-k6-pipelining-work PROFILE=load              # 60 s at 5 batches/s: answers the question
make analyze-k6-pipelining-work                       # repeat the SQL on the latest run
```

The load profile's default `RATE=5` keeps the slower variant well below
saturation: about 2.5 sequential batches per second, each around 60 ms, so
rarely more than one is in flight. Raise `RATE` to look for the knee.

## Read the tables

The server settings header prints first (`BATCH_SIZE` for this run), then the
shared tables (latency, failures, dropped iterations, the 10-second buckets,
custom metrics). The lesson's tables follow the `== pipelining-work ==` banner:

1. **Base lesson table under load**: start here. Compare `p50_core_ms` with the
   base lesson's `p50_batch_ms` (35.4 and 0.28), then compare `p50_core_ms` with
   `p50_batch_ms` on the same row: the difference is what HTTP, JSON, and the
   server add.
2. **Share by variant**: the two rows should be about 50 % each and sum to the
   shared "Operation HTTP count".
3. **The question**: `core_sequential_to_pipeline_ratio` is the number to
   compare with your base `sequential_to_pipeline_ratio`;
   `http_sequential_to_pipeline_ratio` is what an HTTP client sees.
4. **Invariant per variant**: `expected_sum` and `actual_sum` must match, and
   `difference` must be 0.

## Optional profiles

One command each; the default traffic above stays under five minutes.

- `make lab-k6-pipelining-work PROFILE=stress`: about 2.5 minutes stepping from
  5 to 20 batches/s. Does sequential `p95_core_ms` rise before pipeline's does,
  and at what step?
- `make lab-k6-pipelining-work PROFILE=spike`: about 1 minute with a 10-second
  burst at 40 batches/s. Which variant's p95 settles first after the burst in
  the 10-second buckets?
- `make lab-k6-pipelining-work PROFILE=soak`: 5 minutes at 5 batches/s. Do the
  medians drift while the counters grow? Add `DURATION_S=1800` for 30 minutes.
- `RATE=<n>` sets the arrival rate for any profile, `MAX_VUS` caps in-flight
  requests, `DURATION_S` shortens or lengthens a run.

## Questions only this lesson raises

1. Your base run measured 35 ms sequential with one batch at a time. At 5
   batches per second there is usually still only one batch in flight, yet
   `p50_core_ms` may be much higher. What does the base runner do between
   batches that k6 does not? Test it: `make serve-pipelining-work` in one
   terminal, then in another call
   `curl -s -X POST 'http://127.0.0.1:8080/operation?variant=sequential'` ten
   times back to back, then ten times with `sleep 0.1` between calls, and
   compare `elapsed_ms`.
2. The core ratio probably shrank. Did sequential get slower, did pipeline get
   slower, or both? Which of the two is more sensitive to anything that adds a
   fixed cost per batch, and why?
3. `actual_sum` comes from MGET, not from INCR replies. A `failed_batches` count
   above zero could make `difference` positive but never negative. Why, for each
   variant?

**Knob for a second run:**
`make lab-k6-pipelining-work PROFILE=load
BATCH_SIZE=1000`. Predict: sequential
`p50_core_ms` grows about 5× (five times the round trips), pipeline grows less
than 5× (one round trip, more bytes), so the ratio grows.

## What these numbers describe

- HTTP: `p50_batch_ms` / `p95_batch_ms` (from `http_req_duration`), the failure
  rate, and dropped iterations. Dropped iterations mean k6 ran out of virtual
  users, not that the server failed.
- Domain: `p50_core_ms` (the server's own timing of the core call) and the
  invariant table from `domain.json`.
- Limits: k6, the server, and Valkey (in Docker) share one laptop. The round
  trip that pipelining saves is short here; over a real network it is longer, so
  the ratio would be larger. A 60-second run says nothing about production
  capacity or long uptime, and a smoke run's p95 is a wiring check, not an
  estimate.

See `perf/main.go` (adapter), `perf/k6.ts` (workload), and `perf/analyze.sql`
(lesson tables); the root [README](../../../../README.md) explains `serve-` /
`k6-` targets and what each run saves under `perf/results/`.
