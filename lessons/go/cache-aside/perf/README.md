# cache-aside: HTTP and k6 follow-up

Spend about 10–20 minutes here after completing the base lesson. First predict:
**Cache expiry increases database reads; most requests between expirations hit Valkey.**

GET /operation reads one isolated Ada profile. The cache starts cold and expires after one second.

## Run from the repository root

```sh
make up-postgres up-valkey
make lab-k6-cache-aside                     # fresh fixtures, 10-second smoke, SQL, cleanup
make lab-k6-cache-aside PROFILE=load        # 60 seconds at 5 iterations/s
make lab-k6-cache-aside PROFILE=load CACHE_TTL_MS=100
make analyze-k6-cache-aside                 # repeat analysis of the latest run
```

The root [README](../../../../README.md#two-ways-to-learn-each-lesson) explains
the profiles and settings, manual `serve-`/`k6-` use, what each run saves under
`perf/results/`, and what these laptop experiments cannot establish.

## Read the evidence

Open `perf/k6.ts` for the requests and checks, `scripts/perf/analyze.sql` for
the shared queries, and `perf/main.go` for the adapter.

cache_miss is a fraction; domain.json records hits and misses. Cold requests remain in the latency samples.

Discuss these three questions: Do misses cluster near expiration? How does the miss fraction change with TTL? Does p95 move when misses increase?

Change the one knob shown above, predict its effect, then rerun. Compare like
profiles and sample counts; a tiny smoke p95 is noisy.
