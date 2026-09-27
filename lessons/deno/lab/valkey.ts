// Module valkey holds the node-redis helpers Valkey lessons repeat. It lives
// beside lab.ts rather than in it so a lesson that never talks to Valkey does
// not load the redis driver.
import { createClient } from "redis";
import { env } from "lab/lab.ts";

/** DEFAULT_URL is the local dev Valkey from services/index.md (no auth). */
export const DEFAULT_URL = "redis://localhost:6379";

/** url is CACHE_URL, or DEFAULT_URL. */
export function url(): string {
  return env("CACHE_URL", DEFAULT_URL);
}

/**
 * connect opens a client for url(), connects, and pings it. The ping matters:
 * createClient does not contact the server, and node-redis queues commands
 * while disconnected, so without it a stopped Valkey only surfaces on the
 * first timed command.
 */
export async function connect() {
  const client = createClient({ url: url() });
  await client.connect();
  await client.ping();
  return client;
}
