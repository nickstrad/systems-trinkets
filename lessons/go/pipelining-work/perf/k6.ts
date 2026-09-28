import {
  assertResponse,
  request,
  stats,
} from "../../../../scripts/perf/workload.ts";
import { Counter } from "k6/metrics";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

interface Operation {
  mode: "pipeline" | "sequential";
  increments: number;
}
interface Stats {
  mode: string;
  batches: number;
  expected_sum: number;
  actual_sum: number;
}

// One request is one batch; increments sums the INCRs the server confirmed,
// so its total must equal expected_sum in the final domain state.
const increments = new Counter("increments");
export default function (): void {
  const response = request("POST", "/operation");
  assertResponse<Operation>(response, (b) => {
    if (b.increments > 0) increments.add(b.increments);
    return ["pipeline", "sequential"].includes(b.mode) && b.increments > 0;
  });
}
export function teardown(): void {
  stats<Stats>((b) => b.batches > 0 && b.actual_sum === b.expected_sum);
}
