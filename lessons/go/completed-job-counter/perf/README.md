# completed-job-counter: HTTP and k6 follow-up

Spend 10–20 minutes here after the base lesson. This is the same experiment as
`main.go`, now under concurrent traffic: the two strategies from its
`strategies` table (`naive`, `idempotent`), the same `core.Apply`, the same
invariant, and the same three tables, with arrival rate and concurrency as new
axes.

**Hypothesis:** with events arriving concurrently and each delivered three
times, `naive` still counts every delivery (overcount = 2 per event) and
`idempotent` still counts each event once (overcount 0, `total` equal to the
`job_seen` rows). Idempotent retries stay cheaper than its first delivery.

**Predict first.** The base lesson delivered 100 events three times each on one
connection and measured: naive `actual_count` 300, `overcount` 200; idempotent
`actual_count` 100, `overcount` 0; p50 per delivery 0.69 ms (naive, first and
retry), 0.88 ms (idempotent first), 0.58 ms (idempotent retry). Write down your
guess for each at 5 events per second (15 deliveries per second) with up to 32
events in flight over an 8-connection pool.

## How the experiment maps to the base lesson

- One k6 iteration is one event. It picks a variant (iterations alternate
  `naive` and `idempotent`, so both get the same share of the same minute) and
  delivers the event `DELIVERIES` times in a row (the server's setting, which k6
  reads from `/health` in `setup()`), like `main.go`'s `attempt` loop:
  `POST /operation?variant=<name>&id=<id>&attempt=<n>`.
- The event id is the iteration number plus one. k6 numbers iterations uniquely
  across VUs, so an event's only duplicates are its own retries.
- The adapter (`perf/main.go`) calls `core.Apply` once per request in a private
  `perf_*` schema and times only that call (`elapsed_ms`, the base lesson's
  `latency_ms`). It acquires a pool connection before starting the clock, so
  waiting for a free connection shows in HTTP time, not in `elapsed_ms`, as the
  base lesson's single connection never waited.
- `GET /stats` reads `job_counter` and `job_seen`, and adds the adapter's tally
  of confirmed deliveries and distinct events. The runner saves it as
  `domain.json`.

## Run from the repository root

```sh
make up-postgres                     # postgres://trinkets:trinkets@localhost:5432/trinkets
make lab-k6-completed-job-counter    # 10-second smoke: wiring and invariants
make lab-k6-completed-job-counter PROFILE=load   # the run that answers the question: 60 s at RATE=5 events/s
make analyze-k6-completed-job-counter            # repeat the SQL on the newest run
```

`RATE` counts events (iterations); each sends `DELIVERIES` requests, so the
default `load` sends 15 requests per second. At that rate neither variant is
near saturation; raise `RATE` to look for the knee.

## Read the evidence

The shared tables come first: the server settings (`DELIVERIES` and `POOL_SIZE`
for this run), then latency by status, failures, dropped iterations, request
count, and 10-second buckets. The lesson's tables follow from
`perf/analyze.sql`:

1. **Share of operation requests by variant**: about 50/50; the two counts sum
   to the shared operation count.
2. **Base table 1, by variant.** Look here first. Compare `overcount` for
   `naive` and `idempotent`, and `actual_count` against `unique_events`.
3. **Base table 2, by attempt**: `increments` vs `duplicates_ignored` for
   attempts 1, 2, 3.
4. **Base table 3, first vs retry**: `p50_ms`/`p95_ms` is `core.Apply` time, the
   same span the base lesson measured; `p50_http_ms` is what k6 saw.
5. **The question**: `naive_overcount` beside `idempotent_overcount`, and the
   overcount per event (`DELIVERIES - 1` for naive, 0 for idempotent).
6. **The invariant**: `expected_total` (naive: confirmed deliveries; idempotent:
   unique events) vs `actual_total` (the `job_counter` row), and `seen` (the
   `job_seen` rows) for idempotent. `difference` must be 0.

Source: `perf/k6.ts` (requests, checks, `core_ms` and `delivery_applied` tagged
by variant and attempt), `perf/main.go` (adapter), `perf/analyze.sql`.

## Optional profiles (not run by default)

- `PROFILE=stress` (about 2¼ minutes: rate ×1, ×2, ×4, back to ×1): at what rate
  does the 8-connection pool make `p95_http_ms` for first deliveries climb (the
  pool wait is outside `p95_ms`), and does idempotent (two statements) reach it
  before naive (one)?
- `PROFILE=spike` (about 1 minute, ×8 burst): does a burst change the overcount?
  It must not; only latency may move.
- `PROFILE=soak` (5 minutes): does `job_seen` growth change idempotent latency
  over time?

Each takes `RATE=<n>` and `DURATION_S=<s>`; the p95 budget is `P95_MS` (1500 ms,
illustrative).

## Discuss

1. Idempotent's retry does an `insert ... on conflict do nothing` and no
   `update`; naive's retry repeats exactly the work of its first delivery. So
   naive is the control: in the base lesson its first and retry medians were
   equal. Here retries follow their first delivery back to back, while each
   first delivery follows an idle gap. If naive's retry is now faster than its
   first delivery, how much of idempotent's first-vs-retry gap is the strategy,
   and how much is the gap?
2. Every naive delivery updates the same `job_counter` row. Under concurrency
   those updates queue on one row lock; idempotent updates it only on first
   delivery. Which variant's `p95_ms` should grow faster with `RATE`, and what
   does that say about counting at the row?
3. Deliveries of one event here arrive one after another. What would change if
   two deliveries of the same event raced (two consumers)? Which statement in
   `core.Idempotent` makes the second one wait, and does the total stay correct?

**Knob for a second run:**
`DELIVERIES=5 make lab-k6-completed-job-counter
PROFILE=load DURATION_S=20`.
Predict: naive overcount per event rises to 4, idempotent stays 0, and
idempotent's overall median falls because a larger share of its deliveries are
cheap retries.

## What this does and does not show

HTTP numbers (`p50_http_ms`, the shared latency tables) describe this adapter on
loopback. Domain numbers (`overcount`, `actual_total`, `seen`) describe the
strategies and do not depend on the machine. A laptop run with Postgres in
Docker cannot establish production capacity or uptime; smoke numbers verify
wiring, not performance. Smoke sleeps 100 ms between requests, and idle gaps
make each transaction slower than back-to-back calls, so compare latency only
between runs of the same profile.
