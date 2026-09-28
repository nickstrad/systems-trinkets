import {
  assertResponse,
  request,
  stats,
} from "../../../../scripts/perf/workload.ts";
import exec from "k6/execution";
import { Rate } from "k6/metrics";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

interface Operation {
  applied: boolean;
  mode: "naive" | "idempotent";
}
interface Stats {
  total: number;
  seen: number;
  mode: string;
}

const applied = new Rate("delivery_applied");
export default function (): void {
  const id = exec.scenario.iterationInTest % 10000;
  const first = request("POST", `/operation?id=${id}`);
  assertResponse<Operation>(first, (b) => typeof b.applied === "boolean");
  const duplicate = request("POST", `/operation?id=${id}`);
  assertResponse<Operation>(duplicate, (b) => {
    if (typeof b.applied === "boolean") applied.add(b.applied);
    return b.mode === "naive"
      ? b.applied === true
      : b.mode === "idempotent" && b.applied === false;
  });
}
export function teardown(): void {
  stats<Stats>((b) =>
    b.total > 0 && (b.mode === "naive" || b.total === b.seen)
  );
}
