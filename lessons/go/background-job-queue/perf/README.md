# background-job-queue: HTTP and k6 follow-up

Spend about 10–20 minutes here after completing the base lesson.

**Hypothesis:** under concurrent HTTP traffic, rounds of workers that start
together still queue behind one another with `blocking` (`for update`): the k-th
claim in a round waits about k times the time a row stays locked, while
`skip_locked` claims stay near 1 ms. In both modes every job is claimed exactly
once.

**Predict first.** With two workers per round, the base lesson measured a
blocking claim of p50 52.46 ms and max 106.76 ms, against a skip_locked max of
0.88 ms (`blocking_slowdown` 121). Here k6 starts 5 rounds per second, two
requests each, with rounds of the two modes alternating in the same minute.
Write down your guess for the blocking p50 and max claim, and for the slowdown.
Then guess what happens with `WORKERS=1`: one request at a time, 10 per second
in total.

## What the server does

`POST /operation?variant=blocking|skip_locked` (400 for any other name) enqueues
one pending job, then runs `core.Work` once with that variant's claim query:
claim the oldest eligible job, hold it for `WORK_MS` (the base lesson's
`workTime`, 100 ms), mark it done, commit. The claimed job can be another
request's job. The response carries `elapsed_ms`, which is `Claim.Took`: the
claim query alone, the base lesson's `claim_ms`. This is a synchronous worker
per request, not an asynchronous queue: HTTP latency is completion latency, and
it includes the processing time.

Each variant has its **own queue**: its own private `perf_*` schema, table, and
connection pool (`POOL_SIZE` each, 8 by default). A shared table would mix the
experiment: a blocking claim would wait on rows that skip_locked claims hold,
and one variant could finish the other's jobs, so neither the claim times nor
"claimed exactly once" could be attributed to a variant. With two queues the
variants share the server, Postgres, and the minute, but not rows or
connections. `GET /stats` reports per variant: `enqueued`, `claims`,
`distinct_jobs` (distinct job ids the claims returned), `done` and `pending`
(counted in the table), `empty_claims` (409, no pending job), and
`failed_requests`.

**Why rounds.** k6's arrival-rate profiles start iterations evenly spaced. If
each iteration sent one request, a blocking claim would only wait when the next
request arrives before the previous one commits, which at 100 ms of work means
the blocking queue is already saturated. The base lesson's waits come from
workers that start together, so each k6 iteration is one base-lesson round:
`WORKERS` requests of one variant sent at once (`http.batch`, default 2, allowed
1..8). Iterations alternate the variants, so each gets half of `RATE`.

**Contention at the defaults.** A blocking queue serves one claim at a time,
each holding the row lock for about 108 ms (`WORK_MS` plus the insert, update,
commit, and HTTP). At `RATE=5` rounds per second, blocking gets 2.5 rounds, 5
claims per second: about 0.55 of its single lock is busy. Rounds do not overlap,
so the waits you see are the in-round queue from the base lesson. The knee is
near `RATE=9` with `WORKERS=2` (utilization 1): from there each round also waits
for the previous one and waits grow until Postgres's 5-second
`statement_timeout` fails requests. Skip locked serves up to `POOL_SIZE` claims
at once and is nowhere near its limit.

## Run from the repository root

```sh
make up-postgres                                                 # postgres://trinkets:trinkets@localhost:5432/trinkets
make lab-k6-background-job-queue                                 # 10-second smoke, 1 VU, both variants, SQL, cleanup
make lab-k6-background-job-queue PROFILE=load                    # the question: 60 s at 5 rounds/s, WORKERS=2
make lab-k6-background-job-queue PROFILE=load WORKERS=1          # same rate, one request per round
make analyze-k6-background-job-queue                             # repeat the SQL on the latest run
```

Every run starts a fresh server with two new schemas and drops them on shutdown;
`server.log` in the run directory names them. Both default runs together take
under three minutes.

## Read the evidence

The lesson's tables print after the shared ones, under
`== background-job-queue: the base lesson under concurrent traffic ==`.

1. **Base lesson table under load**: compare `p50_claim_ms` and `max_claim_ms`
   for `blocking` with the base lesson's `workers = 2` row (52.46 and 106.76).
   `p50_http_ms` sits about `WORK_MS` above the claim for both variants, because
   processing is inside the request.
2. **Blocking vs skip locked**: `blocking_slowdown` is the base lesson's
   question (`blocking_max_ms / skip_locked_max_ms`); `p95_slowdown` is the same
   ratio at p95, which one unlucky round cannot move.
3. **How busy the blocking queue was**: `single_lock_utilization` for `blocking`
   must stay below 1 for the comparison to mean the base lesson's in-round wait;
   its skip_locked row is only a reference.
4. **Invariant per variant**: `duplicate_claims` is 0, `done` equals `claims`,
   `k6_claims` equals `claims`, and `pending` is 0 unless a request failed. The
   teardown check fails the run otherwise.

HTTP measurements: `http_req_duration` p50/p95, the failure rate, and dropped
iterations. Domain measurements: `claim_ms` (the server's `Claim.Took`, tagged
by variant and workers) and `domain.json` (the counts above).

A verification run (20 s of `PROFILE=load`, 202 requests) measured blocking
p50/max claim 52.45/109.95 ms and skip_locked 0.54/1.82 ms, slowdown 60; with
`WORKERS=1` at the same rate both maxima were about 1 ms and the slowdown was

1. Run your own before reading these.

## Optional profiles

One line each; run them only when you want the question they ask.

- `PROFILE=stress`: about 2.5 minutes stepping 5, 10, 20 rounds/s. Blocking
  crosses its knee at the 10 step: where do its claims and failures start, and
  does skip_locked change at all? Expect the threshold failure.
  `make lab-k6-background-job-queue PROFILE=stress`
- `PROFILE=spike`: about 1 minute at 5 rounds/s with 10 s at 40. How long does
  the blocking backlog take to drain after the burst?
  `make lab-k6-background-job-queue PROFILE=spike`
- `PROFILE=soak`: 5 minutes at 5 rounds/s. Done rows stay in the table and there
  is no index on `status`: does claim p50 drift up for both variants as the
  claim query passes more done rows?
  `make lab-k6-background-job-queue PROFILE=soak`
- `WORKERS=4 RATE=2` or `WORKERS=8 RATE=1` reproduce the base lesson's other
  rows below saturation.

## Discuss

1. With `WORKERS=1` at the same rate, blocking looked like skip_locked. What
   does that say about when `for update` hurts in production: steady traffic, or
   bursts of workers that wake up together (a cron tick, a deploy, a queue that
   was empty)?
2. Past the knee, blocking waits include earlier rounds. Which number grows
   first, `p95_claim_ms` or `dropped_iterations`, and why does the pool (8
   connections held by waiting transactions) matter there?
3. Blocking never claimed a job twice even though every claimer waited on the
   same row. What does Postgres do when a waiting `for update` finds that row
   already `done` (see `knowledge/postgres-go.md`)?

**One knob for a second run:** `WORK_MS=50` with the default load. Predict:
blocking p50 and max claim roughly halve (about 27 and 55 ms), skip_locked stays
near 1 ms, so the slowdown shrinks, and the knee moves to about twice the rate.

## What this cannot establish

One laptop runs k6, the server, and Postgres (in Docker) over loopback. The
claim times describe lock waits in this experiment, not a production queue's
capacity or long-term behavior: real workers poll or listen asynchronously, jobs
vary in length, and the network adds latency to every statement a worker holds a
lock across.
