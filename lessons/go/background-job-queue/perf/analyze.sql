-- background-job-queue under load. run.ts appends this after
-- scripts/perf/analyze.sql, in the same DuckDB session and run directory, so
-- `samples`, settings.json, and domain.json are all readable here.

.print ''
.print '== background-job-queue: the base lesson under concurrent traffic =='
.print 'Settings above: WORK_MS is workTime in the base main.go; POOL_SIZE is per variant'

-- One row per operation sample, with its variant and round size (the k6
-- WORKERS setting, the base lesson's workers column) pulled out of
-- extra_tags with the shared tag() macro.
create temp view operations as
select tag(extra_tags, 'variant') as variant,
       cast(tag(extra_tags, 'workers') as integer) as workers,
       status, timestamp, metric_value
from samples
where metric_name = 'http_req_duration' and name = 'operation';

-- claim_ms is the server's Claim.Took for each 200 response: the claim query
-- alone, exactly what the base runner writes to measurements.csv.
create temp view claims as
select tag(extra_tags, 'variant') as variant,
       cast(tag(extra_tags, 'workers') as integer) as workers,
       metric_value as claim_ms
from samples
where metric_name = 'claim_ms';

.print 'Base lesson table under load: claim wait by variant (milliseconds)'
-- p50_claim_ms and max_claim_ms are the base table's p50_ms and max_ms.
-- HTTP time adds the insert, pool waits, WORK_MS of processing, the update,
-- and the commit, so it sits about WORK_MS above the claim for both variants.
-- Failed requests keep their own row and have no claim time.
with http as (
  select variant, workers, status,
         count(*) as claims,
         round(median(metric_value), 2) as p50_http_ms,
         round(quantile_cont(metric_value, 0.95), 2) as p95_http_ms
  from operations
  group by all
), server as (
  select variant, workers, '200' as status,
         round(median(claim_ms), 2) as p50_claim_ms,
         round(quantile_cont(claim_ms, 0.95), 2) as p95_claim_ms,
         round(max(claim_ms), 2) as max_claim_ms
  from claims
  group by all
)
select http.*, server.p50_claim_ms, server.p95_claim_ms, server.max_claim_ms
from http
left join server using (variant, workers, status)
order by variant, workers, status;

.print 'Share of operation requests by variant (the per-variant counts sum to the shared operation count)'
select coalesce(variant, 'untagged') as variant,
       count(*) as requests,
       round(100 * count(*) / sum(count(*)) over (), 1) as share_pct
from operations
group by all
order by all;

.print 'How busy the blocking queue was: claims per second x time each claim holds the row lock'
-- A blocking queue serves one claim at a time, so utilization near 1 means
-- rounds start overlapping and waits grow without bound; skip_locked serves
-- up to POOL_SIZE claims at once, so its number is not a saturation point.
-- hold_ms_est is median HTTP minus median claim: WORK_MS plus the insert,
-- update, commit, and HTTP overhead. Whole-second timestamps make run_s
-- approximate for short runs.
with span as (
  select greatest(max(timestamp) - min(timestamp), 1) as run_s from operations
), http as (
  select variant, count(*) as claims, median(metric_value) as p50_http_ms
  from operations where status = '200' group by all
), server as (
  select variant, median(claim_ms) as p50_claim_ms from claims group by all
)
select variant, claims, run_s,
       round(claims / run_s, 2) as claims_per_s,
       round(p50_http_ms - p50_claim_ms, 1) as hold_ms_est,
       round(claims_per_s * hold_ms_est / 1000, 2) as single_lock_utilization
from http join server using (variant), span
order by variant;

.print 'Blocking vs skip locked: how many times slower the slowest claim is (the base lesson question)'
-- The base analyze.sql compares max(claim_ms) per worker count. Under load,
-- p95 is shown beside it because one unlucky round decides the max.
-- Named groups: a mistyped variant shows as NULL beside a value.
select workers,
       count(*) as claims,
       round(max(claim_ms) filter (where variant = 'blocking'), 2) as blocking_max_ms,
       round(max(claim_ms) filter (where variant = 'skip_locked'), 2) as skip_locked_max_ms,
       round(blocking_max_ms / skip_locked_max_ms, 0) as blocking_slowdown,
       round(quantile_cont(claim_ms, 0.95) filter (where variant = 'blocking'), 2) as blocking_p95_ms,
       round(quantile_cont(claim_ms, 0.95) filter (where variant = 'skip_locked'), 2) as skip_locked_p95_ms,
       round(blocking_p95_ms / skip_locked_p95_ms, 0) as p95_slowdown
from claims
group by all
order by all;

.print 'Invariant per variant: did every claim get its own job, and did every claim finish?'
-- The base analyze.sql's last table: claims vs distinct_jobs. Here the
-- server counts claims and distinct job ids per variant (each variant has
-- its own table), and the table itself says how many jobs are done. Expect
-- duplicate_claims = 0, done = claims, and pending = enqueued - claims
-- (only failed or empty claims leave a job pending). k6_claims counts the
-- claim_ms samples k6 recorded from 200 responses: it should equal claims.
with k6 as (
  select variant, count(*) as k6_claims from claims group by all
), domain as (
  select unnest(variants, recursive := true) from read_json('domain.json')
)
select variant, enqueued, claims, k6.k6_claims, distinct_jobs,
       claims - distinct_jobs as duplicate_claims,
       done, done - claims as done_minus_claims,
       pending, empty_claims, failed_requests
from domain
left join k6 using (variant)
order by variant;
