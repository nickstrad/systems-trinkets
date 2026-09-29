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
| [pipelining-work/core/core.go](../lessons/go/pipelining-work/core/core.go) | `IncrementSequential(ctx, store, keys)` and `IncrementPipelined(ctx, store, keys)` return only an error; `Sum(ctx, store, keys)` reads every counter back with one MGET so runners and adapters verify stored state, not INCR replies. `Store` is three go-redis methods, satisfied by `*redis.Client` |

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
- Every core exports its table DDL (`core.Schema`) so the
  lesson runner and the perf adapter create identical tables; callers still own
  dropping, truncating, and seeding. SQLite's `core.DSN(path, mode, busyMs)`
  builds the modernc DSN with per-connection pragmas; callers own driver
  registration. Roll back the snapshot to release its connection. Allow a second connection
  for the writer. A locked write is `WriteResult.WriteErr`, while connection or
  preparation failures use the function's returned error.
- Pipelining cores take the key list, not a prefix and ids: the caller builds
  keys once and passes the same slice to setup (`Del`), the operation, and
  `Sum`, so they cannot disagree on the format. A pipeline is not atomic: on
  error, earlier INCRs may already be applied. `Sum` counts a missing key as
  zero.
Adapter notes from the 2026-09-28 perf remake (cores unchanged):

- A core whose SQL names its table without a schema (queue, counter) cannot
  serve two variants from one table; the adapter calls `perf.Database` once
  per variant so each has its own schema and pool.
- `core.ProfileKey(id)` fixes the cache key format, so an adapter namespaces
  the ids (a random base) rather than the keys.
- `WriteEvent` reports connection or preparation failures as setup errors;
  under concurrent DELETE-mode writers that step itself hits SQLITE_BUSY
  (a new connection needs SHARED), so an adapter opens and warms every
  connection first.
- `core.Work`'s processing sleep ignores context cancellation; a cancelled
  request holds its row lock for the full work time before rolling back.
Historical Go core extraction kept runners responsible for fixtures and CSV.
Current build verification is recorded in the Go-only baseline entry of
[performance-labs.md](performance-labs.md).
