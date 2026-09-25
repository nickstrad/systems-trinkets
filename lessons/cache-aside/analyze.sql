create table measurements as from 'measurements.csv';

-- .print writes a line to stdout before the table that follows it.
.print 'Latency per source: hits vs misses (avg, p50, p95 in microseconds)'
select
  source,
  count(*) as requests,
  round(avg(latency_us), 1) as avg_us,
  -- p50 (median): half the requests from this source finished faster than this.
  median(latency_us) as p50_us,
  -- p95: 95% of requests finished faster than this; shows the tail the median hides.
  quantile_cont(latency_us, 0.95) as p95_us
from measurements
-- group by all / order by all: group and sort by the non-aggregate columns.
group by all
order by all;

.print 'Summary: hit rate and how many times slower a miss is than a hit'
-- DuckDB lets a later column reuse an earlier alias in the same select.
select
  count(*) as total_requests,
  count(*) filter (where source = 'cache') as cache_hits,
  round(100.0 * cache_hits / total_requests, 1) as hit_rate_pct,
  round(avg(latency_us) filter (where source = 'postgres'), 1) as miss_us,
  round(avg(latency_us) filter (where source = 'cache'), 1) as hit_us,
  round(miss_us / hit_us, 1) as miss_to_hit_ratio
from measurements;
