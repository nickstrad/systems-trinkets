create table measurements as from 'measurements.csv';

-- .print writes a line to stdout before the table that follows it.
.print 'Per journal mode and outcome: write latency (p50, max in ms) and trial counts'
-- One row per mode and outcome, so writes that waited out busy_timeout and
-- failed are never averaged with writes that succeeded.
select
  mode,
  result,
  count(*) as trials,
  -- p50 (median): half the trials in this group finished faster than this.
  round(median(write_ms), 2) as p50_ms,
  round(max(write_ms), 2) as max_ms
from measurements
-- group by all groups by every selected column that is not an aggregate
-- (here: mode, result). order by all sorts by every selected column, left to right.
group by all
order by all;
