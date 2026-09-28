// Lesson: a write that spans two stores has no transaction. An upload PUTs an
// object to SeaweedFS and writes a metadata row in Postgres; a crash between
// the two leaves the stores disagreeing, and a reconciler has to repair the
// disagreement later. Three experiments, one CSV:
//
//   ordering  For each write order and store size, upload objects with 1 in
//             crashEvery dying right before its last write, then repair. The
//             order decides the failure class: PUT-then-INSERT leaves orphan
//             objects nobody can see, INSERT-then-PUT leaves dangling rows
//             users can hit, and an intent row (pending -> PUT -> committed)
//             leaves only pending rows the reconciler can finish instead of
//             guess about. The same rows show reconciler cost against store
//             size: finding an absence means scanning everything, while
//             pending rows scope the work to what is in doubt.
//   grace     Run the orphan reconciler while uploads are still in flight.
//             With no grace window it deletes objects whose INSERT is
//             milliseconds away and the repair itself corrupts data. The
//             window has to cover the in-flight time plus the store's
//             timestamp granularity (whole seconds here), and every second of
//             it leaves young orphans for the next pass.
//
// The ordering rows also show that scoping the reconciler to pending rows
// makes it check 10x fewer items but not run faster at this crash rate: a
// HEAD plus an UPDATE per row costs more than listing everything in bulk.
// The scoped repair wins only when few writes are in doubt.
//
// Needs: make up-postgres up-seaweedfs. Runs about half a minute.
import {
  createOperations,
  type Mode,
  schema,
  type State,
  table,
} from "./core.ts";
import { mapConcurrent, Measurements, sleep } from "lab/lab.ts";
import * as postgres from "lab/postgres.ts";
import * as seaweedfs from "lab/seaweedfs.ts";

const bucket = "lab";
const prefix = "cross-store-failure/";
// 1 in crashEvery uploads dies right before its last write.
const crashEvery = 10;
const concurrency = 16;

// ordering: every mode at every size.
const modes: Mode[] = ["put_then_insert", "insert_then_put", "intent_then_put"];
const sizes = [100, 1000, 10_000];

// grace: oldOrphans crashed uploads aged past every window, then inflight
// uploads that PUT now and INSERT inflightMs later, with the reconciler
// running in between.
const graceSeconds = [0, 1, 2];
const oldOrphans = 20;
const inflight = 40;
const inflightMs = 1000;

const pg = await postgres.connect();
const s3 = seaweedfs.connect();
const { upload, measure, reconcileOrphans, repair } = createOperations({
  pg,
  s3,
  bucket,
  prefix,
  concurrency,
});
await seaweedfs.ensureBucket(s3, bucket);
await pg.query(`drop table if exists ${table}; ${schema}`);

const key = (id: number) => `${prefix}object-${id}.txt`;
const range = (n: number) => Array.from({ length: n }, (_, i) => i);

async function reset() {
  await pg.query(`truncate ${table}`);
  await seaweedfs.deleteKeys(
    s3,
    bucket,
    await seaweedfs.listKeys(s3, bucket, prefix),
  );
}

async function timed<T>(fn: () => Promise<T>) {
  const start = performance.now();
  const result = await fn();
  return { result, ms: performance.now() - start };
}

const out = new Measurements(
  "experiment",
  "mode",
  "objects",
  "grace_s",
  "objects_before",
  "rows_before",
  "orphans_before",
  "dangling_before",
  "pending_before",
  "checked",
  "reconcile_ms",
  "objects_after",
  "rows_after",
  "orphans_after",
  "dangling_after",
  "pending_after",
);

function record(
  experiment: string,
  mode: Mode,
  objects: number,
  graceS: number,
  before: State,
  checked: number,
  ms: number,
  after: State,
) {
  console.log(
    `${experiment} mode=${mode} objects=${objects} grace=${graceS}s ` +
      `before: orphans=${before.orphans} dangling=${before.dangling} pending=${before.pending} | ` +
      `checked=${checked} in ${ms.toFixed(1)}ms | ` +
      `after: objects=${after.objects} rows=${after.rows} orphans=${after.orphans} dangling=${after.dangling} pending=${after.pending}`,
  );
  out.write(
    experiment,
    mode,
    objects,
    graceS,
    before.objects,
    before.rows,
    before.orphans,
    before.dangling,
    before.pending,
    checked,
    ms.toFixed(3),
    after.objects,
    after.rows,
    after.orphans,
    after.dangling,
    after.pending,
  );
}

for (const mode of modes) {
  for (const n of sizes) {
    await reset();
    await mapConcurrent(
      range(n),
      concurrency,
      (id) => upload(mode, key(id), id % crashEvery === 0),
    );
    const before = await measure();
    const { result: checked, ms } = await timed(repair[mode]);
    const after = await measure();
    record("ordering", mode, n, 0, before, checked, ms, after);
  }
}

// The oldest grace window must already have passed for the old orphans, or
// the reconciler would skip them too.
const ageMs = Math.max(...graceSeconds) * 1000 + 200;
for (const graceS of graceSeconds) {
  await reset();
  await mapConcurrent(
    range(oldOrphans),
    concurrency,
    (id) => upload("put_then_insert", key(id), true),
  );
  await sleep(ageMs);
  // Start the in-flight PUTs late in a wall-clock second. The store records
  // whole seconds, so these objects will look up to a second older than they
  // are, the worst case a real system hits at random; placing them here makes
  // the run show it every time instead of one run in two.
  await sleep((800 - (Date.now() % 1000) + 1000) % 1000);
  const inflightDone = mapConcurrent(range(inflight), inflight, (id) =>
    upload(
      "put_then_insert",
      key(oldOrphans + id),
      id % crashEvery === 0,
      inflightMs,
    ));
  // Reconcile halfway through the in-flight window, then let the uploads
  // finish before measuring so their INSERTs land.
  await sleep(inflightMs / 2);
  const before = await measure();
  const { result: checked, ms } = await timed(() => reconcileOrphans(graceS));
  await inflightDone;
  const after = await measure();
  record(
    "grace",
    "put_then_insert",
    oldOrphans + inflight,
    graceS,
    before,
    checked,
    ms,
    after,
  );
}

out.close();
await pg.end();
