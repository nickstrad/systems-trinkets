// Shared k6 workload: bounded profiles, request helpers, checks, thresholds,
// and the summary. k6 runs this TypeScript directly; Deno only type-checks it
// against @types/k6 (see the root deno.json).
import http, { type Response } from "k6/http";
import { check, sleep } from "k6";
import type { Options, Scenario } from "k6/options";

export const base: string = __ENV.BASE_URL || "http://127.0.0.1:8080";
if (!/^http:\/\/(127\.0\.0\.1|localhost):[0-9]+$/.test(base)) {
  throw new Error("BASE_URL must be a local HTTP listener");
}
const profile: string = __ENV.PROFILE || "smoke";

function number(
  name: string,
  fallback: number,
  min: number,
  max: number,
): number {
  const n = Number(__ENV[name] || fallback);
  if (!Number.isInteger(n) || n < min || n > max) {
    throw new Error(`${name} must be ${min}..${max}`);
  }
  return n;
}

function optionsFor(): Options {
  const rate = number("RATE", 5, 1, 100);
  const maxVUs = number("MAX_VUS", 32, 1, 128);
  const defaultDuration: Record<string, number> = { smoke: 10, soak: 300 };
  const duration = number(
    "DURATION_S",
    defaultDuration[profile] ?? 60,
    1,
    3600,
  );
  const arrival = {
    preAllocatedVUs: Math.min(8, maxVUs),
    maxVUs,
    timeUnit: "1s",
    gracefulStop: "15s",
  };
  const profiles: Record<string, Scenario> = {
    smoke: {
      executor: "constant-vus",
      vus: 1,
      duration: `${duration}s`,
      gracefulStop: "15s",
    },
    load: {
      ...arrival,
      executor: "constant-arrival-rate",
      rate,
      duration: `${duration}s`,
    },
    soak: {
      ...arrival,
      executor: "constant-arrival-rate",
      rate,
      duration: `${duration}s`,
    },
    stress: {
      ...arrival,
      executor: "ramping-arrival-rate",
      startRate: rate,
      stages: [
        { duration: "30s", target: rate },
        { duration: "5s", target: rate * 2 },
        { duration: "30s", target: rate * 2 },
        { duration: "5s", target: rate * 4 },
        { duration: "30s", target: rate * 4 },
        { duration: "5s", target: rate },
        { duration: "30s", target: rate },
      ],
    },
    spike: {
      ...arrival,
      executor: "ramping-arrival-rate",
      startRate: rate,
      stages: [
        { duration: "20s", target: rate },
        { duration: "1s", target: rate * 8 },
        { duration: "10s", target: rate * 8 },
        { duration: "1s", target: rate },
        { duration: "30s", target: rate },
      ],
    },
  };
  const scenario = profiles[profile];
  if (!scenario) {
    throw new Error("PROFILE must be smoke, load, stress, spike, or soak");
  }
  return {
    scenarios: { [profile]: scenario },
    thresholds: {
      checks: ["rate==1"],
      "http_reqs{name:operation}": ["count>0"],
      "http_req_failed{name:operation}": ["rate<0.01"],
      "http_req_duration{name:operation}": [
        `p(95)<${number("P95_MS", 1500, 1, 60000)}`,
      ],
      dropped_iterations: ["count==0"],
    },
  };
}

// Lessons re-export these two so k6 picks them up from the lesson script.
export const options: Options = optionsFor();

// k6 passes the end-of-test summary as plain data; only these fields are read.
interface SummaryData {
  metrics: Record<string, { values?: Record<string, number> }>;
}
export function handleSummary(data: SummaryData): Record<string, string> {
  const checks = data.metrics.checks?.values;
  const p95 = data.metrics["http_req_duration{name:operation}"]?.values
    ?.["p(95)"];
  return {
    "summary.json": JSON.stringify(data, null, 2),
    stdout: `Checks: ${
      checks ? `${checks.passes} passed, ${checks.fails} failed` : "no samples"
    }. Operation p95: ${
      p95 === undefined ? "no samples" : p95.toFixed(2) + " ms"
    }.\n`,
  };
}

export function request(method: string, path: string): Response {
  return http.request(method, base + path, null, {
    tags: { name: "operation" },
    timeout: "12s",
  });
}
// body trusts the adapter's JSON shape; the invariant predicates verify it.
export function body<T extends object>(response: Response): T {
  try {
    return response.json() as T;
  } catch {
    return {} as T;
  }
}
export function assertResponse<T extends object>(
  response: Response,
  predicate: (b: T) => boolean,
): void {
  check(response, {
    "operation succeeds": (r) => r.status === 200,
    "operation invariant": () => predicate(body<T>(response)),
  });
  if (profile === "smoke") sleep(0.1);
}
// repair logs the state a lesson's reconciler starts from, then runs it.
export function repair(): void {
  const before = http.get(base + "/stats", {
    tags: { name: "stats" },
    timeout: "30s",
  });
  check(before, { "pre-repair stats succeeds": (r) => r.status === 200 });
  console.log("Before repair:", before.body);
  const result = http.post(base + "/repair", null, {
    tags: { name: "repair" },
    timeout: "30s",
  });
  check(result, { "repair succeeds": (r) => r.status === 200 });
}
export function stats<T extends object>(predicate: (b: T) => boolean): void {
  const result = http.get(base + "/stats", {
    tags: { name: "stats" },
    timeout: "30s",
  });
  check(result, {
    "stats succeeds": (r) => r.status === 200,
    "final invariant": () => predicate(body<T>(result)),
  });
  console.log("Final domain state:", result.body);
}
