// The base lesson's two variants under concurrent traffic: iterations
// alternate sequential and pipeline, so both get the same arrival share in
// the same run, and every sample carries a variant tag for perf/analyze.sql.
import {
  assertResponse,
  everyVariant,
  request,
} from "../../../../scripts/perf/workload.ts";
import exec from "k6/execution";
import { Trend } from "k6/metrics";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

const variants = ["sequential", "pipeline"] as const;
type Variant = typeof variants[number];

interface Operation {
  variant: Variant;
  elapsed_ms: number;
}
interface VariantStats {
  variant: Variant;
  batches: number;
  failed_batches: number;
  expected_sum: number;
  actual_sum: number;
}

// core_ms is the server's own timing of the core call (the base lesson's
// elapsed_ms), tagged by variant so the analysis can split it.
const coreMs = new Trend("core_ms", true);

export default function (): void {
  const variant: Variant =
    variants[exec.scenario.iterationInTest % variants.length];
  const tags = { variant };
  const response = request("POST", "/operation?variant=" + variant, tags);
  const b = assertResponse<Operation>(
    response,
    (b) => b.variant === variant && b.elapsed_ms >= 0,
  );
  if (b) coreMs.add(b.elapsed_ms, tags);
}

// The base runner's invariant, per variant: the counters Valkey holds (MGET)
// sum to confirmed batches times the batch size, and both variants ran.
export function teardown(): void {
  everyVariant<VariantStats>(
    variants,
    (v) => v.batches > 0 && v.actual_sum === v.expected_sum,
  );
}
