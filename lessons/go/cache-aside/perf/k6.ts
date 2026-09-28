import {
  assertResponse,
  request,
  stats,
} from "../../../../scripts/perf/workload.ts";
import { Rate } from "k6/metrics";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

interface Operation {
  name: string;
  source: "cache" | "postgres";
}
interface Stats {
  hits: number;
  misses: number;
}

const misses = new Rate("cache_miss");
export default function (): void {
  const response = request("GET", "/operation");
  assertResponse<Operation>(response, (b) => {
    if (b.source) misses.add(b.source === "postgres");
    return b.name === "Ada" && ["cache", "postgres"].includes(b.source);
  });
}
export function teardown(): void {
  stats<Stats>((b) => b.hits + b.misses > 0);
}
