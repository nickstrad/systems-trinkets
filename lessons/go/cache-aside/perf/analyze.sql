-- cache-aside under load. run.ts appends this after scripts/perf/analyze.sql,
-- in the same DuckDB session and run directory, so `samples`, settings.json,
-- and domain.json are all readable here.

.print ''
.print '== cache-aside: the base lesson under concurrent traffic =='
.print 'Settings (header above): TTL_MS is cacheTTL in the base main.go; PROFILE_COUNT profiles are read round-robin'

-- The source is only known from the response, so k6 re-records each
-- operation's HTTP time as operation_ms tagged source=cache|postgres|error,
-- and the server's timing of core.ReadProfile as core_ms (reads that passed
-- their checks only).
create temp view operations as
select tag(extra_tags, 'source') as source,
       timestamp, metric_value
from samples
where metric_name = 'operation_ms';

create temp view core as
select tag(extra_tags, 'source') as source,
       metric_value * 1000 as latency_us
from samples
where metric_name = 'core_ms';

.print 'Base lesson table under load: latency per source (HTTP in ms; core in microseconds, as in measurements.csv)'
-- 1. avg_us / p50_us / p95_us are the base table's columns, measured the way
-- main.go measures them (time around core.ReadProfile only). The *_http_ms
-- columns add HTTP, JSON, and queueing in the server. share_pct sums to 100
-- and requests sums to the shared "Operation HTTP count".
with http as (
  select source,
         count(*) as requests,
         round(100 * count(*) / sum(count(*)) over (), 1) as share_pct,
         round(avg(metric_value), 3) as avg_http_ms,
         round(median(metric_value), 3) as p50_http_ms,
         round(quantile_cont(metric_value, 0.95), 3) as p95_http_ms
  from operations
  group by all
), server as (
  select source,
         round(avg(latency_us), 1) as avg_us,
         round(median(latency_us), 1) as p50_us,
         round(quantile_cont(latency_us, 0.95), 1) as p95_us
  from core
  group by all
)
select * from http left join server using (source)
order by source;

.print 'Summary: hit rate and how many times slower a miss is than a hit (the last table of the base analyze.sql)'
-- 2. Named groups: a mistyped source shows as NULL beside a value. miss_us,
-- hit_us, and miss_to_hit_ratio use avg like the base query, from core time;
-- the http_* columns are what a client over HTTP sees.
with http as (
  select count(*) filter (where source in ('cache', 'postgres')) as total_requests,
         count(*) filter (where source = 'cache') as cache_hits,
         avg(metric_value) filter (where source = 'postgres') as miss_http_ms,
         avg(metric_value) filter (where source = 'cache') as hit_http_ms
  from operations
), server as (
  select avg(latency_us) filter (where source = 'postgres') as miss_us,
         avg(latency_us) filter (where source = 'cache') as hit_us
  from core
)
select total_requests, cache_hits,
       round(100.0 * cache_hits / total_requests, 1) as hit_rate_pct,
       round(miss_us, 1) as miss_us,
       round(hit_us, 1) as hit_us,
       round(miss_us / hit_us, 1) as miss_to_hit_ratio,
       round(miss_http_ms, 3) as miss_http_ms,
       round(hit_http_ms, 3) as hit_http_ms,
       round(miss_http_ms / hit_http_ms, 1) as http_miss_to_hit_ratio
from http, server;

.print 'Misses over time (5-second buckets): a TTL that outlasts the run misses only at the start; a short TTL misses every TTL'
with bounds as (
  select min(timestamp) as t0, max(timestamp) as t1 from operations
), buckets as (
  select unnest(generate_series(0, (t1 - t0) // 5 * 5, 5)) as second from bounds
), tagged as (
  select (timestamp - t0) // 5 * 5 as second, source from operations, bounds
)
select b.second as from_second,
       count(t.source) as requests,
       count(t.source) filter (where t.source = 'postgres') as misses,
       round(100.0 * count(t.source) filter (where t.source = 'cache')
             / nullif(count(t.source), 0), 1) as hit_rate_pct
from buckets b left join tagged t using (second)
group by b.second order by b.second;

.print 'Invariant: served reads vs k6 reads that passed; cached keys vs keys that agree with Postgres'
-- 3. k6 checks every response's name against the stored "Ada <id>", and
-- k6_served counts core_ms samples (reads that passed), so it should equal
-- served. hits + misses = served by construction in the server.
-- redundant_misses counts misses that started before another miss of the
-- same profile had returned (a stampede); it is not an error, only extra
-- Postgres reads.
with k6 as (
  select count(*) as k6_served from core
)
select d.ttl_ms, d.profiles, d.served, k6.k6_served, d.hits, d.misses,
       d.failed, d.redundant_misses,
       d.stored, d.cached, d.cached_agree
from read_json('domain.json') d, k6;
