# k6 CSV → DuckDB: the shapes the lesson SQL relies on

k6 writes one row per **metric sample**, not per request:
`k6 run --out csv=metrics.csv perf/k6.ts`. A request produces several rows
(`http_reqs`, `http_req_duration`, `http_req_failed`, and more), so filter by
`metric_name` and by the request `name` tag before counting or aggregating.
[CSV output documentation](https://grafana.com/docs/k6/latest/results-output/real-time/csv/)

Header, as written by k6 v2.3 (verified 2026-09-28):

```
metric_name,timestamp,metric_value,check,error,error_code,expected_response,
group,method,name,proto,scenario,service,status,subproto,tls_version,url,
extra_tags,metadata
```

The shared `scripts/perf/analyze.sql` loads this as `samples`, casting
`metric_value` to double and `timestamp` to bigint (`K6_CSV_TIME_FORMAT=unix`
is set by the runner). The lesson's `perf/analyze.sql` is appended after it
and runs in the same DuckDB session from the run directory, so it can read
`samples`, `settings.json`, and `domain.json` directly.

## Custom tags land in `extra_tags`

System tags (`name`, `scenario`, `status`, ...) have their own columns. Any
other tag on a request or a custom metric is joined into `extra_tags` as
`key=value&key=value`. Verified with a `Trend.add(v, {variant: "pipeline",
name: "operation"})`: the row had `name=operation` in its column and
`extra_tags=variant=pipeline`. The shared `scripts/perf/analyze.sql` defines
`tag(extra, key)` (an anchored `regexp_extract`, so `variant` never matches
`sub_variant`). Extract each tag once in a view and group by it:

```sql
.print 'Base lesson table under load: batch latency by variant (milliseconds)'
select tag(extra_tags, 'variant') as variant,
       count(*) as batches,
       round(median(metric_value), 3) as p50_batch_ms,
       round(quantile_cont(metric_value, 0.95), 3) as p95_batch_ms
from samples
where metric_name = 'http_req_duration' and name = 'operation'
group by all
order by all;
```

Reuse the alias for the lesson's question, in the named-group form so a
mistyped variant shows as NULL beside a value instead of vanishing:

```sql
.print 'Sequential vs pipeline: ratio of median HTTP batch latency'
with by_variant as (
  select tag(extra_tags, 'variant') as variant,
         metric_value
  from samples
  where metric_name = 'http_req_duration' and name = 'operation'
)
select round(median(metric_value) filter (where variant = 'sequential'), 3) as sequential_ms,
       round(median(metric_value) filter (where variant = 'pipeline'), 3) as pipeline_ms,
       round(sequential_ms / pipeline_ms, 2) as sequential_to_pipeline_ratio
from by_variant;
```

A custom metric tagged the same way splits the same way; a `Trend` of the
server's own `elapsed_ms` beside `http_req_duration` shows how much HTTP adds:

```sql
.print 'Core call time reported by the server, by variant (milliseconds)'
select tag(extra_tags, 'variant') as variant,
       count(*) as samples,
       round(median(metric_value), 3) as p50_core_ms
from samples
where metric_name = 'core_ms'
group by all
order by all;
```

## Settings and final state are JSON files beside the CSV

The shared SQL prints `settings.json` first (`select fixtures,
unnest(server) from read_json('settings.json')`); the lesson adds a
`.print` legend for its knobs. The final `/stats` is `domain.json`:

```sql
.print 'Invariant: what the server confirmed vs what the store holds'
select mode, batches, expected_sum, actual_sum,
       actual_sum - expected_sum as difference
from read_json('domain.json');
```

`unnest(server)` spreads the `/health` settings map into one column per
setting (verified against a run's `settings.json`). When `/stats` reports
per variant as `{variants: [{variant, ...}]}`, `select d.batch_size,
unnest(d.variants, recursive := true) from read_json('domain.json') d`
spreads the array into one row per variant with one column per field.
Adapt the column names to what the adapter's `/stats` returns; keep both
sides of the invariant visible, not a boolean. A `.print` label cannot hold
an apostrophe: `'lesson''s'` prints as `lesson s` because the dot command
splits on the quote; reword the label instead.

## Interpretation rules

- `http_reqs` counts requests; `checks` counts assertions. Two checks per
  request plus teardown checks is normal.
- `http_req_duration` excludes connection setup. `http_req_failed` follows
  k6's expected-status policy; body checks are separate.
- Dropped iterations mean the arrival-rate executor could not start work: the
  generator was short of virtual users, not the server of capacity. Report
  them with the latency; both can matter in one run.
- Never average percentiles across buckets or runs; recompute from samples.
  The shared ten-second buckets show recovery after a spike or a stress step;
  boundary buckets are partial.
- A smoke run's p95 is a wiring check, not an estimate. Compare like profiles
  with similar sample counts.
- Keep failed and successful samples in separate rows (group by `status`),
  and treat an empty table as missing evidence, not a passing run.
- For asynchronous work, HTTP latency is acceptance latency. Completion needs
  a domain check or bounded polling with its resolution stated.

## Documentation

- [Metric reference](https://grafana.com/docs/k6/latest/using-k6/metrics/reference/)
- [Tags and groups](https://grafana.com/docs/k6/latest/using-k6/tags-and-groups/)
- [Checks and thresholds](https://grafana.com/docs/k6/latest/using-k6/thresholds/)
- [Open vs closed models](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/)
- [Ramping arrival rate](https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/ramping-arrival-rate/)

Reviewed 2026-09-27; tag and JSON shapes verified locally 2026-09-28.
