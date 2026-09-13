# Counter — invariants

Status: **01–05 agreed 2026-09-12; 06 (crash) proposed 2026-09-12** — two "should" invariants (monotonic reads per client, no half-applied non-2xx) were dropped and can return as 07/08 later. Every `check.Invariant`
in `counter_test.go` cites an ID from this table; a failure points back here
to the primitive that was supposed to guarantee it.

Guide row: *"No increment is lost under concurrent writers."* Primitives:
Valkey `INCR`/`INCRBY`; SQLite `INTEGER` + `UPDATE` inside a transaction;
PostgreSQL `BIGINT` + atomic `UPDATE … RETURNING`.

| ID | Invariant | Guaranteed by | Kind | Must/should | Status |
|----|-----------|---------------|------|-------------|--------|
| INV-COUNTER-01 | **No lost update.** After N concurrent `incr` calls that returned 2xx, `GET` returns exactly N (sum of deltas). | Store's atomic read-modify-write: `INCR`; `UPDATE … SET v = v + ?` in a txn; `UPDATE … RETURNING`. | concurrency | must | agreed |
| INV-COUNTER-02 | **Returned values are unique and complete.** The `value`s returned by N concurrent `incr` (delta 1) calls on one counter are a permutation of `1..N`. | Same primitive; additionally that the SUT returns the store's post-increment result, not a separate read. | concurrency | must | agreed |
| INV-COUNTER-03 | **Sequential correctness.** From one client: `incr` returns previous + delta; `GET` equals the last `incr` response; unknown counters read 0. | Application code (the contract) on top of the primitive. | contract | must | agreed |
| INV-COUNTER-04 | **Reset and delete are complete and synchronous.** After `POST /_reset` returns 204, every counter reads 0 and a subsequent `incr` returns `delta`; after `DELETE /counters/{name}` returns 204, that name reads 0 and other names are unchanged. | `FLUSHALL` / `TRUNCATE` / file delete, `DEL` / `DELETE WHERE name=?`, run to completion before responding. | contract | must | agreed |
| INV-COUNTER-05 | **Isolation between names.** Concurrent increments to counter A never change counter B; each name's final value equals its own 2xx increments. | Keying: one key/row per name. | concurrency | must | agreed |
| INV-COUNTER-06 | **Acknowledged increments survive a crash.** After the SUT is killed (SIGKILL) and restarted mid-load, the final value is bounded by `2xx-before-kill ≤ final ≤ 2xx-before-kill + errored`: every acknowledged increment survives, and no increment is acknowledged before it is committed. Requests that errored around the kill are ambiguous (applied or not), hence the bound. | Application discipline: respond only after the store's atomic write has returned; the store's own persistence (SQLite file, Valkey/PostgreSQL server processes, which survive a kill of the SUT). Not applicable to the memory engine, whose state *is* the process. | crash | must | proposed 2026-09-12 (wording from test-plan.md §10a R4) |

Interview answers (§8 of test-plan.md), recorded so a future session knows why:

1. *Invariant to preserve:* no lost update (01); the value returned from an increment is the true post-increment value (02).
2. *Primitive per engine:* Valkey `INCR`/`INCRBY` (single-threaded command); SQLite `UPDATE … SET v=v+? WHERE name=?` inside `BEGIN IMMEDIATE` (or an `INSERT … ON CONFLICT DO UPDATE … RETURNING`); PostgreSQL `INSERT … ON CONFLICT (name) DO UPDATE SET v = counters.v + EXCLUDED.v RETURNING v` (row lock).
3. *Concurrent access:* the test releases N goroutines on a barrier against one name (01, 02) and against several names (05).
4. *Crash between steps:* INV-06 — a kill mid-`incr` must not leave a partially applied increment, and nothing acknowledged may be lost; on restart the value is bounded by the 2xx count before the kill and that count plus the in-flight (errored) requests. Tested only against targets the harness starts itself (`cmd` in the target file); skips otherwise, and on the memory engine. `--bug write-behind` is the reference violation.
5. *Retries / duplicates:* out of scope for a plain counter — a retried `incr` counts twice by design. (Idempotent increments are the idempotency-key pattern.) *Expiration:* none in this contract (optional `EXPIRE` in the guide is not tested).
6. *Store vs convention:* 01, 02, 05 come from the store primitive; 03, 04 are application convention. Monotonic reads per client depends on deployment (single node) and was left out of Phase 1.
7. *Performance:* record throughput (`incr/s`) and latency p50/p95/p99 per run under the concurrency test; no `[expect]` thresholds by default.

Metrics recorded (not asserted): `incr_throughput` (req/s). Latency percentiles and error counts come from the `samples` table via `harness report --query latency`, not from the suite.
