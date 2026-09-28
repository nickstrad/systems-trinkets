// The base lesson's two journal modes under concurrent traffic: iterations
// alternate DELETE and WAL, so both get the same arrival share in the same
// run, and every sample carries a variant tag for perf/analyze.sql. Each
// write lands while the server holds a reader snapshot on that database.
import {
  assertResponse,
  everyVariant,
  request,
} from "../../../../scripts/perf/workload.ts";
import exec from "k6/execution";
import { Trend } from "k6/metrics";
export { handleSummary, options } from "../../../../scripts/perf/workload.ts";

const variants = ["DELETE", "WAL"] as const;
type Variant = typeof variants[number];

interface Operation {
  variant: Variant;
  outcome: "ok" | "locked";
  error?: string; // the SQLite error text on a locked write, as in measurements.csv
  elapsed_ms: number;
}
interface VariantStats {
  variant: Variant;
  writes_ok: number;
  writes_locked: number;
  writes_failed: number;
  seed_rows: number;
  expected_rows: number;
  rows: number;
  snapshot_rows: number;
}

// write_ms is the server's own timing of the write (the base lesson's
// write_ms), tagged by variant and outcome so the analysis can rebuild the
// base table and count the locked share.
const writeMs = new Trend("write_ms", true);

export default function (): void {
  const variant: Variant =
    variants[exec.scenario.iterationInTest % variants.length];
  const response = request("POST", "/operation?variant=" + variant, {
    variant,
  });
  // A locked DELETE write is a known result, not a failure: the check is
  // that the server answered for this variant with ok or locked.
  const b = assertResponse<Operation>(
    response,
    (b) =>
      b.variant === variant && b.elapsed_ms >= 0 &&
      (b.outcome === "ok" || b.outcome === "locked"),
  );
  if (b) writeMs.add(b.elapsed_ms, { variant, outcome: b.outcome });
}

// Per variant: some writes were answered, none failed outside a lock, rows
// in the database equal the seed rows plus successful writes (a locked write
// adds nothing), and the held reader snapshot still sees only the seed rows.
export function teardown(): void {
  everyVariant<VariantStats>(
    variants,
    (v) =>
      v.writes_ok + v.writes_locked > 0 && v.writes_failed === 0 &&
      v.rows === v.expected_rows && v.snapshot_rows === v.seed_rows,
  );
}
