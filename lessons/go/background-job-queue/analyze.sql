create table measurements as from 'measurements.csv';

-- .print writes a line to stdout before the table that follows it.
.print 'Per mode and worker count: how long a worker waited to claim a job (p50, max in ms)'
-- blocking: every worker queues on the same oldest row, so the k-th worker
-- waits about k times workTime and the slowest claim grows linearly with the
-- worker count. skip_locked: each worker takes the next free row at once, so
-- the slowest claim stays flat no matter how many workers there are.
select
  mode,
  workers,
  count(*) as claims,
  -- p50 (median): half the claims in this group finished faster than this.
  round(median(claim_ms), 2) as p50_ms,
  -- max: the last worker in the queue. This is the number that scales.
  round(max(claim_ms), 2) as max_ms
from measurements
-- group by all / order by all: group and sort by the non-aggregate columns.
group by all
order by all;

.print 'Per worker count: how many times slower the slowest blocking claim is than the slowest skip locked claim'
-- filter (where ...) aggregates one mode at a time; naming the modes keeps the
-- ratio meaning blocking / skip_locked. DuckDB lets a later column reuse an
-- earlier alias in the same select.
select
  workers,
  round(max(claim_ms) filter (where mode = 'blocking'), 2) as blocking_max_ms,
  round(max(claim_ms) filter (where mode = 'skip_locked'), 2) as skip_locked_max_ms,
  round(blocking_max_ms / skip_locked_max_ms, 0) as blocking_slowdown
from measurements
group by all
order by all;

.print 'Per mode and worker count: did every worker get its own job?'
-- A blocked worker re-checks the row it waited on, sees it is done, and moves
-- to the next pending one, so both modes hand out each job exactly once.
-- distinct_jobs counts (trial, job) pairs; it equals claims when no job was
-- claimed twice in the same trial.
select
  mode,
  workers,
  count(*) as claims,
  count(distinct (trial, claimed_job)) as distinct_jobs
from measurements
group by all
order by all;
