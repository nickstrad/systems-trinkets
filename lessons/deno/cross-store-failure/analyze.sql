create table measurements as from 'measurements.csv';

.print 'Write order decides what a crash leaves behind (1000 uploads, 1 in 10 dies before its last write)'
-- objects_kept is how many of the 1000 uploads survive the repair: deleting
-- orphans or dangling rows discards the upload, finishing a pending intent
-- keeps it.
select
  mode,
  orphans_before as orphan_objects,
  dangling_before as dangling_rows,
  pending_before as pending_rows,
  objects_after as objects_kept,
  rows_after as rows_kept,
  orphans_after + dangling_after + pending_after = 0 as consistent
from measurements
where experiment = 'ordering' and objects = 1000
order by mode;

.print 'Reconciler cost vs store size: scanning everything vs scanning what is in doubt'
-- checked is what the reconciler looked at in the object store: every object
-- for the two absence-based repairs, only pending rows for the intent repair.
-- us_per_checked shows why fewer checks did not mean less time: one HEAD and
-- one UPDATE per pending row cost more than listing objects 1000 at a time.
select
  mode,
  objects,
  checked,
  round(reconcile_ms, 1) as reconcile_ms,
  round(1000 * reconcile_ms / objects, 1) as us_per_object,
  round(1000 * reconcile_ms / checked, 1) as us_per_checked
from measurements
where experiment = 'ordering'
order by mode, objects;

.print 'Grace window vs in-flight uploads: false deletes destroy finished writes, a wider window leaves young orphans for the next pass'
-- apparent_orphans counts in-flight uploads too: mid-window they look exactly
-- like crashed ones. false_deletes are rows whose object the reconciler
-- removed before the INSERT landed.
select
  grace_s,
  orphans_before as apparent_orphans,
  objects_before - objects_after as deleted,
  dangling_after - dangling_before as false_deletes,
  orphans_after as orphans_left,
  round(reconcile_ms, 1) as reconcile_ms
from measurements
where experiment = 'grace'
order by grace_s;
