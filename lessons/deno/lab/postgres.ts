// Module postgres holds the node-postgres helpers Postgres lessons repeat. It
// lives beside lab.ts rather than in it so a lesson that never talks to
// Postgres does not load the pg driver.
// pg ships no types; this directive gives every lesson a typed Client.
// @ts-types="npm:@types/pg@^8"
import { Client, Pool } from "pg";
import { env } from "lab/lab.ts";

/**
 * DEFAULT_URL is the local dev database from software/software.md. The
 * credentials are not secret; every lesson prints and uses them as-is.
 */
export const DEFAULT_URL =
  "postgres://trinkets:trinkets@localhost:5432/trinkets";

/** url is DATABASE_URL, or DEFAULT_URL. */
export function url(): string {
  return env("DATABASE_URL", DEFAULT_URL);
}

/**
 * connect opens one connection to url(). Set DATABASE_URL to point a lesson
 * at another database. Call it once per connection a lesson needs: two
 * transactions can only contend for a row lock from two separate connections.
 * A connection error rejects, which stops the lesson: every measurement
 * after a setup failure would be meaningless.
 */
export async function connect(): Promise<Client> {
  const client = new Client({ connectionString: url() });
  await client.connect();
  return client;
}

/**
 * pool opens up to max connections to url() lazily. options are Postgres
 * startup parameters applied to every connection, e.g.
 * "-c search_path=perf_x -c statement_timeout=5000", so pooled connections
 * never need a separate `set` round trip. Callers end() it on shutdown.
 */
export function pool(max: number, options?: string): Pool {
  return new Pool({ connectionString: url(), max, options });
}
