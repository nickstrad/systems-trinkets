-- Shared k6 analysis. run.ts feeds this to DuckDB from a run's results
-- directory, then appends the lesson's optional perf/analyze.sql.
create table samples as
select * replace (cast(metric_value as double) as metric_value,
                  cast(timestamp as bigint) as timestamp)
from read_csv('metrics.csv', header = true, all_varchar = true);

-- tag pulls one custom tag out of extra_tags (k6 writes key=value&key=value);
-- the anchored key keeps 'variant' from matching 'sub_variant'.
create macro tag(extra, key) as
  regexp_extract(extra, '(^|&)' || key || '=([^&]+)', 2);

.print 'Server settings for this run (from /health; fixtures says whether the run started its own server)'
select fixtures, unnest(server) from read_json('settings.json');

.print 'HTTP latency by scenario, operation, and status (milliseconds)'
select scenario, name, status,
       count(*) as samples,
       round(median(metric_value), 2) as p50_ms,
       round(quantile_cont(metric_value, 0.95), 2) as p95_ms
from samples
where metric_name = 'http_req_duration' and name = 'operation'
group by all
order by all;

.print 'HTTP failure rate by scenario and operation'
select scenario, name,
       count(*) as samples,
       round(100 * avg(metric_value), 2) as failed_pct
from samples
where metric_name = 'http_req_failed' and name = 'operation'
group by all
order by all;

.print 'Iterations the load generator could not start'
select coalesce(sum(metric_value), 0) as dropped_iterations
from samples
where metric_name = 'dropped_iterations';

.print 'Operation HTTP count (excludes stats and repair)'
select coalesce(sum(metric_value), 0) as requests
from samples where metric_name = 'http_reqs' and name = 'operation';

.print 'Operation completions per 10-second bucket, including empty buckets'
with bounds as (
  select min(timestamp) // 10 * 10 as lo, max(timestamp) // 10 * 10 as hi
  from samples
), buckets as (
  select unnest(generate_series(lo, hi, 10)) as second from bounds
), duration as (
  select *, timestamp // 10 * 10 as second
  from samples where metric_name = 'http_req_duration' and name = 'operation'
)
select to_timestamp(b.second) as bucket_start,
       count(d.metric_value) as completed,
       round(quantile_cont(d.metric_value, 0.95), 2) as p95_ms
from buckets b left join duration d using(second)
group by b.second order by b.second;

.print 'Custom metrics from the lesson workload (mean of samples)'
select metric_name, count(*) as samples, round(avg(metric_value), 4) as mean_value
from samples
where metric_name not like 'http_%'
  and metric_name not in ('checks', 'data_received', 'data_sent', 'dropped_iterations',
                          'iteration_duration', 'iterations', 'vus', 'vus_max')
group by all
order by all;
