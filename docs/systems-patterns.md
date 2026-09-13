# Systems Patterns with Valkey, SQLite, and PostgreSQL

This guide is the readable companion to the executable catalog in
[`cli/seed_catalog.go`](../cli/seed_catalog.go). That Go file is the source of truth for
seeded names, slugs, explanations, invariants, readings, and the three engine
sketches. The catalog has exactly 29 patterns, three engines, and one core
approach per pattern/engine pair.

Run `trinkets seed` to load missing rows. Use `trinkets seed --update` to
refresh seed-owned fields on existing rows. The explicit reconciliation mode
(`seed --update --prune`) is required to rename and retire the old catalog;
ordinary seeding never removes user rows.

## Ordered curriculum and contracts

The invariant text below is intentionally concrete. It is the contract an HTTP
implementation should expose and a future harness should check. Claim order is
not completion order, delivery is not exactly-once external side effects, and a
storage primitive alone does not establish the whole application contract.

| # | Slug | Pattern | Family | Contract |
|---:|---|---|---|---|
| 1 | `counter` | Atomic counter | counting | No increment is lost under concurrent writers; each successful increment returns a value consistent with one serial order. |
| 2 | `optimistic-concurrency` | Optimistic concurrency / versioned writes | state | A write succeeds only when its expected version is current; conflicting writes cannot both overwrite one version; a successful write advances the version once. |
| 3 | `state-machine` | Guarded state machine | state | Only declared transitions succeed; the source state is checked at mutation time; repeated transitions return the documented current-state result without a second effect. |
| 4 | `idempotency-key` | Idempotency key | idempotency | One key binds to one request identity and business effect; concurrent and later retries return the stored outcome; the binding survives the promised restart boundary. |
| 5 | `inbox` | Inbox / idempotent consumer | idempotency | A delivery identity is processed at most once within the atomicity boundary; duplicates obtain the stored outcome; distinct identities remain distinct. |
| 6 | `expiring-reservation` | Expiring reservation | idempotency | At most one unexpired reservation holds a resource; expired pending reservations become available; completion requires the current token and persists a terminal state. |
| 7 | `fifo-queue` | Reliable FIFO work queue | queueing | Each claim selects the minimum enqueue sequence among jobs eligible at that instant; each job has at most one live claim; unacknowledged claims recover; acknowledged jobs are not reissued. |
| 8 | `lease` | Lease | coordination | At most one unexpired owner holds a lease; only its token renews or releases it; an abandoned lease becomes acquirable after its deadline. |
| 9 | `fencing-tokens` | Fencing tokens | coordination | Each grant has a greater token; the protected resource rejects stale tokens; issuing a token without checking it at the protected mutation is insufficient. |
| 10 | `task-ownership` | Task ownership and recovery | coordination | A live task has one owner token; only that owner completes it; completed work stays complete; expired/orphaned claims become safely claimable. |
| 11 | `semaphore` | Semaphore / bounded concurrency | coordination | No more than N permits are live; a permit is released at most once; a crashed holder's permit eventually returns. |
| 12 | `delayed-queue` | Delayed queue / scheduler | queueing | No job is claimed before its due time; every due job eventually becomes claimable; one worker holds a live claim for a due job. |
| 13 | `retry-lifecycle` | Retry and dead-letter lifecycle | queueing | Backoff is respected; attempt and state changes are atomic; poison jobs stop at their limit; redrive is explicit and does not silently duplicate a completed effect. |
| 14 | `priority-queue` | Priority queue with fairness | queueing | Eligible claims use weighted round robin in an 8 high : 2 normal : 1 low schedule; each class is FIFO; a continuously nonempty low class receives one claim in every eleven; each job has at most one live claim. |
| 15 | `rate-limiter` | Comparative rate limiter | rate-limiting | Fixed-window, sliding-window, and token-bucket admission each respect their configured policy; concurrent decisions are atomic for every algorithm. |
| 16 | `heartbeat` | Heartbeat and failure detector | coordination | A timely heartbeat keeps a worker live; silence makes it suspected after the timeout; re-registration uses an identity/epoch that prevents two processes from sharing one live identity. |
| 17 | `leader-election` | Leader election | coordination | At most one unexpired leader is visible; only the current token renews; takeover follows expiry; leader work uses fencing where stale mutation is possible. |
| 18 | `notification-vs-delivery` | Ephemeral notification vs durable delivery | messaging | Connected subscribers receive live notifications; disconnected subscribers may miss them; required delivery comes from durable state; notifications do not promise exactly-once effects. |
| 19 | `outbox` | Transactional outbox | messaging | Business state and its outbox record commit together; every committed record is eventually delivered with fair retries; dispatch may redeliver and consumers use message identity. |
| 20 | `saga` | Saga / compensating transaction | messaging | Step outcomes are durable; partial failure runs defined compensations in order; retries resume from progress and use idempotent step requests; independent stores are not one transaction. |
| 21 | `durable-event-log` | Durable event log | log | SQL committed positions are contiguous and in commit order; Valkey Stream IDs are unique/increasing and may have gaps; cursors cannot skip late commits; restart durability follows configuration. |
| 22 | `consumer-checkpoints` | Consumer checkpoints / offsets | log | Progress survives restart; a checkpoint cannot pass an unprocessed record; recovery may replay but cannot lose records before the committed checkpoint. |
| 23 | `consumer-groups` | Consumer group | log | A record/partition has one live group owner; abandoned work is reassignable; acknowledgement follows the local effect; redelivery is possible and handled. |
| 24 | `incremental-projection` | Incremental projection / read model | state | Derived state records its source position; duplicate application is harmless; rebuilding from the same log gives the same result. |
| 25 | `snapshots-compaction` | Snapshots and log compaction | log | A snapshot names its included position; snapshot plus later events equals full replay; compaction retains records required by the oldest consumer or recovery policy. |
| 26 | `cache-consistency` | Cache consistency and stampede protection | caching | Freshness ends at `fresh_until`; one live fenced owner publishes refresh and waiters share its outcome; failed refresh may serve stale only through `fresh_until + 30s`; expired/replaced owners cannot publish. |
| 27 | `distributed-id` | Distributed ID generation | counting | IDs are unique across concurrency/restart; one generator increases locally; each leased range is 1,024 IDs and abandoned values are never reused; allocation fails before signed 64-bit overflow; no global return or clock ordering is promised. |
| 28 | `sharded-counter` | Sharded/hot-key counter | counting | Operation identity hashes modulo 64; Valkey totals are eventually consistent while SQL totals are exact in one read snapshot; retries do not double count; each shard remains atomic. |
| 29 | `partition-rebalancing` | Partition assignment and rebalancing | coordination | A partition has at most one live assignment per epoch; membership changes eventually redistribute without overlap; stale epochs are rejected; crash recovery permits reassignment. |

## Core map: the reviewed engine sketches

Each cell below summarizes the core approach seeded for that pattern and
engine. These are design sketches, not claims that this repository already
implements every exercise runtime.

| # / pattern | Valkey | SQLite | PostgreSQL |
|---|---|---|---|
| 1 `counter` | `INCR`/`INCRBY` atomically mutates and returns the counter. | Atomic `UPDATE value = value + ?` in a transaction; return after commit. | Atomic `UPDATE ... RETURNING` under row locking. |
| 2 `optimistic-concurrency` | Hash plus `WATCH`/`MULTI`/`EXEC` or Lua compare-and-set. | Guarded update by expected version; zero rows is a conflict. | Guarded update with `RETURNING`. |
| 3 `state-machine` | Lua validates and records each transition atomically. | Guarded update in an `IMMEDIATE` transaction; optional checks/triggers. | Row lock or guarded update, with transition record in the transaction. |
| 4 `idempotency-key` | Lua binds key, effect, and status/body; bare `SET NX` is insufficient. | Unique key, business mutation, and stored outcome commit together. | Unique key row and business mutation commit together; `ON CONFLICT` returns the outcome. |
| 5 `inbox` | Stream plus Lua records message ID and effect/outcome before acknowledgement. | Unique inbox row, domain mutation, and outcome commit together. | Inbox identity, effect, and result are written in one transaction. |
| 6 `expiring-reservation` | Lua atomically creates token/deadline and token-checks completion. | Unique resource row with guarded expiry reclaim and completion. | Guard token, pending state, and unexpired deadline; persist completion, and reclaim only expired pending rows. |
| 7 `fifo-queue` | Committed sequence, ready list, processing claim, and recovery are Lua operations. | Queue-head transaction serializes sequence/claim; token/deadline guards acknowledgement and expired unacknowledged recovery. | Queue-head lock serializes FIFO; token/deadline guards acknowledgement and expired unacknowledged requeue; stale tokens fail. |
| 8 `lease` | `SET NX PX`, token-checked Lua renewal/release. | Lease row with guarded transactional acquire/renew/release. | Lease row with `FOR UPDATE` and guarded updates. |
| 9 `fencing-tokens` | Lua increments and activates a fence; resource mutations reject older fences. | Grant and protected-row fence update transactionally. | Locked counter and protected-row fence check on every mutation. |
| 10 `task-ownership` | Hash plus Lua claim, heartbeat, completion, and orphan recovery. | State/owner/deadline guarded in an `IMMEDIATE` transaction. | Ordered `FOR UPDATE SKIP LOCKED` claim and token-checked recovery. |
| 11 `semaphore` | Lua counts unexpired tokens, grants below N, and makes release idempotent. | Permit rows with expiry; reap/count/insert transactionally. | Permit table with row/advisory locking around count-and-insert. |
| 12 `delayed-queue` | Sorted due set and Lua claim/recovery. | Indexed `run_at` and transactional due claim. | `run_at` index, `FOR UPDATE SKIP LOCKED`, and timeout recovery. |
| 13 `retry-lifecycle` | Lua atomically increments attempts and moves to due or terminal state. | Guarded attempt, next-time, status, and explicit redrive. | Guarded lifecycle updates and `SKIP LOCKED` due claims. |
| 14 `priority-queue` | Per-class FIFO lists and a shared 8:2:1 schedule cursor in Lua. | FIFO sequence and cursor update in one `IMMEDIATE` transaction. | Lock cursor, select lowest sequence in class, advance, and claim together. |
| 15 `rate-limiter` | Lua implements fixed-window, sliding-window, and token-bucket admission atomically. | Transactional bucket rows, timestamp events, and elapsed-time token rows. | Upsert counters, indexed timestamps, and locked token row. |
| 16 `heartbeat` | TTL identity/epoch key and matching-epoch Lua renewal. | Worker identity/epoch/last-seen rows and one clock convention. | Transactional epoch compare-and-set and last-seen index. |
| 17 `leader-election` | TTL lease plus fencing counter; protected work checks its fence. | Leader token/fence/expiry and guarded protected mutations. | Durable lease/fence; advisory locks do not replace persisted checks. |
| 18 `notification-vs-delivery` | Pub/Sub is an ephemeral wakeup; Stream/key is durable delivery. | Durable events plus polling/notifier wakeup hint. | `LISTEN`/`NOTIFY` wakes clients; durable events are the source of truth. |
| 19 `outbox` | Lua writes Valkey business state and Stream record together; SQL-owned outbox publishes separately. | Business and outbox rows share one transaction; dispatcher tolerates redelivery. | Business/outbox rows share one ACID transaction; `SKIP LOCKED` dispatcher uses message ID. |
| 20 `saga` | Saga hash/Stream and `saga_id/step` inbox identity make retries safe. | Saga, step, outbox, and inbox commit local progress together. | Durable saga/step/outbox/inbox orchestrates participants and compensation. |
| 21 `durable-event-log` | Stream server IDs are ordered replay cursors; numeric gaps are allowed. | Singleton log head and event insert commit together. | Lock singleton head and insert before commit; do not use a bare sequence as commit order. |
| 22 `consumer-checkpoints` | Advance after durable processing; next is the next existing Stream ID. | Effect and contiguous checkpoint advance in one transaction. | Checkpoint advances only from `n` to `n+1`; never skips a hole. |
| 23 `consumer-groups` | Stream groups, pending entries, post-effect acknowledgement, and idle recovery. | Assignment/claim/checkpoint tables plus inbox deduplication. | Lease/claim tables with `SKIP LOCKED` and expired-owner recovery. |
| 24 `incremental-projection` | Lua records event ID, updates projection, and advances position. | Unique marker, derived update, and checkpoint share a transaction. | Unique marker, projection update, and source position share a transaction. |
| 25 `snapshots-compaction` | Snapshot Stream ID and consumer minima determine safe `XTRIM`. | Snapshot/position and consumer minima bound deletes. | Snapshot metadata and offsets bound partition/delete retention. |
| 26 `cache-consistency` | `stale_until = fresh_until + 30s`; fenced lease, guarded publish, and waiter coalescing. | Same deadlines/token/fence with transactional ownership and publication. | Locked row and guarded publication reject expired owners; waiters share outcome. |
| 27 `distributed-id` | `INCRBY 1024` reserves a range; persist next ID and abandon remainder after restart. | Guard head ≤ 9223372036854774783 before adding 1,024; integer CHECK rejects overflow promotion; persist before issue. | Lock `id_head`, commit a 1,024-ID lease, and make no clock-order claim. |
| 28 `sharded-counter` | Hash identity to 64 shards; Lua deduplicates and increments; sum is eventual. | Unique operation and shard increment in one transaction; sum one snapshot. | Increment only when identity `INSERT ... RETURNING` produces a new row; duplicate identities do not increment; sum one snapshot. |
| 29 `partition-rebalancing` | TTL members plus epoch/fence assignments; Lua fences replacements. | Serialized rebalance and stale-epoch rejection. | Transactional assignment epoch and current epoch/owner check. |

## Shared durability and harness scope

For every Valkey restart guarantee, the catalog assumes AOF with
`appendfsync always`, no eviction for durable keys, and one primary. This does
not promise lossless failover. SQLite and PostgreSQL still require explicit
application transaction boundaries and recovery policies. Claims, leases,
notifications, and dispatchers provide an atomicity boundary; they do not make
external side effects exactly once.

The current harness has the counter concurrency suite and the current toolkit.
Process crash/restart control and operation histories are planned capabilities;
multiple independently failing nodes are deferred beyond the current harness.
The later exercises describe what should be observable over HTTP but do not
imply that their suites already exist. Outbox and saga require controlled
failure points or independently failing stores to test their central behavior.

Recommended stopping points are after #7 for atomicity and concurrency, #17 for
ownership and recovery, #25 for durable messaging and replay, and #29 for
scaling and worker assignment.

## Questions for every implementation

1. What invariant must the system preserve?
2. Which primitive actually provides that guarantee?
3. What happens under concurrent access?
4. What happens if a process crashes between steps?
5. How are retries, duplicate work, ordering, expiration, and recovery handled?
6. Which guarantees come from the store, and which are application rules?
7. What are the contention, durability, operational, and scaling tradeoffs?

The goal is to compare how the same behavior emerges from different primitives,
then record the result in `approach` and `attempt` rows. The key question is:
**what primitive is actually providing the guarantee?**

## Pattern readings

The exact title and URL pairs are seeded from `cli/seed_catalog.go`; the links below
provide the same quick map for readers.

- `counter`: [Valkey INCR](https://valkey.io/commands/incr/), [PostgreSQL UPDATE](https://www.postgresql.org/docs/current/sql-update.html)
- `optimistic-concurrency`: [PostgreSQL transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html), [SQLite isolation](https://www.sqlite.org/isolation.html)
- `state-machine`: [PostgreSQL explicit locking](https://www.postgresql.org/docs/current/explicit-locking.html), [SQLite triggers](https://www.sqlite.org/lang_createtrigger.html)
- `idempotency-key`: [Stripe idempotent requests](https://docs.stripe.com/api/idempotent_requests), [PostgreSQL INSERT ON CONFLICT](https://www.postgresql.org/docs/current/sql-insert.html)
- `inbox`: [Valkey Streams](https://valkey.io/topics/streams-intro/), [PostgreSQL INSERT ON CONFLICT](https://www.postgresql.org/docs/current/sql-insert.html)
- `expiring-reservation`: [Valkey SET](https://valkey.io/commands/set/), [PostgreSQL range types](https://www.postgresql.org/docs/current/rangetypes.html)
- `fifo-queue`: [Valkey lists](https://valkey.io/topics/lists/), [PostgreSQL locking clause](https://www.postgresql.org/docs/current/sql-select.html)
- `lease`: [Valkey EXPIRE](https://valkey.io/commands/expire/), [PostgreSQL explicit locking](https://www.postgresql.org/docs/current/explicit-locking.html)
- `fencing-tokens`: [Valkey INCR](https://valkey.io/commands/incr/), [PostgreSQL sequences](https://www.postgresql.org/docs/current/functions-sequence.html)
- `task-ownership`: [Valkey hashes](https://valkey.io/topics/hashes/), [PostgreSQL locking clause](https://www.postgresql.org/docs/current/sql-select.html)
- `semaphore`: [Valkey sorted sets](https://valkey.io/topics/sorted-sets/), [PostgreSQL advisory locks](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS)
- `delayed-queue`: [Valkey sorted sets](https://valkey.io/topics/sorted-sets/), [PostgreSQL locking clause](https://www.postgresql.org/docs/current/sql-select.html)
- `retry-lifecycle`: [AWS exponential backoff and jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/), [PostgreSQL locking clause](https://www.postgresql.org/docs/current/sql-select.html)
- `priority-queue`: [Valkey sorted sets](https://valkey.io/topics/sorted-sets/), [PostgreSQL ORDER BY](https://www.postgresql.org/docs/current/queries-order.html)
- `rate-limiter`: [Valkey INCR](https://valkey.io/commands/incr/), [PostgreSQL window functions](https://www.postgresql.org/docs/current/tutorial-window.html)
- `heartbeat`: [Valkey EXPIRE](https://valkey.io/commands/expire/), [PostgreSQL date/time](https://www.postgresql.org/docs/current/functions-datetime.html)
- `leader-election`: [Valkey SET](https://valkey.io/commands/set/), [PostgreSQL advisory locks](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS)
- `notification-vs-delivery`: [Valkey Pub/Sub](https://valkey.io/topics/pubsub/), [PostgreSQL NOTIFY](https://www.postgresql.org/docs/current/sql-notify.html)
- `outbox`: [AWS transactional outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html), [PostgreSQL transactions](https://www.postgresql.org/docs/current/tutorial-transactions.html)
- `saga`: [AWS saga patterns](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/saga-patterns.html), [PostgreSQL transactions](https://www.postgresql.org/docs/current/tutorial-transactions.html)
- `durable-event-log`: [Valkey Streams](https://valkey.io/topics/streams-intro/), [PostgreSQL sequences](https://www.postgresql.org/docs/current/functions-sequence.html)
- `consumer-checkpoints`: [Valkey Streams](https://valkey.io/topics/streams-intro/), [PostgreSQL UPDATE](https://www.postgresql.org/docs/current/sql-update.html)
- `consumer-groups`: [Valkey consumer groups](https://valkey.io/topics/streams-intro/), [PostgreSQL locking clause](https://www.postgresql.org/docs/current/sql-select.html)
- `incremental-projection`: [PostgreSQL materialized views](https://www.postgresql.org/docs/current/rules-materializedviews.html), [Valkey Streams](https://valkey.io/topics/streams-intro/)
- `snapshots-compaction`: [Valkey XTRIM](https://valkey.io/commands/xtrim/), [PostgreSQL partitioning](https://www.postgresql.org/docs/current/ddl-partitioning.html)
- `cache-consistency`: [Valkey EXPIRE](https://valkey.io/commands/expire/), [PostgreSQL advisory locks](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS)
- `distributed-id`: [PostgreSQL sequences](https://www.postgresql.org/docs/current/functions-sequence.html), [Valkey INCR](https://valkey.io/commands/incr/)
- `sharded-counter`: [Valkey INCR](https://valkey.io/commands/incr/), [PostgreSQL INSERT ON CONFLICT](https://www.postgresql.org/docs/current/sql-insert.html)
- `partition-rebalancing`: [Valkey sets](https://valkey.io/topics/sets/), [PostgreSQL locking clause](https://www.postgresql.org/docs/current/sql-select.html)
