# Deno lessons: layout, shared helpers, and gotchas

Applies to every Deno lesson. `lessons/deno/` is the Deno root the way the
repo root is the Go module root: it holds one `deno.json` (import map) and one
`deno.lock`, the shared helper module `lab/`, and one directory per lesson
with `main.ts`, `analyze.sql`, and the `measurements.csv` it writes. Run from
the repo root: `make run-<name>`, `make analyze-<name>`, `make lab-<name>`.
Set up 2026-09-27 with the first Deno lesson, `cross-store-failure`.

- **Imports resolve through `lessons/deno/deno.json`.** Deno walks up from
  the entry file to find it, so `cd lessons/deno/<name> && deno run -A main.ts`
  needs no per-lesson config. The map has `"lab/": "./lab/"` (relative to the
  `deno.json`), so a lesson writes `import * as postgres from "lab/postgres.ts"`,
  plus one pinned npm specifier per driver (`pg`, `redis`, `@aws-sdk/client-s3`).
  One lockfile means one version per dependency, as with the root `go.sum`.
  Verified 2026-09-27: the import map resolved from a lesson subdirectory in
  a scratch copy and in `make lab-cross-store-failure`.
- **Shared helpers live in `lessons/deno/lab/`, one file per concern,
  mirroring `internal/lab`.** `lab.ts`: `env(key, def)` and the
  `Measurements` class (`new Measurements(...columns)` creates
  `measurements.csv` in the current directory and writes the header,
  `write(...fields)` appends a row, quoting like Go's `encoding/csv`,
  `close()` closes and prints `wrote measurements.csv`). It imports no driver.
  `postgres.ts`: `url()` is `DATABASE_URL` or
  `postgres://trinkets:trinkets@localhost:5432/trinkets`; `connect()` opens
  one `pg.Client`. `valkey.ts`: `url()` is `CACHE_URL` or
  `redis://localhost:6379`; `connect()` creates a node-redis client, connects,
  and pings. `seaweedfs.ts`: `url()` is `OBJECT_STORE_URL` or
  `http://localhost:8333` with access key `trinkets`, secret key
  `trinkets-secret`, region `us-east-1`, path-style; `connect()` builds the
  `S3Client`; `ensureBucket`, `listKeys` (paginated), and `deleteKeys`
  (`DeleteObjects` in batches of 1000) cover setup and reset. The env var is
  the only override, so there is one way to redirect a lesson, same as Go.
- **`pg` ships no types; without a directive `Client` is `any`.** The
  failure is indirect: `deno check` passes on the helper and then reports
  `TS7006 implicitly has an 'any' type` on unrelated callbacks in the lesson,
  because `Promise.all([typed, any])` collapses the tuple. `lab/postgres.ts`
  carries `// @ts-types="npm:@types/pg@^8"` above the import, which types
  every lesson's `Client` and `query<Row>()` results. `redis` and
  `@aws-sdk/client-s3` ship their own types. Verified 2026-09-27.
- **Ping after connect.** node-redis `createClient` does not contact the
  server and queues commands while disconnected, so `valkey.connect()` awaits
  `connect()` and `ping()`. Verified 2026-09-27: a set/get round trip against
  `make up-valkey`, and `CACHE_URL=redis://localhost:1` failed immediately with
  `ECONNREFUSED`.
- **SeaweedFS supports `DeleteObjects`.** One batched request replaced a
  per-key `DeleteObject` loop; the lesson's reconcile step deleted 4 orphans
  in one round trip. Verified 2026-09-27 in `cross-store-failure`.
- **Checking without services:** `make check` runs `go vet ./...` and
  `cd lessons/deno && deno check .`, which type-checks every Deno file
  without running anything. `deno fmt` from `lessons/deno` formats the tree
  (it also rewrites `.md` and `.json` files it finds, so run it there, not
  from the repo root).
- **VS Code:** `.vscode/settings.json` sets `"deno.enablePaths":
  ["./lessons/deno"]` so the `denoland.vscode-deno` extension
  (`.vscode/extensions.json` recommends it) owns that tree and the regular
  TypeScript service leaves it alone. New Deno lessons need no settings edit.
  Reload the window if old diagnostics persist. Editor diagnostics were not
  inspected after the 2026-09-27 restructure.
- **Adding a lesson:** create `lessons/deno/<name>/main.ts` and `analyze.sql`;
  the Makefile discovers `lessons/deno/*/main.ts`. Start `main.ts` with a
  comment naming the services it needs, import from `lab/`, and use top-level
  `await` (no `run()` wrapper: an un-awaited wrapper turns a rejection into an
  unhandled-promise exit instead of a normal error).

See [Deno's VS Code documentation](https://docs.deno.com/runtime/reference/vscode/).
