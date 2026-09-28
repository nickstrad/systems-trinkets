# sqlite-wal-lab: HTTP and k6 follow-up

Spend about 10–20 minutes here after completing the base lesson. First predict:
**WAL allows writes while a reader snapshot is held; DELETE mode causes writes to wait and fail.**

The server creates a temporary database, opens one reader snapshot for its lifetime, and exposes POST /operation for a timed insert. The busy timeout is 200 ms.

## Run from the repository root

```sh
# No backing service needed
make lab-k6-sqlite-wal-lab                  # fresh fixtures, 10-second smoke, SQL, cleanup
make lab-k6-sqlite-wal-lab PROFILE=load     # 60 seconds at 5 iterations/s
make lab-k6-sqlite-wal-lab PROFILE=load MODE=DELETE
make analyze-k6-sqlite-wal-lab              # repeat analysis of the latest run
```

The root [README](../../../../README.md#two-ways-to-learn-each-lesson) explains
the profiles and settings, manual `serve-`/`k6-` use, what each run saves under
`perf/results/`, and what these laptop experiments cannot establish.

## Read the evidence

Open `perf/k6.ts` for the requests and checks, `scripts/perf/analyze.sql` for
the shared queries, and `perf/main.go` for the adapter.

DELETE is a deliberate failing comparison: 503 responses and failed k6 thresholds are expected, and DuckDB still analyzes them. WAL has a default successful-write budget. Rows are capped at 100000 attempts; the held snapshot can retain WAL growth, so this is a bounded learning run.

Discuss these three questions: Does DELETE produce write failures? How does WAL latency change with concurrent writers? Why does a long-lived reader matter during a soak?

Change the one knob shown above, predict its effect, then rerun. Compare like
profiles and sample counts; a tiny smoke p95 is noisy.
