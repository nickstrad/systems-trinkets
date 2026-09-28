// The base lesson's two strategies under concurrent traffic. Each iteration
// is one event: it picks a variant (alternating naive and idempotent, so both
// get the same arrival share in the same run) and delivers the event
// DELIVERIES times in a row (the server's setting, read from /health in
// setup()), like main.go's attempt loop. Every request and custom metric
// sample carries variant and attempt tags for perf/analyze.sql.
import {
  assertResponse,
  everyVariant,
  request,
  settings,
} from "../../../../scripts/perf/workload.ts";
import exec from "k6/execution";
import { Rate, Trend } from "k6/metrics";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

const variants = ["naive", "idempotent"] as const;
type Variant = typeof variants[number];

interface Operation {
  variant: Variant;
  attempt: number;
  applied: boolean;
  total?: number;
  elapsed_ms: number;
}
interface VariantStats {
  variant: Variant;
  deliveries: number;
  failed_deliveries: number;
  unique_events: number;
  seen: number | null;
  expected_total: number;
  actual_total: number;
}
interface Setup {
  deliveries: number;
}

// core_ms is the server's timing of core.Apply (the base lesson's
// latency_ms); delivery_applied is 1 when the delivery incremented the
// counter and 0 when it was skipped as a duplicate. Both are tagged by
// variant and attempt so the analysis can rebuild the base tables.
const coreMs = new Trend("core_ms", true);
const deliveryApplied = new Rate("delivery_applied");

// The adapter owns DELIVERIES (it validates the range and rejects an attempt
// above it); reading it here means k6 and the server cannot disagree.
export function setup(): Setup {
  const { DELIVERIES } = settings<{ DELIVERIES?: number }>();
  if (DELIVERIES === undefined) throw new Error("/health has no DELIVERIES");
  return { deliveries: DELIVERIES };
}

export default function ({ deliveries }: Setup): void {
  const n = exec.scenario.iterationInTest;
  const variant: Variant = variants[n % variants.length];
  // iterationInTest is unique across VUs, so each event id belongs to one
  // iteration: its only duplicates are this loop's own retries. Ids start
  // at 1 because the server's warm-up uses 0.
  const id = n + 1;
  for (let attempt = 1; attempt <= deliveries; attempt++) {
    const tags = { variant, attempt: String(attempt) };
    const response = request(
      "POST",
      `/operation?variant=${variant}&id=${id}&attempt=${attempt}`,
      tags,
    );
    const b = assertResponse<Operation>(response, (b) => {
      // main.go's semantics per call: naive applies every delivery;
      // idempotent applies only the first. A total comes only with applied.
      const want = variant === "naive" || attempt === 1;
      return b.variant === variant && b.attempt === attempt &&
        b.applied === want && (b.total !== undefined) === b.applied &&
        b.elapsed_ms >= 0;
    });
    if (b) {
      deliveryApplied.add(b.applied, tags);
      coreMs.add(b.elapsed_ms, tags);
    }
  }
}

// main.go's invariant, per variant: the counter row equals what the strategy
// promises (naive: every confirmed delivery; idempotent: each unique event
// once, which job_seen also holds), and both variants ran.
export function teardown(): void {
  everyVariant<VariantStats>(variants, (v) => {
    if (v.unique_events === 0) return false;
    const holds = v.actual_total === v.expected_total;
    return v.variant === "naive" ? holds : holds && v.seen === v.actual_total;
  });
}
