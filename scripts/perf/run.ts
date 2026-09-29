#!/usr/bin/env -S deno run -A
// Local performance lifecycle: serve, run, analyze, or the combined lab.
//   deno run -A scripts/perf/run.ts <serve|run|analyze|lab> lessons/go/<lesson>
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname!, "../..");
const SHARED_SQL = path.join(ROOT, "scripts/perf/analyze.sql");
const ACTIONS = ["serve", "run", "analyze", "lab"];

// Failure is an expected, explained exit; anything else is a real crash.
class Failure extends Error {}

function exists(p: string): boolean {
  try {
    Deno.statSync(p);
    return true;
  } catch {
    return false;
  }
}

function require(tool: string) {
  const dirs = (Deno.env.get("PATH") ?? "").split(path.delimiter);
  if (!dirs.some((dir) => exists(path.join(dir, tool)))) {
    throw new Failure(
      `${tool} is required; install it before running this target`,
    );
  }
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function command(
  argv: string[],
  options: Omit<Deno.CommandOptions, "args"> = {},
): Deno.Command {
  return new Deno.Command(argv[0], { args: argv.slice(1), ...options });
}

async function outputOf(argv: string[], env?: Record<string, string>) {
  const { code, stdout } = await command(argv, { env, stderr: "inherit" })
    .output();
  if (code !== 0) throw new Failure(`${argv.join(" ")} exited with ${code}`);
  return new TextDecoder().decode(stdout).trim();
}

async function fetchOk(url: string, timeoutMs: number): Promise<Response> {
  const response = await fetch(url, { signal: AbortSignal.timeout(timeoutMs) });
  if (!response.ok) throw new Failure(`${url} answered ${response.status}`);
  return response;
}

// copy forwards a child's output stream to every writer, in order per stream.
async function copy(
  readable: ReadableStream<Uint8Array>,
  ...writers: WritableStreamDefaultWriter<Uint8Array>[]
) {
  for await (const chunk of readable) {
    for (const writer of writers) await writer.write(chunk);
  }
}

async function analyze(lesson: string, output: string): Promise<number> {
  require("duckdb");
  if (!exists(path.join(output, "metrics.csv"))) {
    throw new Failure(`no metrics.csv to analyze in ${output}`);
  }
  let sql = await Deno.readTextFile(SHARED_SQL);
  const extra = path.join(lesson, "perf/analyze.sql");
  if (exists(extra)) sql += await Deno.readTextFile(extra);
  const duckdb = command(["duckdb"], {
    cwd: output,
    stdin: "piped",
    stdout: "inherit",
    stderr: "inherit",
  }).spawn();
  const stdin = duckdb.stdin.getWriter();
  await stdin.write(new TextEncoder().encode(sql));
  await stdin.close();
  return (await duckdb.status).code;
}

// latestRun picks the newest run that produced metrics; a run that failed
// before traffic keeps its directory (and server.log) but is not analyzable.
function latestRun(results: string): string {
  const runs = exists(results)
    ? [...Deno.readDirSync(results)].filter((e) =>
      e.isDirectory && exists(path.join(results, e.name, "metrics.csv"))
    ).map((e) => e.name).sort()
    : [];
  if (runs.length === 0) throw new Failure(`no runs to analyze in ${results}`);
  return path.join(results, runs.at(-1)!); // run ids start with a timestamp
}

function runId(): string {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${
    pad(now.getDate())
  }-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  return `${stamp}-${crypto.randomUUID().replaceAll("-", "").slice(0, 8)}`;
}

async function main(): Promise<number> {
  const [action, target] = Deno.args;
  if (!ACTIONS.includes(action) || !target) {
    throw new Failure(`usage: run.ts <${ACTIONS.join("|")}> <lesson dir>`);
  }
  const lesson = path.resolve(ROOT, target);
  if (!exists(path.join(lesson, "perf/k6.ts"))) {
    throw new Failure(`no performance workload: ${lesson}`);
  }
  const results = path.join(lesson, "perf/results");
  if (action === "analyze") {
    const override = Deno.env.get("RESULTS_DIR");
    return analyze(
      lesson,
      override ? path.resolve(override) : latestRun(results),
    );
  }
  if (action !== "serve") require("k6");
  if (action === "lab") require("duckdb");
  const env = Deno.env.toObject();
  let serverArgv: string[] = [];
  if (action !== "run") {
    require("go");
    // One binary per lesson; run directories hold only logs and metrics.
    await Deno.mkdir(results, { recursive: true });
    const binary = path.join(results, "server");
    const build = await command(["go", "build", "-o", binary, "./perf"], {
      cwd: lesson,
      stdout: "inherit",
      stderr: "inherit",
    }).output();
    if (build.code !== 0) throw new Failure("go build failed");
    serverArgv = [binary];
  }

  if (action === "serve") {
    // Ctrl-C reaches the server through the terminal's process group; keep this
    // process alive until the server has drained so make sees its exit status.
    Deno.addSignalListener("SIGINT", () => {});
    const server = command(serverArgv, {
      cwd: lesson,
      env,
      stdin: "inherit",
      stdout: "inherit",
      stderr: "inherit",
    }).spawn();
    return (await server.status).code;
  }
  const output = path.join(results, runId());
  await Deno.mkdir(output, { recursive: true });
  let server: Deno.ChildProcess | undefined;
  let serverExited = false;
  let serverLog: Promise<void> | undefined;
  let logWriter: WritableStreamDefaultWriter<Uint8Array> | undefined;
  try {
    if (action === "lab") {
      env.PORT = "0";
      env.READY_FILE = path.join(output, "ready.txt");
      const log = await Deno.open(path.join(output, "server.log"), {
        write: true,
        create: true,
        truncate: true,
      });
      logWriter = log.writable.getWriter();
      server = command(serverArgv, {
        cwd: lesson,
        env,
        stdout: "piped",
        stderr: "piped",
      }).spawn();
      server.status.then(() => serverExited = true);
      serverLog = Promise.all([
        copy(server.stdout, logWriter),
        copy(server.stderr, logWriter),
      ]).then(() => {});
      const deadline = performance.now() + 45_000;
      while (!exists(env.READY_FILE)) {
        if (serverExited) {
          throw new Failure(`server exited; see ${output}/server.log`);
        }
        if (performance.now() >= deadline) {
          throw new Failure(
            `server readiness timed out; see ${output}/server.log`,
          );
        }
        await sleep(100);
      }
      env.BASE_URL = (await Deno.readTextFile(env.READY_FILE)).trim();
    }
    env.BASE_URL ||= "http://127.0.0.1:8080";
    // The server reports the settings it actually read; workload settings land in workload.json.
    const health = await (await fetchOk(env.BASE_URL + "/health", 5_000))
      .json();
    const settings = {
      base_url: env.BASE_URL,
      fixtures: server
        ? "fresh isolated server fixtures"
        : "existing server; state is reused",
      server: health.settings ?? {},
      k6_version: await outputOf(["k6", "version"]),
    };
    await Deno.writeTextFile(
      path.join(output, "settings.json"),
      JSON.stringify(settings, null, 2) + "\n",
    );
    const script = path.join(lesson, "perf/k6.ts");
    env.K6_CSV_TIME_FORMAT = "unix";
    await Deno.writeTextFile(
      path.join(output, "workload.json"),
      await outputOf(
        ["k6", "inspect", "--include-system-env-vars", script],
        env,
      ) + "\n",
    );
    console.log(`Results: ${output}`);
    // Save k6's output and show it live. Ctrl-C reaches k6 through the terminal;
    // this process then finishes the run's bookkeeping and keeps k6's status.
    Deno.addSignalListener("SIGINT", () => {});
    const k6 = command(["k6", "run", "--out", "csv=metrics.csv", script], {
      cwd: output,
      env,
      stdout: "piped",
      stderr: "piped",
    }).spawn();
    const k6Log = await Deno.open(path.join(output, "k6.log"), {
      write: true,
      create: true,
      truncate: true,
    });
    const k6Writer = k6Log.writable.getWriter();
    const terminal = Deno.stdout.writable.getWriter();
    await Promise.all([
      copy(k6.stdout, k6Writer, terminal),
      copy(k6.stderr, k6Writer, terminal),
    ]);
    terminal.releaseLock();
    await k6Writer.close();
    let status = (await k6.status).code;
    try {
      const stats = await fetchOk(env.BASE_URL + "/stats", 30_000);
      await Deno.writeFile(
        path.join(output, "domain.json"),
        new Uint8Array(await stats.arrayBuffer()),
      );
    } catch (error) {
      console.error(`Could not capture final domain state: ${error}`);
      status ||= 1;
    }
    if (action === "lab") {
      // Analyze failed runs too; k6's status still wins.
      const sqlStatus = await analyze(lesson, output);
      status ||= sqlStatus;
    }
    return status;
  } finally {
    if (server) {
      if (!serverExited) server.kill("SIGTERM");
      const stopped = await Promise.race([
        server.status.then(() => true),
        sleep(30_000).then(() => false),
      ]);
      if (!stopped) {
        server.kill("SIGKILL");
        await server.status;
        console.error(
          "Server forced to stop; inspect server.log for fixture cleanup.",
        );
      }
      await serverLog;
      await logWriter?.close();
    }
  }
}

try {
  Deno.exit(await main());
} catch (error) {
  if (error instanceof Failure) {
    console.error(error.message);
    Deno.exit(1);
  }
  throw error;
}
