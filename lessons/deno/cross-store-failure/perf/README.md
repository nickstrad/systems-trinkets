# cross-store-failure: the same experiment under concurrent traffic

Spend 10–20 minutes here after the base lesson. This is the base lesson's
`ordering` experiment, `put_then_insert` vs `insert_then_put` vs
`intent_then_put`, sent as HTTP uploads from k6 instead of one `mapConcurrent`
loop per order in `main.ts`. As in `main.ts`, 1 in 10 uploads of each order dies
right before its last write. When the traffic stops, the teardown repairs each
order with its own reconciler.

**Hypothesis:** under concurrent traffic, each write order still leaves only its
own failure class: orphan objects for `put_then_insert`, dangling rows for
`insert_then_put`, pending rows for `intent_then_put`. After repair, all three
are consistent. The intent order keeps every upload, and the other two keep only
uploads that did not crash. The pending reconciler checks about 10× fewer items
than the full scans, but it pays more for each item it checks.

**Predict first.** Your base lesson measured this for 100 uploads per order
(`measurements.csv`, `objects = 100`): 10 orphans, 10 dangling rows, and 10
pending rows; 90, 90, and 100 uploads kept; repair in 5.1 ms (100 checked), 3.9
ms (90 checked), and 5.7 ms (10 checked, about 570 µs per checked item). The
recommended run below sends 5 uploads per second for 60 seconds, round-robin
over the three orders: about 100 per order, arriving on a clock rather than one
after another. Write down your guess for each order's `orphan_objects`,
`dangling_rows`, `pending_rows`, `objects_kept`, `reconcile_ms`, and
`us_per_checked`. Also guess the median time of one upload (`p50_upload_ms`),
which the base lesson never measured.

## What the adapter does

- `POST /operation?variant=<mode>&crash=0|1` calls the core's
  `upload(mode, key, crash)` once. An unknown or missing variant, or a crash
  value other than `0` or `1`, answers 400. There is no default order. A
  simulated crash is a 200 with `crashed: true`, because it is the experiment,
  not a failure. A thrown store error is a 503 and counts as `failed`.
- `elapsed_ms` times the `upload()` call only. `main.ts` times only the repair;
  the per-upload time is new here. k6 records it as the `upload_ms` metric,
  tagged by variant and crash.
- Each order has its own Postgres schema (`perf_<token>_<mode>`), its own S3
  prefix (`perf/<token>/<mode>/` in bucket `lab`), and its own
  `createOperations` instance. The core's metadata table has a fixed name, and
  its prefix scopes only S3 listings. In a shared table, `measure()` would count
  another order's rows as dangling, and the `insert_then_put` repair would
  delete them.
- `POST /repair` does what one `ordering` row of `main.ts` does, for each order:
  `measure()`, then a timed `repair[mode]()`. It answers 409 while uploads are
  in flight. That case is the base lesson's `grace` experiment, which this
  follow-up does not repeat.
- `GET /stats` reports each order's `uploads`, `crashed`, and `failed`,
  `expected_kept` (`uploads` for the intent order, whose repair finishes a
  crashed upload; `uploads - crashed` for the other two, whose repair deletes
  it), the current `measure()` (objects, rows, orphans, dangling, pending), and
  the last repair's `before_repair` state and cost (`checked`, `reconcile_ms`).
  The runner saves the final `/stats` as `domain.json`, so one file holds the
  state both before and after repair. k6 and the SQL both compare against the
  server's `expected_kept`.
- At startup the server opens all `POOL_SIZE` Postgres connections of each order
  (`select 1`, no rows), so the first uploads do not pay a connect inside
  `upload_ms`. The lab pool keeps node-postgres's 10-second idle timeout, so
  after a longer lull an upload can re-dial inside the timed window.
- On shutdown, the server deletes its objects and drops its three schemas.

## Run from the repository root

```sh
make up-postgres up-seaweedfs                     # postgres://trinkets:trinkets@localhost:5432/trinkets
                                                  # S3 http://localhost:8333 (key trinkets, secret trinkets-secret)
make lab-k6-cross-store-failure                   # 10-second smoke: wiring and invariant
make lab-k6-cross-store-failure PROFILE=load      # 60 s at 5 uploads/s: answers the question
make analyze-k6-cross-store-failure               # repeat the SQL on the latest run
```

`RATE=5` keeps every order far below saturation. One upload takes a few
milliseconds, so about one upload is in flight at a time. Raise `RATE` to add
concurrency. Each order accepts at most 10000 uploads per server.

## Read the tables

The shared tables print first: the server settings (`POOL_SIZE` is Postgres
connections per order), latency, failures, dropped iterations, the 10-second
buckets, and the custom metrics. The lesson's tables follow the
`== cross-store-failure ==` banner.

1. **Base lesson table under load**: start here. It has the same columns as the
   first table of the base `analyze.sql`. Compare `orphan_objects`,
   `dangling_rows`, and `pending_rows` with your 10 / 10 / 10. Each order should
   show only its own column, equal to its `crashed`. Then check that
   `objects_kept = rows_kept = expected_kept` and that `consistent` is true.
2. **Reconciler cost**: the base lesson's second table. Compare `reconcile_ms`
   and `us_per_checked` with your 100-upload rows. The repair runs after the
   traffic stops, so this cost is not measured under load.
3. **Upload latency by variant and crash**: `p50_upload_ms` is the server's time
   for one upload. `p50_http_ms` is what k6 saw. The difference is what HTTP
   adds. A crashed upload skips its last write.
4. **Share by variant**: each order should have about 33 %, and the counts
   should add up to the shared "Operation HTTP count".
5. **The question**: `intent_kept` beside `put_first_kept` and
   `insert_first_kept`, then `intent_checked` beside `put_first_checked`, and
   `intent_us_per_checked` beside `put_first_us_per_checked`.

## Optional profiles

Each profile is one command. The default traffic above stays under five minutes.

- `make lab-k6-cross-store-failure PROFILE=stress`: about 2.5 minutes, from 5 to
  20 uploads/s. Does `intent_then_put`, with two Postgres writes per upload,
  reach its knee before the single-write orders?
- `make lab-k6-cross-store-failure PROFILE=spike`: about 1 minute, with a
  10-second burst at 40 uploads/s. Does the burst change what the repair keeps?
  (It should not. Why?)
- `make lab-k6-cross-store-failure PROFILE=soak`: 5 minutes at 5 uploads/s,
  about 500 uploads per order. Does `reconcile_ms` for the two full scans grow
  with the store while the pending repair stays flat?
- `RATE=<n>` sets the arrival rate, `MAX_VUS` caps in-flight requests,
  `DURATION_S` shortens or lengthens a run, and `POOL_SIZE` sets the server's
  connections per order.

## Questions only this lesson raises

1. The k6 upload latency now shows the price of each order. `intent_then_put`
   makes three writes (insert, PUT, update), and the other orders make two. Is
   its `p50_upload_ms` about 1.5× theirs? What does that extra write buy you in
   the first table?
2. Uploads here arrive on a clock, so at a higher `RATE` they overlap. Yet the
   before-repair counts equal `crashed` exactly. Why does concurrency not create
   extra orphans or dangling rows here, when in the base lesson's `grace`
   experiment it did? (Hint: when does `/repair` run, and what does it refuse to
   do?)
3. `put_then_insert` and `insert_then_put` both keep `uploads - crashed`. But
   for users, the dangling rows before repair are worse than the orphans. Why?
   Which of the two failure classes could a reader of the metadata table
   actually hit during this run?

**Knob for a second run:**
`make lab-k6-cross-store-failure PROFILE=load CRASH_EVERY=2`. Predict: the
before-repair columns and the pending repair's `checked` grow about 5×,
`put_first_kept` and `insert_first_kept` fall to about half of `uploads`,
`intent_kept` stays at `uploads`, and `intent_reconcile_ms` grows about 5× while
the full scans barely change.

## What these numbers describe

- HTTP: `p50_http_ms` / `p95_http_ms` (from `http_req_duration`), the failure
  rate, and dropped iterations. Dropped iterations mean k6 ran out of virtual
  users, not that the server failed.
- Domain: `p50_upload_ms`, and everything from `domain.json`: the before-repair
  and after-repair counts and the reconciler cost.
- Limits: k6, the server, Postgres, and SeaweedFS all run on one laptop, over
  loopback. The stores never fail here; every crash is simulated at the one
  point that matters. A real process can also die mid-request, lose a network
  partition, or run a repair while uploads are in flight (the base lesson's
  `grace` experiment). A 60-second run says nothing about production capacity or
  long uptime. A smoke run's p95 is a wiring check, not an estimate.

See `perf/server.ts` (adapter), `perf/k6.ts` (workload), and `perf/analyze.sql`
(lesson tables). The root [README](../../../../README.md) explains the `serve-`
/ `k6-` targets and what each run saves under `perf/results/`.
