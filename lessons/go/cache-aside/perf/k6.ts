// The base lesson's read path under concurrent traffic. Iterations request
// the server's profiles round-robin (?id=1..PROFILE_COUNT), so each profile
// is read every PROFILE_COUNT/RATE seconds and a TTL shorter than that run
// expires between reads. The source (cache or postgres) is known only from
// the response, so the per-source metrics are custom Trends tagged by source.
import exec from "k6/execution";
import { Trend } from "k6/metrics";
import {
  assertResponse,
  request,
  settings,
  stats,
} from "../../../../scripts/perf/workload.ts";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

const sources = ["cache", "postgres"] as const;
type Source = typeof sources[number];

interface Operation {
  id: number;
  name: string;
  source: Source;
  elapsed_ms: number;
}
interface Stats {
  profiles: number;
  served: number;
  failed: number;
  stored: number;
  cached: number;
  cached_agree: number;
}
interface Setup {
  profiles: number;
}

// operation_ms is http_req_duration for the same request, re-recorded with
// the source tag the request could not carry; core_ms is the server's timing
// of core.ReadProfile (the base lesson's latency_us, in milliseconds).
const operationMs = new Trend("operation_ms", true);
const coreMs = new Trend("core_ms", true);

// The profile count is a server setting; read it from /health rather than
// asking the learner to pass it twice.
export function setup(): Setup {
  const profiles = settings<{ PROFILE_COUNT: number }>().PROFILE_COUNT;
  if (!Number.isInteger(profiles) || profiles < 1) {
    throw new Error("/health did not report PROFILE_COUNT");
  }
  return { profiles };
}

export default function (data: Setup): void {
  const id = (exec.scenario.iterationInTest % data.profiles) + 1;
  const response = request("GET", "/operation?id=" + id);
  // The base runner's per-call expectation: the stored name comes back for
  // the requested profile, from one of the two sources.
  const b = assertResponse<Operation>(
    response,
    (r) =>
      sources.includes(r.source) && r.id === id && r.name === `Ada ${id}` &&
      r.elapsed_ms >= 0,
  );
  // Every operation gets exactly one operation_ms sample, so the per-source
  // counts sum to the shared operation count; failures land under "error".
  const tags = { source: b ? b.source : "error" };
  operationMs.add(response.timings.duration, tags);
  if (b) coreMs.add(b.elapsed_ms, tags);
}

// The invariant on the server's own counts: every read was served, and every
// key still cached agrees with Postgres. (Each response's name is checked
// above; hits + misses = served holds by construction in the server.)
export function teardown(): void {
  stats<Stats>((s) =>
    s.served > 0 && s.failed === 0 &&
    s.stored === s.profiles &&
    s.cached_agree === s.cached
  );
}
