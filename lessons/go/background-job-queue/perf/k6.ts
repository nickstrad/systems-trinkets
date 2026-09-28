// The base lesson's two claim modes under concurrent traffic. Each iteration
// is one base-lesson round: WORKERS requests of one variant sent at once, so
// they race for the same queue exactly as the runner's workers do.
// Iterations alternate blocking and skip_locked, so both get the same arrival
// share in the same run, and every sample carries a variant tag for
// perf/analyze.sql.
import http from "k6/http";
import exec from "k6/execution";
import { Trend } from "k6/metrics";
import type { Options } from "k6/options";
import {
  assertResponse,
  base,
  everyVariant,
  number,
  operationParams,
  optionsFor,
} from "../../../../scripts/perf/workload.ts";
export { handleSummary } from "../../../../scripts/perf/workload.ts";

// WORKERS is the base lesson's workers per round (1, 2, 4, 8 there). Evenly
// spaced single requests (WORKERS=1) only contend once the blocking queue is
// saturated; simultaneous requests contend at any rate, as in the base runner.
const workers = number("WORKERS", 2, 1, 8);
// k6 sends at most 6 requests of one batch to a host at once by default;
// raise it so a round of 8 still starts together.
export const options: Options = optionsFor({}, { batch: 8, batchPerHost: 8 });

const variants = ["blocking", "skip_locked"] as const;
type Variant = typeof variants[number];

interface Operation {
  variant: Variant;
  job_id: number;
  elapsed_ms: number;
}
interface VariantStats {
  variant: Variant;
  enqueued: number;
  claims: number;
  distinct_jobs: number;
  done: number;
  pending: number;
  empty_claims: number;
  failed_requests: number;
}

// claim_ms is the server's Claim.Took, the base runner's claim_ms column,
// tagged by variant and round size so the analysis can split it.
const claimMs = new Trend("claim_ms", true);

export default function (): void {
  const variant: Variant =
    variants[exec.scenario.iterationInTest % variants.length];
  const tags = { variant, workers: String(workers) };
  // The shared request() helper sends one request and waits for it; a round
  // needs WORKERS in flight together, so this uses http.batch with the same
  // operationParams, which keeps the shared SQL's operation filter.
  const round = Array.from({ length: workers }, () => ({
    method: "POST",
    url: `${base}/operation?variant=${variant}`,
    params: operationParams(tags),
  }));
  for (const response of http.batch(round)) {
    const b = assertResponse<Operation>(
      response,
      (b) => b.variant === variant && b.job_id > 0 && b.elapsed_ms >= 0,
    );
    if (b) claimMs.add(b.elapsed_ms, tags);
  }
}

// The base invariant, per variant: every job claimed exactly once (claims ==
// distinct_jobs), every claim finished (done == claims), both variants ran.
export function teardown(): void {
  everyVariant<VariantStats>(
    variants,
    (v) => v.claims > 0 && v.distinct_jobs === v.claims && v.done === v.claims,
  );
}
