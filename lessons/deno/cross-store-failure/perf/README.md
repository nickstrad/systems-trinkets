# cross-store-failure: HTTP and k6 follow-up

Spend about 10–20 minutes here after completing the base lesson. First predict:
**Intent records let reconciliation repair incomplete uploads after injected failures.**

POST /operation performs one upload; every tenth iteration simulates a crash before the final step. Teardown runs POST /repair after uploads finish, then checks /stats. Repeating a workload against a manually started server reuses object IDs, so restart that server between workloads.

## Run from the repository root

```sh
make up-postgres up-seaweedfs
make lab-k6-cross-store-failure             # fresh fixtures, 10-second smoke, SQL, cleanup
make lab-k6-cross-store-failure PROFILE=load# 60 seconds at 5 iterations/s
make lab-k6-cross-store-failure PROFILE=load MODE=put_then_insert
make analyze-k6-cross-store-failure         # repeat analysis of the latest run
```

The root [README](../../../../README.md#two-ways-to-learn-each-lesson) explains
the profiles and settings, manual `serve-`/`k6-` use, what each run saves under
`perf/results/`, and what these laptop experiments cannot establish.

## Read the evidence

Open `perf/k6.ts` for the requests and checks, `scripts/perf/analyze.sql` for
the shared queries, and `perf/server.ts` for the adapter.

A simulated crash is labelled in a 200 response because it is intentional experiment behavior, not a network error. The default mode is intent_then_put. At most 10000 uploads are accepted per server; longer experiments must choose a rate within that budget. Repair duration appears in raw k6 metrics under name=repair, separate from operation latency.

Discuss these three questions: Are pending/orphan/dangling counts zero after repair? How does changing mode affect repair work? How does upload latency change with arrivals?

Change the one knob shown above, predict its effect, then rerun. Compare like
profiles and sample counts; a tiny smoke p95 is noisy.
