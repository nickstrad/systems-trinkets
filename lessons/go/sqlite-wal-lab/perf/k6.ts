import {
  assertResponse,
  request,
  stats,
} from "../../../../scripts/perf/workload.ts";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

interface Operation {
  written: boolean;
  mode: string;
}
interface Stats {
  writes: number;
  write_failures: number;
}

export default function (): void {
  const response = request("POST", "/operation");
  assertResponse<Operation>(
    response,
    (b) => b.written === true && b.mode === "WAL",
  );
}
export function teardown(): void {
  stats<Stats>((b) => b.writes > 0 && b.write_failures === 0);
}
