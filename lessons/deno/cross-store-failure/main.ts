// Lesson: a write that spans two stores has no transaction. Each upload PUTs
// an object to SeaweedFS and then INSERTs its metadata row in Postgres; a
// crash between the two leaves an orphan object that no row points at. The
// repair is a reconciler: list both sides, diff, delete what only the object
// store has.
//
// Needs: make up-postgres up-seaweedfs
import { PutObjectCommand } from "@aws-sdk/client-s3";
import { Measurements } from "lab/lab.ts";
import * as postgres from "lab/postgres.ts";
import * as seaweedfs from "lab/seaweedfs.ts";

const bucket = "lab";
const prefix = "cross-store-failure/";
const uploads = 20;
// Every crashEvery-th upload "crashes" after the PUT and before the INSERT.
const crashEvery = 5;

const pg = await postgres.connect();
const s3 = seaweedfs.connect();

await seaweedfs.ensureBucket(s3, bucket);
await pg.query(`
  drop table if exists cross_store_failure_metadata;

  create table cross_store_failure_metadata(
    object_key text primary key,
    created_at timestamptz not null default now()
  );
`);
await seaweedfs.deleteKeys(
  s3,
  bucket,
  await seaweedfs.listKeys(s3, bucket, prefix),
);

for (let id = 1; id <= uploads; id++) {
  const key = `${prefix}object-${id}.txt`;
  await s3.send(
    new PutObjectCommand({ Bucket: bucket, Key: key, Body: `payload-${id}` }),
  );
  if (id % crashEvery === 0) {
    console.log(`simulated crash after PUT: ${key}`);
    continue;
  }
  await pg.query(
    `insert into cross_store_failure_metadata(object_key) values ($1)`,
    [key],
  );
}

// findOrphans is the reconciler's read side: the object keys the store has
// that no metadata row claims. The two reads are independent, so they overlap.
async function findOrphans() {
  const [keys, metadata] = await Promise.all([
    seaweedfs.listKeys(s3, bucket, prefix),
    pg.query<{ object_key: string }>(
      `select object_key from cross_store_failure_metadata`,
    ),
  ]);
  const known = new Set(metadata.rows.map((row) => row.object_key));
  return {
    objects: keys.length,
    metadata: known.size,
    orphans: keys.filter((key) => !known.has(key)),
  };
}

const before = await findOrphans();
console.log("before reconciliation", before);

const start = performance.now();
for (const key of before.orphans) console.log(`deleting orphan: ${key}`);
await seaweedfs.deleteKeys(s3, bucket, before.orphans);
const reconcileMs = performance.now() - start;

const after = await findOrphans();
console.log("after reconciliation", after);

const out = new Measurements(
  "phase",
  "object_count",
  "metadata_count",
  "orphan_count",
  "reconcile_ms",
);
out.write(
  "before_reconcile",
  before.objects,
  before.metadata,
  before.orphans.length,
  0,
);
out.write(
  "after_reconcile",
  after.objects,
  after.metadata,
  after.orphans.length,
  reconcileMs.toFixed(3),
);
out.close();

await pg.end();
