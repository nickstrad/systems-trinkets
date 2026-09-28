-- cross-store-failure under load. run.ts appends this after
-- scripts/perf/analyze.sql, in the same DuckDB session and run directory, so
-- `samples`, the tag() macro, and domain.json are all readable here.

.print ''
.print '== cross-store-failure: the base lesson under concurrent traffic (POOL_SIZE is Postgres connections per variant) =='

-- One row per operation sample, with the variant and crash flag pulled out of
-- extra_tags.
create temp view operations as
select tag(extra_tags, 'variant') as variant,
       tag(extra_tags, 'crash') = '1' as crashed,
       status, metric_value
from samples
where metric_name = 'http_req_duration' and name = 'operation';

create temp view uploads as
select tag(extra_tags, 'variant') as variant,
       tag(extra_tags, 'crash') = '1' as crashed,
       metric_value
from samples
where metric_name = 'upload_ms';

-- domain.json is the final /stats, taken after teardown's repair: the flat
-- fields are the state after repair, before_repair is the measure() the
-- repair started from, repair is its cost.
create temp view domain as
select unnest(v)
from (select unnest(variants) as v from read_json('domain.json'));

-- The reconciler cost per variant, shared by tables 2 and 5.
create temp view repair_cost as
select variant, uploads,
       before_repair.objects as objects_before,
       repair.checked as checked,
       repair.reconcile_ms as reconcile_ms,
       round(1000 * repair.reconcile_ms / nullif(uploads, 0), 1) as us_per_object,
       round(1000 * repair.reconcile_ms / nullif(repair.checked, 0), 1) as us_per_checked
from domain;

.print 'Base lesson table under load: what each write order leaves behind and what the repair keeps'
-- 1. The base analyze.sql's first table, one row per variant. uploads counts
-- upload calls that returned; crashed of them died before their last write.
-- expected_kept comes from the server's /stats.
select variant, uploads, crashed, failed,
       before_repair.orphans as orphan_objects,
       before_repair.dangling as dangling_rows,
       before_repair.pending as pending_rows,
       objects as objects_kept,
       rows as rows_kept,
       expected_kept,
       orphans + dangling + pending = 0 as consistent
from domain
order by variant;

.print 'Reconciler cost: scanning everything vs scanning what is in doubt'
-- 2. The base analyze.sql's second table. objects is the number of uploads
-- (the base column: store size attempted), objects_before what the store held
-- when the repair started; checked is what the reconciler looked at in the
-- object store. Timed around repair[mode]() as main.ts does,
-- after the traffic stopped, so it is not a latency under load.
select variant,
       uploads as objects,
       objects_before,
       checked,
       round(reconcile_ms, 1) as reconcile_ms,
       us_per_object,
       us_per_checked
from repair_cost
order by variant;

.print 'Upload latency by variant and crash (milliseconds): HTTP beside the server-timed upload call'
-- 3. p50_http_ms is k6's http_req_duration; p50_upload_ms is the server's
-- timing of core upload() alone (main.ts never times single uploads). Their
-- gap is what HTTP, JSON, and the server add. A crashed upload skips its last
-- write, so it should be faster. Failed requests keep their own row and have
-- no upload time.
with http as (
  select variant, crashed, status,
         count(*) as requests,
         round(median(metric_value), 3) as p50_http_ms,
         round(quantile_cont(metric_value, 0.95), 3) as p95_http_ms
  from operations
  group by all
), server as (
  select variant, crashed, '200' as status,
         round(median(metric_value), 3) as p50_upload_ms,
         round(quantile_cont(metric_value, 0.95), 3) as p95_upload_ms
  from uploads
  group by all
)
select http.*, server.p50_upload_ms, server.p95_upload_ms
from http left join server using (variant, crashed, status)
order by variant, crashed, status;

.print 'Share of operation requests by variant (sums to the shared operation count)'
select coalesce(variant, 'untagged') as variant,
       count(*) as requests,
       round(100 * count(*) / sum(count(*)) over (), 1) as share_pct
from operations
group by all
order by all;

.print 'The question: intent_then_put beside put_then_insert (and insert_then_put)'
-- 4. Named groups: a mistyped variant shows as NULL beside a value. The
-- intent order keeps every upload the others lose to repair, and checks only
-- what is in doubt; compare its us_per_checked with the full scans'.
select max(d.objects) filter (where variant = 'intent_then_put') as intent_kept,
       max(d.objects) filter (where variant = 'put_then_insert') as put_first_kept,
       max(d.objects) filter (where variant = 'insert_then_put') as insert_first_kept,
       max(r.checked) filter (where variant = 'intent_then_put') as intent_checked,
       max(r.checked) filter (where variant = 'put_then_insert') as put_first_checked,
       max(r.reconcile_ms) filter (where variant = 'intent_then_put') as intent_reconcile_ms,
       max(r.reconcile_ms) filter (where variant = 'put_then_insert') as put_first_reconcile_ms,
       max(r.us_per_checked) filter (where variant = 'intent_then_put') as intent_us_per_checked,
       max(r.us_per_checked) filter (where variant = 'put_then_insert') as put_first_us_per_checked
from domain d join repair_cost r using (variant);
