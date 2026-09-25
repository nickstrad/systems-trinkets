create table measurements as from 'measurements.csv';

-- 1. Did the database count jobs or delivery attempts?
-- actual_count is the total seen after the last delivery; it must equal the
-- increments the client saw applied.
select
  mode,
  count(*) as delivery_attempts,
  count(distinct event_id) as unique_events,
  -- arg_max(value, key) returns value from the row where key is largest.
  -- The key (event_id, attempt) orders rows by delivery, so this picks the
  -- total observed after the last delivery, even if the counter ever went down.
  arg_max(observed_total, (event_id, attempt)) as actual_count,
  sum(applied) as increments,
  actual_count - unique_events as overcount
from measurements
-- group by all groups by every selected column that is not an aggregate
-- (here: mode). order by all sorts by every selected column, left to right.
group by all
order by all;

-- 2. Which attempts actually changed the counter?
select
  mode,
  attempt,
  count(*) as deliveries,
  count(*) filter (where applied = 1) as increments,
  count(*) filter (where applied = 0) as duplicates_ignored
from measurements
-- group by all / order by all: group and sort by the non-aggregate columns.
group by all
order by all;

-- 3. Inspect client-observed transaction latency, not just SQL execution time.
select
  mode,
  if(attempt = 1, 'first', 'retry') as delivery_kind,
  count(*) as samples,
  -- p50 (median): half the deliveries in this group finished faster than this.
  round(median(latency_ms), 3) as p50_ms,
  -- p95: 95% of deliveries in this group finished faster than this. It shows
  -- the slow tail the median hides. quantile_cont interpolates between rows.
  round(quantile_cont(latency_ms, 0.95), 3) as p95_ms
from measurements
-- group by all / order by all: group and sort by the non-aggregate columns.
group by all
order by all;
