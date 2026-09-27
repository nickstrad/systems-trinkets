# Postgres from Go (pgx): gotchas

Applies when a lesson talks to the local Postgres with `github.com/jackc/pgx/v5`
(see `lessons/go/completed-job-counter/main.go`). Connect with
`postgres.Connect(ctx)` from `internal/lab/postgres`, which reads
`DATABASE_URL` and defaults to
`postgres://trinkets:trinkets@localhost:5432/trinkets`.

- **First call includes statement preparation.** pgx's default mode prepares
  and caches each new SQL text on first use, so the first timed call of a code
  path is slower. Run each path once, reset the tables, then measure.
  (Reasoned from pgx's documented behavior, not measured separately.)
- **Multi-statement `Exec` only without arguments.** A DDL block with several
  statements works in one `db.Exec`; a statement with `$1` parameters must be
  its own `Exec` (extended protocol allows one statement).
- **`$1` is for values only, never identifiers.** `drop table if exists $1` or
  `select ... from $1` fails with `syntax error at or near "$1"` (SQLSTATE
  42601): the query is parsed before arguments arrive, so table and column
  names must be in the SQL text. Write constant names in directly; quote a
  dynamic name with `pgx.Identifier{name}.Sanitize()` before splicing it in.
  Verified 2026-09-26 in `lessons/go/background-job-queue/main.go`.
- **A blocked `for update` re-checks the row after waiting.** When a second
  transaction's `select ... where status = 'pending' ... for update` blocks
  on a row the first holds, and the first sets that row to `done` and commits,
  the waiter does not return the stale row: it re-evaluates the `where` on
  the committed version, skips it, and locks the next matching row. So N
  workers all selecting the oldest pending row form a queue and are served
  one per commit: the k-th claim waits about k times the hold time, and the
  slowest claim grows linearly with N. `skip locked` gives each worker a
  different row at once, so its slowest claim stays flat. Separate
  connections are required to see this; one connection cannot block itself.
  Verified 2026-09-26 in `lessons/go/background-job-queue` with 1, 2, 4, and 8
  workers holding rows for 100 ms: slowest blocking claim 0.6, 107, 315, and
  730 ms; slowest skip locked claim 0.6 to 1.3 ms; every job claimed once in
  both modes.
- **Read a changed value in the same round trip:** `update ... returning total`
  with `QueryRow(...).Scan`, and treat `pgx.ErrNoRows` as "row missing".

Verified 2026-09-24: `completed-job-counter` ran end to end (naive total 300,
idempotent 100) using these patterns.
