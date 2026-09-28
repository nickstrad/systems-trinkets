import {
  assertResponse,
  repair,
  request,
  stats,
} from "../../../../scripts/perf/workload.ts";
import exec from "k6/execution";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

interface Operation {
  simulated_crash: boolean;
  uploaded: boolean;
}
interface Stats {
  orphans: number;
  dangling: number;
  pending: number;
}

export default function (): void {
  const id = exec.scenario.iterationInTest;
  const crash = id % 10 === 0;
  const response = request("POST", `/operation?id=${id}&crash=${crash}`);
  assertResponse<Operation>(
    response,
    (b) => b.simulated_crash === crash && b.uploaded === !crash,
  );
}
export function teardown(): void {
  repair();
  stats<Stats>((b) => b.orphans === 0 && b.dangling === 0 && b.pending === 0);
}
