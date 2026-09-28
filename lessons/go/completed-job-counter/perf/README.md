# completed-job-counter: HTTP and k6 follow-up

Spend about 10–20 minutes here after completing the base lesson. First predict:
**Idempotent delivery leaves one increment per unique event even when each event is sent twice.**

Each k6 iteration sends two POST /operation requests with the same event ID. IDs cycle through 0..9999, bounding fixture growth.

## Run from the repository root

```sh
make up-postgres
make lab-k6-completed-job-counter           # fresh fixtures, 10-second smoke, SQL, cleanup
make lab-k6-completed-job-counter PROFILE=load# 60 seconds at 5 iterations/s
make lab-k6-completed-job-counter PROFILE=load MODE=naive
make analyze-k6-completed-job-counter       # repeat analysis of the latest run
```

The root [README](../../../../README.md#two-ways-to-learn-each-lesson) explains
the profiles and settings, manual `serve-`/`k6-` use, what each run saves under
`perf/results/`, and what these laptop experiments cannot establish.

## Read the evidence

Open `perf/k6.ts` for the requests and checks, `scripts/perf/analyze.sql` for
the shared queries, and `perf/main.go` for the adapter.

RATE counts iterations: this workload sends twice as many operation requests. delivery_applied measures whether the duplicate was applied. A skipped response omits total because the core does not read the current total on a duplicate.

Discuss these three questions: Does total equal seen for idempotent mode? What changes under naive mode? Does contention increase latency as arrivals rise?

Change the one knob shown above, predict its effect, then rerun. Compare like
profiles and sample counts; a tiny smoke p95 is noisy.
