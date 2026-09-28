import { createOperations, type Mode, schema as createTable } from "../core.ts";
import { env } from "lab/lab.ts";
import * as postgres from "lab/postgres.ts";
import * as seaweedfs from "lab/seaweedfs.ts";

function choice<T extends string>(name: string, allowed: readonly T[]): T {
  const value = env(name, allowed[0]);
  if (!allowed.includes(value as T)) {
    throw new Error(`${name} must be one of ${allowed.join(", ")}`);
  }
  return value as T;
}
function int(name: string, fallback: number, min: number, max: number) {
  const n = Number(env(name, String(fallback)));
  if (!Number.isInteger(n) || n < min || n > max) {
    throw new Error(`${name} must be ${min}..${max}`);
  }
  return n;
}
const mode = choice<Mode>("MODE", [
  "intent_then_put",
  "put_then_insert",
  "insert_then_put",
]);
const poolSize = int("POOL_SIZE", 8, 1, 64);
const port = int("PORT", 8080, 0, 65535);
const settings = { MODE: mode, POOL_SIZE: poolSize };
const token = crypto.randomUUID().replaceAll("-", "");
const schema = `perf_${token}`;
const bucket = "lab";
const prefix = `perf/${token}/`;
console.log("temporary fixtures:", { schema, bucket, prefix });
// Every pooled connection starts in the private schema; the schema may not
// exist yet at connect time, which search_path tolerates.
const pg = postgres.pool(
  poolSize,
  `-c search_path=${schema} -c statement_timeout=5000`,
);
const s3 = seaweedfs.connect();
await pg.query(`create schema ${schema}`);
await pg.query(createTable);
await seaweedfs.ensureBucket(s3, bucket);
const operations = createOperations({
  pg,
  s3,
  bucket,
  prefix,
  concurrency: poolSize,
});
let stopping = false;
let active = 0;
let accepted = 0;
const json = (body: unknown, status = 200) => Response.json(body, { status });
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
      return json(await operations.measure());
    }
    if (request.method === "POST" && url.pathname === "/repair") {
      if (active !== 0) {
        return json({ error: "wait for uploads to finish" }, 409);
      }
      return json({ checked: await operations.repair[mode]() });
    }
    if (request.method === "POST" && url.pathname === "/operation") {
      const id = Number(url.searchParams.get("id"));
      const crashValue = url.searchParams.get("crash");
      if (
        !url.searchParams.has("id") || !Number.isInteger(id) || id < 0 ||
        id >= 10000 || !["true", "false"].includes(crashValue || "")
      ) {
        return json({
          error: "id must be 0..9999; crash must be true or false",
        }, 400);
      }
      if (active >= 64 || accepted >= 10000) {
        return json({ error: "run limit reached" }, 429);
      }
      accepted++;
      active++;
      try {
        const crash = crashValue === "true";
        await operations.upload(mode, `${prefix}object-${id}.txt`, crash);
        return json({ uploaded: !crash, simulated_crash: crash });
      } finally {
        active--;
      }
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
  await seaweedfs.deleteKeys(
    s3,
    bucket,
    await seaweedfs.listKeys(s3, bucket, prefix),
  );
  await pg.query(`drop schema ${schema} cascade`);
} finally {
  await pg.end();
  s3.destroy();
  Deno.removeSignalListener("SIGINT", stop);
  Deno.removeSignalListener("SIGTERM", stop);
}
