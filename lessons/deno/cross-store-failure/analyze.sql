create or replace table measurements as from 'measurements.csv';

.print 'Object store vs metadata per phase: consistent means every object has a row'
select
  phase,
  object_count,
  metadata_count,
  object_count - metadata_count as gap,
  orphan_count,
  object_count = metadata_count and orphan_count = 0 as consistent
from measurements;

.print 'Repair summary: orphans found, orphans left, and how long the delete took'
select
  max(orphan_count) filter (where phase = 'before_reconcile') as initial_orphans,
  max(orphan_count) filter (where phase = 'after_reconcile') as remaining_orphans,
  initial_orphans - remaining_orphans as repaired,
  max(reconcile_ms) filter (where phase = 'after_reconcile') as reconcile_ms
from measurements;
