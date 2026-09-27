# A small k6 → CSV → DuckDB path

Generate the lesson's SQL from real smoke output. k6 CSV contains multiple metric
samples per request, not a request table. Filter by `metric_name` before counting
or aggregating. Inspect the actual header and preserve the needed system tags.
The native output command is `k6 run --out csv=metrics.csv script.js`.
[CSV output documentation](https://grafana.com/docs/k6/latest/results-output/real-time/csv/)

Prefer one output directory per run. The following SQL assumes it is executed
from that directory, containing `metrics.csv`; a generated walkthrough must give
the exact command to reach the directory and load `perf/analyze.sql`.

```sql
create table samples as
select * from read_csv('metrics.csv', header = true, all_varchar = true);

.print 'HTTP latency by scenario, operation, and status (milliseconds)'
select scenario, name, status,
       count(*) as samples,
       round(median(cast(metric_value as double)), 2) as p50_ms,
       round(quantile_cont(cast(metric_value as double), 0.95), 2) as p95_ms
from samples
where metric_name = 'http_req_duration'
group by all
order by all;

.print 'HTTP failure rate by scenario and operation'
select scenario, name,
       count(*) as samples,
       round(100 * avg(cast(metric_value as double)), 2) as failed_pct
from samples
where metric_name = 'http_req_failed'
group by all
order by all;

.print 'Iterations the load generator could not start'
select coalesce(sum(cast(metric_value as double)), 0) as dropped_iterations
from samples
where metric_name = 'dropped_iterations';
```

This is a starting shape, not a requirement to copy all three queries. Adapt it
to the hypothesis and exclude readiness/polling requests by stable `name` tags.
Use `http_reqs` values for HTTP counts; `checks` counts assertions, not requests.
Treat an empty duration or failure table as missing evidence, not a passing run.

`http_req_duration` excludes connection establishment and DNS time. Failure rate
follows k6's configured expected-response policy; body checks are separate.
Dropped iterations indicate unsent work, not HTTP errors. All can matter in the
same run. [Metric definitions](https://grafana.com/docs/k6/latest/using-k6/metrics/reference/)

For stress/spike, substitute or add a short time-bucket query so a whole-run
percentile does not hide recovery. Set the CSV timestamp format explicitly if
using timestamps. Include zero-traffic buckets when calculating rates; do not
divide by only the seconds that contain samples. Keep the configured arrival
schedule beside the results to compare offered demand with achieved throughput.
Do not average percentiles across buckets or runs; recompute from raw samples.

Use p50/p95 with sample counts by default. A tiny smoke run does not support a
stable tail estimate. Label warm-up, cold-start, failures, and successful results
instead of blending them into a single unexplained number. Retain failed requests.

For a queue, use a separate final domain check for accepted/completed/pending work.
If the lesson needs completion latency, instrument completion or use bounded
polling and explain its resolution. HTTP latency alone cannot answer that question.

Consult the installed version's official documentation if an API differs:

- [Checks and thresholds](https://grafana.com/docs/k6/latest/using-k6/thresholds/)
- [Arrival models](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/)
- [Ramping arrival rate](https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/ramping-arrival-rate/)

Documentation reviewed 2026-09-27. The examples are guidance for generated
scaffolding; verify the final lesson analysis against its actual k6 output.
