# background-job-queue: HTTP and k6 follow-up

Spend about 10–20 minutes here after completing the base lesson. First predict:
**Blocking claims increase lock wait as requests overlap; skip-locked claims let independent jobs proceed.**

POST /operation submits one job and synchronously claims and completes one pending job. There are no separate background workers in this first extension. The claimed job can differ from the one submitted by that request.

## Run from the repository root

```sh
make up-postgres
make lab-k6-background-job-queue            # fresh fixtures, 10-second smoke, SQL, cleanup
make lab-k6-background-job-queue PROFILE=load# 60 seconds at 5 iterations/s
make lab-k6-background-job-queue PROFILE=load MODE=blocking
make analyze-k6-background-job-queue        # repeat analysis of the latest run
```

The root [README](../../../../README.md#two-ways-to-learn-each-lesson) explains
the profiles and settings, manual `serve-`/`k6-` use, what each run saves under
`perf/results/`, and what these laptop experiments cannot establish.

## Read the evidence

Open `perf/k6.ts` for the requests and checks, `scripts/perf/analyze.sql` for
the shared queries, and `perf/main.go` for the adapter.

claim_ms excludes the 100 ms processing delay. HTTP latency includes insertion, pool waits, claim, and processing. This version teaches transactional worker contention; it does not measure an asynchronous queue consumer or claim per-job end-to-end latency.

Discuss these three questions: Does claim_ms rise with overlap? How does changing POOL_SIZE change throughput? Are all inserted jobs done after traffic stops?

Change the one knob shown above, predict its effect, then rerun. Compare like
profiles and sample counts; a tiny smoke p95 is noisy.
