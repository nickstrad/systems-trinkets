-- completed-job-counter under load. run.ts appends this after
-- scripts/perf/analyze.sql, in the same DuckDB session and run directory, so
-- `samples`, settings.json, and domain.json are all readable here.

.print '== completed-job-counter: the base lesson under concurrent traffic =='
.print 'DELIVERIES in the settings above is deliveries in the base main.go'

-- One row per operation sample. k6 writes custom tags to extra_tags as
-- key=value&key=value; variant is the base table's mode, attempt is the
-- delivery number of one event (1 = first delivery, 2.. = retries).
create temp view operations as
select tag(extra_tags, 'variant') as variant,
       cast(tag(extra_tags, 'attempt') as integer) as attempt,
       status, metric_value
from samples
where metric_name = 'http_req_duration' and name = 'operation';

-- The lesson's own signals, recorded by k6 from responses that passed both checks.
create temp view deliveries as
select tag(extra_tags, 'variant') as variant,
       cast(tag(extra_tags, 'attempt') as integer) as attempt,
       metric_name, metric_value
from samples
where metric_name in ('core_ms', 'delivery_applied');

create temp view domain as
select unnest(d.variants, recursive := true)
from read_json('domain.json') d;

.print 'Share of operation requests by variant (the counts sum to the shared operation count)'
select coalesce(variant, 'untagged') as variant,
       count(*) as requests,
       round(100 * count(*) / sum(count(*)) over (), 1) as share_pct
from operations
group by all
order by all;

.print 'Per variant: did the counter count unique jobs or delivery attempts? (base table 1)'
-- 1. Same columns as the base analyze.sql. delivery_attempts and increments
-- come from k6 samples; unique_events and actual_count come from /stats
-- (domain.json): the adapter's tally of distinct confirmed events, and the
-- job_counter row itself. failed_attempts keeps non-200 requests visible.
with k6 as (
  select o.variant,
         count(*) as delivery_attempts,
         count(*) filter (where o.status <> '200') as failed_attempts
  from operations o
  group by all
), applied as (
  select variant, cast(sum(metric_value) as bigint) as increments
  from deliveries where metric_name = 'delivery_applied'
  group by all
)
select variant, k6.delivery_attempts, k6.failed_attempts,
       d.unique_events,
       d.actual_total as actual_count,
       applied.increments,
       d.actual_total - d.unique_events as overcount
from domain d
left join k6 using (variant)
left join applied using (variant)
order by variant;

.print 'Per variant and attempt number: deliveries that incremented vs duplicates ignored (base table 2)'
select variant, attempt,
       count(*) as deliveries,
       cast(sum(metric_value) as bigint) as increments,
       count(*) - cast(sum(metric_value) as bigint) as duplicates_ignored
from deliveries
where metric_name = 'delivery_applied'
group by all
order by all;

.print 'Per variant, first delivery vs retry: core.Apply time (p50_ms, p95_ms as in the base) beside HTTP time (base table 3)'
-- 2. p50_ms/p95_ms are the server's timing of core.Apply, the same span as
-- the base lesson's latency_ms (the adapter acquires a pool connection before
-- starting the clock). p50_http_ms/p95_http_ms are what k6 saw for the same
-- successful requests; the gap is HTTP, JSON, and server queueing.
with core as (
  select variant, if(attempt = 1, 'first', 'retry') as delivery_kind,
         count(*) as samples,
         round(median(metric_value), 3) as p50_ms,
         round(quantile_cont(metric_value, 0.95), 3) as p95_ms
  from deliveries
  where metric_name = 'core_ms'
  group by all
), http as (
  select variant, if(attempt = 1, 'first', 'retry') as delivery_kind,
         round(median(metric_value), 3) as p50_http_ms,
         round(quantile_cont(metric_value, 0.95), 3) as p95_http_ms
  from operations
  where status = '200'
  group by all
)
select * from core left join http using (variant, delivery_kind)
order by variant, delivery_kind;

.print 'The question: naive overcount beside idempotent overcount (base: 200 vs 0 for 100 events x 3 deliveries)'
-- 3. Named groups: a mistyped variant shows as NULL beside a value.
-- overcount_per_event should be DELIVERIES - 1 for naive and 0 for idempotent.
select max(actual_total - unique_events) filter (where variant = 'naive') as naive_overcount,
       max(actual_total - unique_events) filter (where variant = 'idempotent') as idempotent_overcount,
       round(max((actual_total - unique_events) / unique_events) filter (where variant = 'naive'), 3) as naive_overcount_per_event,
       round(max((actual_total - unique_events) / unique_events) filter (where variant = 'idempotent'), 3) as idempotent_overcount_per_event
from domain;

.print 'Invariant per variant: job_counter total vs what the strategy promises (main.go panics when they differ)'
-- 4. expected_total is main.go's wantTotal computed from confirmed calls:
-- naive = every confirmed delivery, idempotent = unique events. seen is the
-- job_seen row count (idempotent only; naive never claims). A failed
-- delivery whose commit outcome is unknown could show as a difference.
select variant, deliveries, failed_deliveries, unique_events, seen,
       expected_total, actual_total,
       actual_total - expected_total as difference
from domain
order by variant;
