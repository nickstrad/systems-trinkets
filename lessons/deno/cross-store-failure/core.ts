// Importing this module does not connect, reset data, or run experiments.
import { PutObjectCommand, type S3Client } from "@aws-sdk/client-s3";
import { mapConcurrent, sleep } from "lab/lab.ts";
import type * as postgres from "lab/postgres.ts";
import * as seaweedfs from "lab/seaweedfs.ts";

export const table = "cross_store_failure_metadata";
/** schema creates the metadata table; callers own dropping or truncating it. */
export const schema = `create table ${table}(
  object_key text primary key,
  status text not null default 'committed',
  created_at timestamptz not null default now()
)`;
export type Mode = "put_then_insert" | "insert_then_put" | "intent_then_put";

export interface Dependencies {
  pg: Pick<Awaited<ReturnType<typeof postgres.connect>>, "query">;
  s3: S3Client;
  bucket: string;
  prefix: string;
  concurrency: number;
}

// The caller owns client lifecycle, schema, fixtures, and experiment scheduling.
// Operations reject on dependency errors. Simulated crashes retain the lesson's
// early-return semantics. All metadata operations use the lesson's fixed table;
// prefix scopes object listing only, not independent metadata tenants.
export function createOperations(
  { pg, s3, bucket, prefix, concurrency }: Dependencies,
) {
  // upload is one two-store write in the given order. crash simulates the
  // process dying right before its last step, the only crash that matters:
  // before the first step nothing happened, after the last both stores agree.
  // delayMs holds the upload between its steps so a reconciler can catch it in
  // flight.
  async function upload(mode: Mode, key: string, crash: boolean, delayMs = 0) {
    const put = () =>
      s3.send(new PutObjectCommand({ Bucket: bucket, Key: key, Body: key }));
    const insert = (status: string) =>
      pg.query(`insert into ${table}(object_key, status) values ($1, $2)`, [
        key,
        status,
      ]);
    const commit = () =>
      pg.query(
        `update ${table} set status = 'committed' where object_key = $1`,
        [
          key,
        ],
      );

    switch (mode) {
      case "put_then_insert":
        await put();
        await sleep(delayMs);
        if (crash) return;
        await insert("committed");
        return;
      case "insert_then_put":
        await insert("committed");
        await sleep(delayMs);
        if (crash) return;
        await put();
        return;
      case "intent_then_put":
        await insert("pending");
        await put();
        await sleep(delayMs);
        if (crash) return;
        await commit();
        return;
    }
  }

  // measure compares the two stores. An orphan is an object with no row at all,
  // a dangling row has no object, and a pending row is an intent never
  // committed.
  async function measure() {
    const [keys, result] = await Promise.all([
      seaweedfs.listKeys(s3, bucket, prefix),
      pg.query<{ object_key: string; status: string }>(
        `select object_key, status from ${table}`,
      ),
    ]);
    const rows = result.rows;
    const known = new Set(rows.map((r) => r.object_key));
    const present = new Set(keys);
    return {
      objects: keys.length,
      rows: rows.length,
      orphans: keys.filter((k) => !known.has(k)).length,
      dangling: rows.filter((r) => !present.has(r.object_key)).length,
      pending: rows.filter((r) => r.status === "pending").length,
    };
  }

  // Each reconciler repairs one mode's failure class and returns how many
  // items in the object store it had to look at.

  // reconcileOrphans (put_then_insert) deletes objects no row claims, skipping
  // objects modified less than graceS seconds ago because a young unclaimed
  // object may be an upload whose INSERT has not happened yet. It must list the
  // whole prefix: an orphan is defined by absence, so nothing narrower finds it.
  async function reconcileOrphans(graceS: number): Promise<number> {
    const [objects, result] = await Promise.all([
      seaweedfs.listObjects(s3, bucket, prefix),
      pg.query<{ object_key: string }>(`select object_key from ${table}`),
    ]);
    const known = new Set(result.rows.map((r) => r.object_key));
    const cutoff = Date.now() - graceS * 1000;
    const orphans = objects
      .filter((o) => !known.has(o.key) && o.lastModified.getTime() <= cutoff)
      .map((o) => o.key);
    await seaweedfs.deleteKeys(s3, bucket, orphans);
    return objects.length;
  }

  // reconcileDangling (insert_then_put) deletes rows whose object never
  // arrived. Same full listing: "object missing" is also an absence.
  async function reconcileDangling(): Promise<number> {
    const [keys, result] = await Promise.all([
      seaweedfs.listKeys(s3, bucket, prefix),
      pg.query<{ object_key: string }>(`select object_key from ${table}`),
    ]);
    const present = new Set(keys);
    const dangling = result.rows
      .map((r) => r.object_key)
      .filter((k) => !present.has(k));
    await pg.query(`delete from ${table} where object_key = any($1)`, [
      dangling,
    ]);
    return keys.length;
  }

  // reconcilePending (intent_then_put) reads only pending rows and checks each
  // object. Present means the upload finished and only the commit was lost, so
  // finish it; absent means roll the intent back. Work grows with writes in
  // doubt, not with the store.
  async function reconcilePending(): Promise<number> {
    const result = await pg.query<{ object_key: string }>(
      `select object_key from ${table} where status = 'pending'`,
    );
    const pending = result.rows.map((r) => r.object_key);
    await mapConcurrent(pending, concurrency, async (key) => {
      if (await seaweedfs.exists(s3, bucket, key)) {
        await pg.query(
          `update ${table} set status = 'committed' where object_key = $1`,
          [key],
        );
      } else {
        await pg.query(`delete from ${table} where object_key = $1`, [key]);
      }
    });
    return pending.length;
  }

  const repair: Record<Mode, () => Promise<number>> = {
    put_then_insert: () => reconcileOrphans(0),
    insert_then_put: reconcileDangling,
    intent_then_put: reconcilePending,
  };

  return {
    upload,
    measure,
    reconcileOrphans,
    reconcileDangling,
    reconcilePending,
    repair,
  };
}

export type Operations = ReturnType<typeof createOperations>;
export type State = Awaited<ReturnType<Operations["measure"]>>;
