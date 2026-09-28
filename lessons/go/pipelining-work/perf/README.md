# pipelining-work: HTTP and k6 follow-up

Spend about 10–20 minutes here after completing the base lesson. First predict:
**At the same arrival rate, a pipelined batch answers far faster than a
sequential one, and neither loses an increment under concurrent requests.**

POST /operation increments the server's private set of counters (200 by
default) with the mode chosen at startup: `pipeline` sends every INCR in one
round trip, `sequential` waits for each reply. GET /stats reads the counters
back with MGET and compares their sum to the batches the server confirmed.

Write down your guess for the sequential p50 in milliseconds before running
it. The base lesson measured one batch at a time; here up to 32 virtual users
send batches at once.

## Run from the repository root

```sh
make up-valkey
make lab-k6-pipelining-work                         # pipeline mode, 10-second smoke, SQL, cleanup
make lab-k6-pipelining-work PROFILE=load            # 60 seconds at 5 batches/s, pipelined
make lab-k6-pipelining-work PROFILE=load MODE=sequential
make analyze-k6-pipelining-work                     # repeat analysis of the latest run
```

Every run starts a fresh server with its own key prefix, so counters begin at
zero; the server deletes its keys on shutdown. The invariant is checked in
teardown: `actual_sum` in Valkey must equal `batches * BATCH_SIZE`.

## Optional profiles

Each is one command; keep the defaults unless you are asking a new question.

- `PROFILE=stress`: about 2.5 minutes stepping from 5 to 20 batches/s with
  holds. Where does sequential p95 rise, and does k6 drop iterations?
- `PROFILE=spike`: about 1 minute at 5 batches/s with a 10-second burst at 40.
  How quickly does latency settle after the burst?
- `PROFILE=soak`: 5 minutes by default at 5 batches/s. Does p95 drift while
  the counters keep growing? Set `DURATION_S=1800` for a 30-minute extension.
- `RATE=20` raises the arrival rate of any profile; `MAX_VUS` caps concurrency.

The root [README](../../../../README.md#two-ways-to-learn-each-lesson) explains
manual `serve-`/`k6-` use, what each run saves under `perf/results/`, and
what these laptop experiments cannot establish.

## Read the evidence

Open `perf/k6.ts` for the request and checks, `scripts/perf/analyze.sql` for
the shared queries, and `perf/main.go` for the adapter.

HTTP measurements: `http_req_duration` p50/p95 for `operation` requests, the
failure rate, and dropped iterations. Domain measurements: the `increments`
custom metric (sum of confirmed INCRs seen by k6) and `domain.json`, which
records `batches`, `expected_sum`, and `actual_sum` from the server.

Discuss these three questions: How does the sequential-to-pipeline p50 ratio
under load compare with the base lesson's single-batch ratio? Did
`actual_sum` equal `expected_sum` in both modes, and why does MGET, not the
INCR replies, prove that? Which mode would first fall behind if RATE doubled,
and would k6 report it as latency or as dropped iterations?

Change one knob, predict its effect, then rerun: `BATCH_SIZE=1000` with
`MODE=sequential` multiplies round trips per request. Compare like profiles
and sample counts; a tiny smoke p95 is noisy.

This experiment cannot establish production capacity or long uptime: one
laptop runs the client, the server, and Valkey over loopback, so the
round-trip time it amortizes is far smaller than a real network's.
