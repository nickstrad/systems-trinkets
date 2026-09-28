create table measurements as from 'measurements.csv';

.print 'Per variant: did every run leave the counters summing to the batch size?'
-- 1. The runner panics on a failed invariant after writing its row, so a
-- false here means a run failed and the CSV shows which.
select
  variant,
  count(*) as runs,
  bool_and(actual_sum = batch_size) as all_sums_match
from measurements
group by all
order by all;

.print 'Per variant: whole-batch latency (avg, p50 in ms) and amortized cost per write'
-- 2. us_per_write is derived here rather than stored: elapsed_ms covers the
-- whole batch, so dividing by batch_size gives the per-command cost.
select
  variant,
  round(avg(elapsed_ms), 3) as avg_batch_ms,
  round(median(elapsed_ms), 3) as p50_batch_ms,
  round(p50_batch_ms * 1000 / max(batch_size), 3) as p50_us_per_write
from measurements
group by all
order by all;

.print 'Sequential vs pipeline: ratio of median batch latency'
-- 3. Name each group with filter so the ratio reads sequential / pipeline
-- regardless of which one happens to be slower.
select
  round(median(elapsed_ms) filter (where variant = 'sequential'), 3) as sequential_ms,
  round(median(elapsed_ms) filter (where variant = 'pipeline'), 3) as pipeline_ms,
  round(sequential_ms / pipeline_ms, 2) as sequential_to_pipeline_ratio
from measurements;
