# Reusing lesson operations

Applies when adding an HTTP adapter or another runner to an existing lesson.
The entry files retain fixture setup, resets, experiment loops, and measurements.
Import the core instead of invoking the entire lesson per request.

| Core source | Callable operations |
|---|---|
| [cache-aside/core/core.go](../lessons/go/cache-aside/core/core.go) | `ReadProfile(ctx, pg, cache, id, ttl)` returns name, source, error; `ProfileKey(id)` shares the cache key with fixture invalidation |
| [background-job-queue/core/core.go](../lessons/go/background-job-queue/core/core.go) | `Work(ctx, db, claimSQL, workTime)` returns `Claim` and error; choose `ClaimQuery` or `SkipLockedQuery` in the adapter |
| [completed-job-counter/core/core.go](../lessons/go/completed-job-counter/core/core.go) | `Apply(ctx, db, strategy, id)` returns total, applied, error; construct strategies with `Naive()` or `Idempotent()` |
| [sqlite-wal-lab/core/core.go](../lessons/go/sqlite-wal-lab/core/core.go) | `OpenSnapshot(ctx, db)` returns a transaction and row count; `WriteEvent(ctx, db)` returns timed write outcome and setup error |
| [cross-store-failure/core.ts](../lessons/deno/cross-store-failure/core.ts) | `createOperations({pg, s3, bucket, prefix, concurrency})` returns `upload`, `measure`, the three reconcilers, and the mode-to-repair map |

Go packages are importable under
`github.com/nickstrad/systems-trinkets/lessons/go/<lesson>/core`.
The Postgres interfaces accept the runner's existing connection or a future
server's pool. Each concurrent transaction needs its own connection. Cores do
not create or close clients, reset fixtures, print, write CSV, or panic on
dependency errors; adapters decide how to report returned errors.

Keep these existing semantics in mind:

- Counter `total` is valid only when `applied` is true. A duplicate returns zero;
  the sequential runner carries forward its last observed total for reporting.
- Queue `Claim.Took` measures the claim query, excluding simulated processing.
  An empty queue returns `pgx.ErrNoRows`. Processing retains the lesson's sleep.
- Every core exports its table DDL (`core.Schema`, or `schema` in Deno) so the
  lesson runner and the perf adapter create identical tables; callers still own
  dropping, truncating, and seeding. SQLite's `core.DSN(path, mode, busyMs)`
  builds the modernc DSN with per-connection pragmas; callers own driver
  registration. Roll back the snapshot to release its connection. Allow a second connection
  for the writer. A locked write is `WriteResult.WriteErr`, while connection or
  preparation failures use the function's returned error.
- Deno core import and factory construction perform no I/O. The caller owns
  client lifecycle and bucket setup. Its fixed metadata `table` and the
  `schema` that creates it are exported for setup; the supplied prefix scopes
  S3 listings, not metadata rows.
  A simulated crash preserves the lesson's early return; it is not a thrown
  dependency failure. Reconciliation still follows the lesson's assumptions
  about in-flight work and grace windows.

Verified 2026-09-27: `make check` passed (Go vet and Deno type checking); importing
`core.ts` with `deno eval --cached-only` completed without running the lesson;
`make -n` retained all five run commands and all three service startup commands.
The Makefile and CSV analysis scripts were not changed by this refactor.
Full experiments against backing services were not rerun for this change.
