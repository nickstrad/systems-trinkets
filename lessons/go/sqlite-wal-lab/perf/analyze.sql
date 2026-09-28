-- sqlite-wal-lab under load. run.ts appends this after
-- scripts/perf/analyze.sql, in the same DuckDB session and run directory, so
-- `samples`, the tag() macro, settings.json, and domain.json are all readable
-- here.

.print ''
.print '== sqlite-wal-lab: the base lesson under concurrent traffic =='
.print '(BUSY_MS in the settings at the top is the busy timeout, 2000 in the base main.go)'

-- The server's write_ms trend: one sample per answered write, tagged with
-- variant (journal mode) and outcome (ok or locked).
create temp view writes as
select tag(extra_tags, 'variant') as variant,
       tag(extra_tags, 'outcome') as outcome,
       metric_value as write_ms
from samples
where metric_name = 'write_ms';

-- One row per operation request, with its variant pulled out of extra_tags.
create temp view operations as
select tag(extra_tags, 'variant') as variant,
       status, metric_value as http_ms
from samples
where metric_name = 'http_req_duration' and name = 'operation';

.print 'Base lesson table under load: write latency by journal mode and outcome (write_ms from the server, milliseconds)'
-- 1. The base analyze.sql groups by mode and result; here variant is the
-- mode and outcome is the result (locked = database is locked, SQLITE_BUSY).
-- writes replaces trials. Locked writes keep their own row, never averaged
-- with successful ones.
select variant, outcome,
       count(*) as writes,
       round(median(write_ms), 2) as p50_ms,
       round(quantile_cont(write_ms, 0.95), 2) as p95_ms,
       round(max(write_ms), 2) as max_ms
from writes
group by all
order by all;

.print 'HTTP time beside write time, by journal mode (the gap is HTTP, JSON, and pool wait)'
-- 2. Requests are tagged before the outcome is known, so HTTP time splits by
-- variant and status only. share_pct: the two variants should be near 50 %
-- each, and requests should sum to the shared "Operation HTTP count".
with http as (
  select variant, status,
         count(*) as requests,
         round(100 * count(*) / sum(count(*)) over (), 1) as share_pct,
         round(median(http_ms), 2) as p50_http_ms,
         round(quantile_cont(http_ms, 0.95), 2) as p95_http_ms
  from operations
  group by all
), server as (
  select variant, '200' as status,
         round(median(write_ms), 2) as p50_write_ms,
         round(max(write_ms), 2) as max_write_ms
  from writes
  group by all
)
select http.*, server.p50_write_ms, server.max_write_ms,
       round(http.p50_http_ms - server.p50_write_ms, 2) as p50_http_minus_write_ms
from http
left join server using (variant, status)
order by variant, status;

.print 'DELETE vs WAL: locked-write share and median write time (the question the base table answers)'
-- 3. Named groups: a mistyped variant shows as NULL beside a value. The base
-- lesson measured 3 of 3 DELETE writes locked at ~2027 ms (its busy timeout)
-- and 3 of 3 WAL writes ok at ~0.13 ms.
select round(100 * count(*) filter (where variant = 'DELETE' and outcome = 'locked')
             / count(*) filter (where variant = 'DELETE'), 1) as delete_locked_pct,
       round(100 * count(*) filter (where variant = 'WAL' and outcome = 'locked')
             / count(*) filter (where variant = 'WAL'), 1) as wal_locked_pct,
       round(median(write_ms) filter (where variant = 'DELETE'), 2) as delete_p50_write_ms,
       round(median(write_ms) filter (where variant = 'WAL'), 2) as wal_p50_write_ms,
       round(delete_p50_write_ms / wal_p50_write_ms, 0) as delete_to_wal_ratio,
       (select cast(server['BUSY_MS'] as integer) from read_json('settings.json')) as busy_ms
from writes;

.print 'Invariant per journal mode: rows the database holds vs seed rows + successful writes'
-- 4. A locked write must add no row, so rows = expected_rows (seed_rows +
-- writes_ok) and difference = 0. writes_locked should match the locked
-- writes counted in the tables above. snapshot_rows is what the reader held
-- open since startup still sees: the seed count, however many rows WAL
-- writers added after it (snapshot isolation). wal_bytes is the -wal file
-- the held reader keeps from being checkpointed.
with domain as (
  select unnest(d.variants, recursive := true)
  from read_json('domain.json') d
)
select variant, journal_mode, writes_ok, writes_locked, writes_failed,
       seed_rows, expected_rows, rows, rows - expected_rows as difference,
       snapshot_rows, wal_bytes
from domain
order by variant;
