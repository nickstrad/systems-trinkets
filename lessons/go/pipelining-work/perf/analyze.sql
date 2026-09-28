-- pipelining-work under load. run.ts appends this after
-- scripts/perf/analyze.sql, in the same DuckDB session and run directory, so
-- `samples`, the tag() macro, and domain.json are all readable here.

.print ''
.print '== pipelining-work: the base lesson under concurrent traffic =='
.print 'BATCH_SIZE in the settings header is batchSize in the base main.go.'

-- One row per operation sample, with its variant pulled out of extra_tags.
create temp view operations as
select tag(extra_tags, 'variant') as variant, status, metric_value
from samples
where metric_name = 'http_req_duration' and name = 'operation';

create temp view core as
select tag(extra_tags, 'variant') as variant, metric_value
from samples
where metric_name = 'core_ms';

.print 'Base lesson table under load: batch latency by variant and status (milliseconds)'
-- 1. p50_batch_ms is HTTP time; p50_core_ms is the server timing the core
-- call exactly as main.go times it (the base lesson's elapsed_ms). Their gap
-- is what HTTP, JSON, and queueing in the server add. Failed requests keep
-- their own row and have no core time.
with http as (
  select variant, status,
         count(*) as batches,
         round(avg(metric_value), 3) as avg_batch_ms,
         round(median(metric_value), 3) as p50_batch_ms,
         round(quantile_cont(metric_value, 0.95), 3) as p95_batch_ms
  from operations
  group by all
), server as (
  select variant, '200' as status,
         round(median(metric_value), 3) as p50_core_ms,
         round(quantile_cont(metric_value, 0.95), 3) as p95_core_ms
  from core
  group by all
)
select http.*, server.p50_core_ms, server.p95_core_ms,
       round(server.p50_core_ms * 1000 / d.batch_size, 3) as p50_core_us_per_write
from http
left join server using (variant, status)
cross join read_json('domain.json') d
order by variant, status;

.print 'Share of operation requests by variant (the per-variant counts sum to the shared operation count)'
select coalesce(variant, 'untagged') as variant,
       count(*) as requests,
       round(100 * count(*) / sum(count(*)) over (), 1) as share_pct
from operations
group by all
order by all;

.print 'Sequential vs pipeline: ratio of median batch latency (the last table of the base analyze.sql)'
-- 2. Named groups: a mistyped variant shows as NULL beside a value. The core
-- ratio is the one to compare with measurements.csv; the HTTP ratio shows
-- how much of the advantage a client over HTTP still sees.
with http as (
  select median(metric_value) filter (where variant = 'sequential') as sequential_http_ms,
         median(metric_value) filter (where variant = 'pipeline') as pipeline_http_ms
  from operations where status = '200'
), server as (
  select median(metric_value) filter (where variant = 'sequential') as sequential_core_ms,
         median(metric_value) filter (where variant = 'pipeline') as pipeline_core_ms
  from core
)
select round(sequential_core_ms, 3) as sequential_core_ms,
       round(pipeline_core_ms, 3) as pipeline_core_ms,
       round(sequential_core_ms / pipeline_core_ms, 2) as core_sequential_to_pipeline_ratio,
       round(sequential_http_ms, 3) as sequential_http_ms,
       round(pipeline_http_ms, 3) as pipeline_http_ms,
       round(sequential_http_ms / pipeline_http_ms, 2) as http_sequential_to_pipeline_ratio
from http, server;

.print 'Invariant per variant: counters Valkey holds (MGET) vs confirmed batches x batch size'
-- 3. The base runner panics when actual_sum != batch_size after one batch.
-- Here counters accumulate over the run, so expected_sum = batches *
-- batch_size. failed_batches may have applied some INCRs before failing,
-- which would show as a positive difference.
with domain as (
  select d.batch_size, unnest(d.variants, recursive := true)
  from read_json('domain.json') d
)
select variant, batches, failed_batches, batch_size,
       expected_sum, actual_sum,
       actual_sum - expected_sum as difference
from domain
order by variant;
