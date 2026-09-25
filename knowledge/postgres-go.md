# Postgres from Go (pgx): gotchas

Applies when a lesson talks to the local Postgres with `github.com/jackc/pgx/v5`
(see `lessons/completed-job-counter/main.go`). Connect with
`postgres://trinkets:trinkets@localhost:5432/trinkets`.

- **First call includes statement preparation.** pgx's default mode prepares
  and caches each new SQL text on first use, so the first timed call of a code
  path is slower. Run each path once, reset the tables, then measure.
  (Reasoned from pgx's documented behavior, not measured separately.)
- **Multi-statement `Exec` only without arguments.** A DDL block with several
  statements works in one `db.Exec`; a statement with `$1` parameters must be
  its own `Exec` (extended protocol allows one statement).
- **Read a changed value in the same round trip:** `update ... returning total`
  with `QueryRow(...).Scan`, and treat `pgx.ErrNoRows` as "row missing".

Verified 2026-09-24: `completed-job-counter` ran end to end (naive total 300,
idempotent 100) using these patterns.
