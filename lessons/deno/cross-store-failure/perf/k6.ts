// The base lesson's three write orders under concurrent traffic. Iterations
// round-robin the variants, so each gets the same arrival share in the same
// run, and every 10th upload of each variant crashes before its last write
// (main.ts: id % crashEvery === 0). Teardown logs /stats, repairs every
// variant, and checks what the repair kept.
import {
  assertResponse,
  everyVariant,
  number,
  repair,
  request,
} from "../../../../scripts/perf/workload.ts";
import exec from "k6/execution";
import { Trend } from "k6/metrics";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

const variants = [
  "put_then_insert",
  "insert_then_put",
  "intent_then_put",
] as const;
type Variant = typeof variants[number];

// CRASH_EVERY is main.ts's crashEvery: 1 in N uploads per variant dies.
const crashEvery = number("CRASH_EVERY", 10, 2, 1000);

interface Operation {
  variant: Variant;
  key: string;
  crashed: boolean;
  elapsed_ms: number;
}
interface Measure {
  objects: number;
  rows: number;
  orphans: number;
  dangling: number;
  pending: number;
}
interface VariantStats extends Measure {
  variant: Variant;
  uploads: number;
  crashed: number;
  failed: number;
  expected_kept: number;
  before_repair: Measure | null;
  repair: { checked: number; reconcile_ms: number } | null;
}

// upload_ms is the server's timing of the core upload call, tagged by variant
// and crash for perf/analyze.sql.
const uploadMs = new Trend("upload_ms", true);

export default function (): void {
  const i = exec.scenario.iterationInTest;
  const variant: Variant = variants[i % variants.length];
  // The variant's own upload index, so each variant crashes 1 in crashEvery.
  const crash = Math.floor(i / variants.length) % crashEvery === 0;
  const tags = { variant, crash: crash ? "1" : "0" };
  const response = request(
    "POST",
    "/operation?variant=" + variant + "&crash=" + tags.crash,
    tags,
  );
  const b = assertResponse<Operation>(
    response,
    (b) => b.variant === variant && b.crashed === crash && b.elapsed_ms >= 0,
  );
  if (b) uploadMs.add(b.elapsed_ms, tags);
}

// The base lesson's ordering row, per variant: each order leaves only its own
// failure class, and after repair the stores agree and hold the kept uploads
// (the server's expected_kept).
export function teardown(): void {
  repair();
  everyVariant<VariantStats>(variants, (v) => {
    const before = v.before_repair;
    if (!before || !v.repair || v.uploads === 0 || v.failed !== 0) {
      return false;
    }
    const leftBehind = {
      put_then_insert: [before.orphans, before.dangling + before.pending],
      insert_then_put: [before.dangling, before.orphans + before.pending],
      intent_then_put: [before.pending, before.orphans + before.dangling],
    }[v.variant];
    return leftBehind[0] === v.crashed && leftBehind[1] === 0 &&
      v.orphans + v.dangling + v.pending === 0 &&
      v.objects === v.expected_kept && v.rows === v.expected_kept;
  });
}
