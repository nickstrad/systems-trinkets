import {
  assertResponse,
  request,
  stats,
} from "../../../../scripts/perf/workload.ts";
import { Trend } from "k6/metrics";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

interface Operation {
  claim_ms: number;
  completed: boolean;
  job_id: number;
}
interface Stats {
  done: number;
  pending: number;
}

const claims = new Trend("claim_ms", true);
export default function (): void {
  const response = request("POST", "/operation");
  assertResponse<Operation>(response, (b) => {
    if (typeof b.claim_ms === "number") claims.add(b.claim_ms);
    return b.completed === true && b.job_id > 0;
  });
}
export function teardown(): void {
  stats<Stats>((b) => b.done > 0 && b.pending === 0);
}
