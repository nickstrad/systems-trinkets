// HTTP adapter for the cross-store-failure lesson: the base lesson's three
// write orders, chosen per request, so one k6 run fills the base lesson's
// ordering table. It imports core.ts, never main.ts (importing main.ts would
// run the whole experiment).
//
//   POST /operation?variant=<mode>&crash=0|1  one upload in that write order;
//        crash=1 dies right before the last write, as in main.ts. A simulated
//        crash is a 200 with crashed: true, not an error.
//   POST /repair  measure() then repair[mode]() per variant, timed like
//        main.ts; answers 409 while uploads are in flight.
//   GET  /stats   per variant: uploads, crashed, failed, expected_kept,
//        the current measure(), and the last pre-repair measure() and repair
//        cost.
//   GET  /health  settings for settings.json.
//
// Fixtures: each variant gets its own schema (perf_<token>_<variant>) and its
// own S3 prefix (perf/<token>/<variant>/) in bucket `lab`. The core's metadata
// table has a fixed name and its prefix scopes only S3 listings, so variants
// sharing one table would see each other's rows as dangling, and the
// insert_then_put repair would delete them. Shutdown removes both.
//
// Needs: make up-postgres up-seaweedfs
//   Postgres  postgres://trinkets:trinkets@localhost:5432/trinkets
//   SeaweedFS http://localhost:8333 (access key trinkets, secret trinkets-secret)
import {
  createOperations,
  type Mode,
  type Operations,
  schema as createTable,
  type State,
} from "../core.ts";
import { env } from "lab/lab.ts";
import * as postgres from "lab/postgres.ts";
import * as seaweedfs from "lab/seaweedfs.ts";

const variants: readonly Mode[] = [
  "put_then_insert",
  "insert_then_put",
  "intent_then_put",
];
const isVariant = (v: string | null): v is Mode => variants.includes(v as Mode);

function int(name: string, fallback: number, min: number, max: number) {
  const n = Number(env(name, String(fallback)));
  if (!Number.isInteger(n) || n < min || n > max) {
    throw new Error(`${name} must be ${min}..${max}`);
  }
  return n;
}
// POOL_SIZE is connections per variant; three variants open up to 3x this.
const poolSize = int("POOL_SIZE", 4, 1, 32);
const port = int("PORT", 8080, 0, 65535);
// Per-variant cap: repair and /stats list the whole prefix, so bound the store.
const maxUploads = 10_000;
const maxActive = 64;
const settings = { POOL_SIZE: poolSize, MAX_UPLOADS_PER_VARIANT: maxUploads };

const token = crypto.randomUUID().replaceAll("-", "");
const bucket = "lab";
const root = `perf/${token}/`;
const s3 = seaweedfs.connect();

interface Repair {
  checked: number;
  reconcile_ms: number;
}
interface Variant {
  schema: string;
  prefix: string;
  pg: ReturnType<typeof postgres.pool>;
  ops: Operations;
  next: number; // next object id; keys are never reused within a server
  uploads: number; // upload calls that returned (crashed or not)
  crashed: number; // simulated crashes among them
  failed: number; // upload calls that threw a dependency error
  before: State | null; // measure() taken by the last /repair, before repairing
  repair: Repair | null;
}

const state = {} as Record<Mode, Variant>;
for (const variant of variants) {
  const schema = `perf_${token}_${variant}`;
  const prefix = `${root}${variant}/`;
  // Every pooled connection starts in this variant's schema; the schema may
  // not exist yet at connect time, which search_path tolerates.
  const pg = postgres.pool(
    poolSize,
    `-c search_path=${schema} -c statement_timeout=5000`,
  );
  state[variant] = {
    schema,
    prefix,
    pg,
    ops: createOperations({ pg, s3, bucket, prefix, concurrency: poolSize }),
    next: 0,
    uploads: 0,
    crashed: 0,
    failed: 0,
    before: null,
    repair: null,
  };
}
console.log("temporary fixtures:", {
  bucket,
  prefixes: variants.map((v) => state[v].prefix),
  schemas: variants.map((v) => state[v].schema),
});

async function cleanup() {
  try {
    await seaweedfs.deleteKeys(
      s3,
      bucket,
      await seaweedfs.listKeys(s3, bucket, root),
    );
    for (const v of variants) {
      await state[v].pg.query(
        `drop schema if exists ${state[v].schema} cascade`,
      );
    }
    console.log("removed fixtures under", root);
  } finally {
    await Promise.all(variants.map((v) => state[v].pg.end()));
    s3.destroy();
  }
}

function warm(pg: Variant["pg"]) {
  return Promise.all(Array.from({ length: poolSize }, async () => {
    const client = await pg.connect();
    try {
      await client.query("select 1");
    } finally {
      client.release();
    }
  }));
}

try {
  await seaweedfs.ensureBucket(s3, bucket);
  for (const v of variants) {
    await state[v].pg.query(`create schema ${state[v].schema}`);
    await state[v].pg.query(createTable);
  }
  // pool() dials lazily, so the first upload on each connection would pay a
  // connect inside upload_ms. Open all poolSize connections per variant now;
  // `select 1` writes no rows. lab/postgres.ts does not expose pool config,
  // so node-postgres's default idleTimeoutMillis (10 s) still applies: after
  // a lull longer than that, an upload re-dials inside the timed window.
  await Promise.all(variants.map((v) => warm(state[v].pg)));
} catch (error) {
  await cleanup();
  throw error;
}

let stopping = false;
let active = 0;
const json = (body: unknown, status = 200) => Response.json(body, { status });
const ms = (start: number) => Number((performance.now() - start).toFixed(3));

async function stats() {
  return {
    variants: await Promise.all(variants.map(async (variant) => {
      const v = state[variant];
      return {
        variant,
        uploads: v.uploads,
        crashed: v.crashed,
        failed: v.failed,
        // Kept means object and committed row both survive the repair.
        // Deleting an orphan or a dangling row discards the crashed upload;
        // finishing a pending intent keeps it.
        expected_kept: variant === "intent_then_put"
          ? v.uploads
          : v.uploads - v.crashed,
        ...(await v.ops.measure()),
        before_repair: v.before,
        repair: v.repair,
      };
    })),
  };
}

// repairAll mirrors one ordering row of main.ts per variant: measure, time
// repair[mode], and keep both for /stats (the runner saves the final /stats).
async function repairAll() {
  const result = [];
  for (const variant of variants) {
    const v = state[variant];
    v.before = await v.ops.measure();
    const start = performance.now();
    const checked = await v.ops.repair[variant]();
    v.repair = { checked, reconcile_ms: ms(start) };
    result.push({ variant, ...v.repair });
  }
  return { variants: result };
}

async function operation(url: URL) {
  const variant = url.searchParams.get("variant");
  const crashValue = url.searchParams.get("crash");
  if (!isVariant(variant) || (crashValue !== "0" && crashValue !== "1")) {
    return json({
      error: `variant must be one of ${
        variants.join(", ")
      }; crash must be 0 or 1`,
    }, 400);
  }
  const v = state[variant];
  if (active >= maxActive || v.next >= maxUploads) {
    return json({ error: "run limit reached" }, 429);
  }
  const key = `${v.prefix}object-${v.next++}.txt`;
  const crash = crashValue === "1";
  active++;
  try {
    // elapsed_ms times the upload call only. main.ts times only the repair;
    // the per-upload time is what this follow-up adds.
    const start = performance.now();
    await v.ops.upload(variant, key, crash);
    const elapsed_ms = ms(start);
    v.uploads++;
    if (crash) v.crashed++;
    return json({ variant, key, crashed: crash, elapsed_ms });
  } catch (error) {
    v.failed++;
    return json({ variant, key, error: String(error) }, 503);
  } finally {
    active--;
  }
}

const server = Deno.serve({
  hostname: "127.0.0.1",
  port,
  onListen: ({ port }) => {
    const url = `http://127.0.0.1:${port}`;
    const ready = Deno.env.get("READY_FILE");
    if (ready) Deno.writeTextFileSync(ready, url);
    console.log("serving", url);
  },
}, async (request) => {
  const url = new URL(request.url);
  if (stopping) return json({ error: "stopping" }, 503);
  try {
    if (request.method === "GET" && url.pathname === "/health") {
      return json({ ready: true, settings });
    }
    if (request.method === "GET" && url.pathname === "/stats") {
      return json(await stats());
    }
    if (request.method === "POST" && url.pathname === "/repair") {
      // A repair during uploads is main.ts's grace experiment, not this one.
      if (active !== 0) {
        return json({ error: "wait for uploads to finish" }, 409);
      }
      return json(await repairAll());
    }
    if (request.method === "POST" && url.pathname === "/operation") {
      return await operation(url);
    }
    return json({ error: "not found" }, 404);
  } catch (error) {
    return json({ error: String(error) }, 503);
  }
});

async function stop() {
  if (stopping) return;
  stopping = true;
  await server.shutdown();
}
Deno.addSignalListener("SIGINT", stop);
Deno.addSignalListener("SIGTERM", stop);
await server.finished;
try {
  await cleanup();
} finally {
  Deno.removeSignalListener("SIGINT", stop);
  Deno.removeSignalListener("SIGTERM", stop);
}
