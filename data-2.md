# Distributed Systems Pattern Curriculum and Data Migration Plan

**Status: complete — implementation, independent review, and tracked database application verified.**


## Living work log and restart instructions

This file is both the implementation plan and the living event log for the
work. Assume the conversation and agent context can disappear at any time.
Keep enough verified state here for a new primary agent to resume without the
old conversation. The primary agent owns updates to this file; delegates send
results to the primary instead of concurrently editing the log.

**Current checkpoint:** A–H complete; the tracked database contains the verified 29-exercise curriculum.
**Next action:** publish the user-authorized commit; implementation is complete. Do not repeat the migration.
**Scope decision:** add only `patterns.curriculum_order`; all other curriculum
content uses existing columns. Replace the catalog with the 29 exercises and
update the tracked database through the verified application sequence.
**Current blockers:** none. All implementation, review, rehearsal, and live-data gates passed.
**Active implementation delegates:** none; all delegate work is finished and accepted.

### Resume from an empty context window

1. Read `AGENTS.md`, `knowledge/index.md`, its relevant linked writeups, and this
   entire file. Before schema work read `docs/schema/AGENTS.md`. Use the
   repository knowledge-update skill when implementation yields reusable lessons.
2. Read the checkpoint, chunk status table, and latest events below. Inspect
   `git status --short` and relevant source/diffs before changing anything. The
   working tree already contained extensive modified and untracked work during
   planning; do not reset, clean, or mistake all changes for this task's output.
3. Verify recorded artifacts, tests, database state, and any running delegates
   against reality. A missing artifact or stale test result is not completion.
   Inspect unfinished edits before rerunning work; never blindly repeat a data
   apply whose outcome was interrupted. Check the actual database first.
4. Select the earliest incomplete chunk whose dependencies are satisfied. Use
   the exact owner/model/reasoning assignments in section 7. If a model is
   unavailable, the primary owns that chunk and records the substitution; do
   not silently switch models. User authorization includes these delegations.
5. Update the checkpoint and log as work proceeds. At a handoff, leave one
   concrete next action, its prerequisites, and exact commands/paths needed.
   Once implementation is started, context loss alone does not require asking
   the user to authorize the same work again.

### Update discipline

Update this file immediately after each meaningful decision, delegation,
completed edit batch, validation result, failure, or database operation, and
before handing off or ending a turn. Before a long-running or destructive
operation, record its intent, target, recovery artifact, and pending status;
afterward record the observed outcome. This allows recovery if context is lost
between the action and its result.

Keep the checkpoint and status table current. Append numbered events; do not
rewrite earlier events to imply a different history. If a decision changes,
append the correction and update the affected plan sections so the executable
plan remains consistent. Mark completion only with evidence, not intentions.

Each new event records:

- Date/time, event number, chunk, owner, and model/reasoning for delegated work.
- Action or decision, why it matters, and affected file/artifact paths.
- Commands and working directory for validation; passed/failed/not-run outcome,
  relevant result, and any remaining limitation.
- Delegate task ID while active, exact file ownership, and completion report
  location or an inline result sufficient to survive loss of agent messages.
- Database state when relevant: target path, backup path, whether only schema
  changed or content committed, observed counts, and verification outcome.
- Outstanding issues and the next concrete step.

Keep durable decisions and concise results here. Large logs may live in a
referenced artifact, but record the essential finding inline and check the
artifact exists before relying on it. Never rely solely on chat, temporary
agent memory, or an unrecorded temporary database path.

### Chunk status

| Chunk | State | Owner / level | Evidence / next step |
|---|---|---|---|
| A | Complete | Primary | E006: source and baseline rechecked; contracts frozen |
| B | Complete | Terra / high | E009: primary diff review and integrated root tests passed |
| C | Complete | Terra / high | E012: reviewed corrections and exact content tests passed |
| D | Complete | Primary | E012/E020: integrated reconciliation checks and independent review passed |
| E | Complete | Luna / high + primary integration | E016: code reviewed and full negative/omission tests passed |
| F | Complete | Luna / high | E015: primary source/guide/schema/link checks passed |
| G | Complete | Sol / high | E020: five findings fixed and independently rechecked; no unresolved findings |
| H | Complete | Primary | E024: backup, rehearsal, live apply, CLI and checkpoint audits passed |

Allowed states: Not started, In progress, Blocked, Complete. Record a concrete
blocker and resolution condition whenever marking a chunk Blocked.

### Event log

**E001 — 2026-09-12 — Planning / primary.** Expanded the curriculum into an
implementation plan. Read-only inspection found 28 patterns, three engines,
84 approaches, zero attempts, zero nonempty pattern notes, and zero nonempty
approach writeups. Verified the identity accounting: 19 survivors (including
seven renames), nine removals, ten additions; target 29 patterns/87 core
approaches. A Python read-only check validated the mapping and knowledge links;
`git diff --check` passed. No implementation tests or database writes occurred.
A preliminary read-only review used `gpt-5.6-terra` / high (`plan_review`, now
finished). Its history-preservation concern is addressed by explicit conflicts
before removing authored or historical data, rather than retaining obsolete
catalog rows. Chunk G still requires review of actual implementation.

**E002 — 2026-09-12 — Scope correction / primary.** User approved only the
`curriculum_order` column. Removed proposed harness metadata columns, CLI flags,
and their validation work from this plan. Harness fit stays prose. Updated
`data-2.md`; `git diff --check` passed. Source and database remain unchanged by
this planning task.

**E003 — 2026-09-12 — Restart preparation / primary.** User requested this file
serve as a living event log through arbitrary context resets. Added restart
instructions, update discipline, chunk states, and explicit high reasoning for
all Terra and Luna assignments. Implementation is intentionally not started;
the user will clear context and start it. Verified explicit Terra/Luna high
assignments and absence of the removed harness columns; `git diff --check`
passed. Next step is chunk A after that start.

**E004 — 2026-09-12 — Delegation and validation review / primary.** User permits
primary ownership and requires validation of all incoming work before commits.
Reassessed A–H: primary keeps A/D/H; Terra high owns B/C; Luna high owns E/F;
G uses Opus only if exposed in the resumed session, otherwise a fresh Terra
high reviewer. Opus is not currently callable. Added a primary acceptance and
commit gate covering diff review, integrated checks, correction verification,
and staged scope. Implementation remains not started. Next action remains A
when the user starts implementation.

**E005 — 2026-09-12 — Model correction / primary.** User clarified that “Opus”
meant Sol. This supersedes E004's optional Opus/fallback review assignment.
Chunk G now belongs to `gpt-5.6-sol` with high reasoning, which is available.
Updated both assignment tables and section 7; all other owners and the primary
validation/commit gate remain unchanged. Implementation has not started.

**E006 — 2026-09-12 — A / primary.** Implementation started from the user's active goal. Read repository knowledge, full plan, schema guidance, and knowledge skill. Read-only Python SQLite inventory reconfirmed exactly the 28 mapped legacy slugs, three engines, 84 approaches, zero attempts/notes/writeups. Current source confirms no order support yet. Mapping accounts for every old identity. Frozen contracts: `Pattern.CurriculumOrder *int` (number/null JSON); `seedPattern.curriculumOrder int` (canonical 1–29). Section 4 boundaries are mandatory content requirements. Retained authored approaches/history are preserved; unknown patterns/engines and non-core approaches on renamed/retired rows are conflicts. Extra retained authored approaches are reported honestly rather than asserting 87 total. Existing uncommitted work is preserved; work continues in this workspace as the restart plan directs. No database writes. Next: B/C delegation; primary prepares reconciliation.

**E007 — 2026-09-12 — B/C delegation / primary.** Started `/root/schema` (`gpt-5.6-terra`, high) owning `db.go`, `store.go`, migration/store tests; `/root/catalog` (same model/level) owning new `seed_catalog.go` and optional catalog tests. Primary will remove old declarations from `seed.go` during D integration, allowing disjoint work. Both must use temporary databases and cannot commit or modify tracked DB/log. Acceptance pending. Primary inspected driver source: modernc SQLite exposes `NewBackup` through raw connection for read-only-source backup. Next: implement transactional reconciliation/report and dry-run routing.

**E008 — 2026-09-12 — D preparation / primary.** Preserved starting source copies at `/var/folders/bh/gj52slt51639d18cgb918pcr0000gn/T/trinkets-data2-baseline-yuydifjr` and exported read-only baseline SQL to `testdata/legacy-catalog.sql` for reproducible migration tests. Removed old data declarations from `seed.go` (C supplies replacement), drafted caller-owned transaction seed helpers with conditional updates, `reconcile.go` conflict/report/apply/backup logic, and pre-open seed option routing in `main.go`. This batch is unvalidated until B/C land. No tracked database write. Next: exercise rollback, identity preservation, idempotence, and source-unchanged dry run with fixtures.

**E009 — 2026-09-12 — B acceptance / D tests / E delegation — primary.** Reviewed full B source delta against saved baseline and `db_store_test.go`: nullable order, transactional column/index migration, preserved scans/FKs, and list/matrix ordering meet scope. Primary `go test ./...` passed on integrated B+C draft+D code (0.362s), including reconciliation rollback/conflict/identity/idempotence/source-byte-preservation tests. B accepted. Started `/root/cli` (`gpt-5.6-luna`, high), owning `cmd_pattern.go`, `cmd_matrix.go`, optional `output.go` helper and CLI tests. Added `reconcile_test.go`; tests verify all 19 pattern and 57 approach identities, 87 final approaches, retained authored/history content, precise conflicts and no writes on repeated reconciliation using rejecting UPDATE triggers. C still pending content acceptance. No tracked DB writes. Next: review C, accept E, synchronize F, then independent G.

**E010 — 2026-09-12 — C review / D verification — primary.** C delivered `seed_catalog.go` and reports all 31 primary reading URLs verified. Primary inspected all 29 definitions and rejected acceptance pending concrete corrections: PostgreSQL allocation versus commit-order hole; FIFO/SKIP LOCKED mismatch; unspecified priority quotas/cache stale policy/ID recovery; fencing activation and repeated owner writes; saga store combinations and ambiguous step completion; explicit Valkey persistence assumptions. Sent bounded corrections to `/root/catalog` (Terra high, same ownership). Added D tests for committed-WAL backup, ordinary seed retaining legacy/custom rows, order swaps, and occupied custom-order rollback. Primary `go test ./...` passed (0.347s); prior `go vet ./...` passed. No tracked DB writes. Next: accept corrected C and E, then F/G.

**E011 — 2026-09-12 — D / knowledge / B hardening — primary.** Added `seed_catalog_test.go` with the independent exact 29-slug/order contract and canonical persisted-content verification. Apply postconditions now check full seed-owned pattern content and every engine primitive sketch, not only counts. Primary `go test ./...` passed (0.458s). Updated existing persistence/architecture knowledge and index for verified migration, preview, identity, and ordering behavior. Sent B hardening to `/root/schema` (Terra high): validate the full index definition rather than accepting a wrong predicate or extra key columns. `git diff --check` passed. Active work: C correction, E CLI, B index hardening. Tracked DB still unchanged. Next: validate those deliveries then F.

**E012 — 2026-09-12 — B hardening / C and D acceptance — primary.** Reviewed B's full canonical index-DDL guard and negative predicate/compound-index tests; accepted. Reviewed corrected C in full, then applied final wording fixes: FIFO minimum eligible sequence at each claim (including requeues); explicit cache waiter coalescing and unexpired token/fence publication checks; ID issue-order and overflow failure; outbox eventual delivery with fair retries; SQLite RETURNING waits for commit before HTTP success. All 29 Valkey approaches carry the shared AOF-always/no-eviction/single-primary restart assumption. C now meets section 4 and is accepted. Primary root tests passed after final edits; D accepted subject to independent G. E completing negative CLI coverage. No tracked DB writes. Next: accept E and launch F documentation.

**E013 — 2026-09-12 — E review / F delegation — primary.** Primary reviewed E source and current CLI test coverage; final flags/output implementation is present and integrated root tests/vet pass. E delegate is finishing additional negative coverage. Started `/root/docs` (`gpt-5.6-luna`, high) against that implemented surface, owning only the active catalog guide, CLI guide, relevant README changes, and both schema HTML pages. Added stale-preview regression proving intervening authored notes block apply and survive unchanged; root tests passed (0.452s), vet passed. No live DB writes. Next: accept E/F reports then independent G.

**E014 — 2026-09-12 — H audit preparation / primary.** Prepared read-only `/private/tmp/trinkets-data2-verify.py` (syntax compilation passed) to compare a baseline backup, migrated target, and fresh canonical database. It checks integrity/FKs, exact 29/3/87/0 counts, exact ordered slug set, all canonical content, 19 preserved pattern identities, 57 preserved approach identities, and 27 removed approach identities. It has not yet been run on an application outcome. Intended usage after G: `python3 /private/tmp/trinkets-data2-verify.py BASELINE TARGET CANONICAL`. E/F delegates remain active. No live DB writes or backup/application yet.

**E015 — 2026-09-12 — F acceptance / primary.** Reviewed all F files against current implementation and saved pre-task README; unrelated README content preserved. Primary corrected table NULL display (`-` vs JSON null), made seed prose preservation explicitly pattern notes, replaced the stale SKIP LOCKED FIFO example with a queue-head gate, and expanded the ERD pattern box to show the full `curriculum_order` field with aligned relationship path. Python checks passed: exact 29 ordered contracts and 29 three-engine core rows, all relative links resolve, SVG parses as XML and includes the exact field name. Integrated `go test ./...` passed (0.416s), `go vet ./...` and `git diff --check` passed. F accepted. E final delegate report still pending; production CLI code has already passed primary review/checks. No live DB writes. Next: finish E acceptance and G.

**E016 — 2026-09-12 — E acceptance / G handoff — primary.** E production code and delivered test file were inspected directly and integrated tests passed. Asked E delegate to finalize without further production/knowledge edits; primary completed remaining requested edge coverage in disjoint `cli_validation_test.go` (fractional/non-numeric/overflow/nonpositive/duplicate values on add and edit leave exact JSON rows unchanged; clearing preserves notes/invariants). Root `go test ./...` passed (0.554s), whitespace check passed. E accepted based on authoritative source and primary tests rather than waiting for its narrative report; no model substitution for implementation. F already accepted. Next: fresh Sol high independent review of integrated B–F; tracked DB remains unchanged.

**E017 — 2026-09-12 — G delegation / application-audit validation — primary.** Started fresh `/root/review` (`gpt-5.6-sol`, high), read-only B–F review; it must report findings before live application. Built the CLI at `/private/tmp/trinkets-data2-njttrz05/trinkets` and validated the H audit script on historical SQL fixture copies, not the tracked database. `fixture-baseline.db` → `fixture-applied.db` matched freshly seeded `canonical.db`: 29 patterns, 3 engines, 87 approaches, zero attempts; integrity ok, no FK errors, exact ordered content, 19/57 retained identities and 27 retired approaches. Audit also confirms schema adds only curriculum_order. Actual live backup/rehearsal remain pending G. Artifacts and fixture apply JSON are in that temporary directory. No live DB writes.

**E018 — 2026-09-12 — E final report / primary.** After E source and tests were accepted, stopped its remaining investigation and requested a text-only completion report. `/root/cli` reports changes to `cmd_pattern.go`, `cmd_matrix.go`, `output.go`, `cli_curriculum_order_test.go`, and CLI notes in the existing persistence writeup; focused CLI/root tests, vet, and formatting passed. Primary had already reviewed those surfaces and added the remaining explicit negative/omission coverage. No outstanding E work. G remains active. Tracked DB still unchanged.

**E019 — 2026-09-12 — G findings and corrections / primary.** Sol confirmed five findings: existing malformed order columns bypass the guarantee; SQLite ID arithmetic needs an overflow guard; PostgreSQL shard increments must depend on a newly inserted operation identity; SQL FIFO sketches omitted recovery/acknowledgement; PostgreSQL reservation completion needs an unexpired deadline. Assigned the column guard to fresh `/root/schema_guard` (`gpt-5.6-terra`, high), owning only `db.go` and migration tests. Primary updated `seed_catalog.go` and guide for the four content findings, including deadline evaluation after acquiring the reservation row lock. G continues remaining review and will verify fixes. Previously built binary/canonical DB are now stale due to content changes and MUST be rebuilt/reseeded before H. No live DB writes.

**E020 — 2026-09-12T22:10:02+00:00 — G acceptance / H preflight — primary.** Sol independently rechecked all five fixes and reports no unresolved findings. It verified writer-lock acquisition before planning and byte-identical legacy source after dry run, and root tests/vet passed. Primary reviewed B guard/source/tests and updated both schema pages plus knowledge for incompatible-definition rejection; primary integrated tests (0.397s), vet, and whitespace check passed. Read-only live preflight compared every row of all four tables to the original fixture: identical, still 28/3/84/0. Main-file SHA-256 `b20b52b1f6b55fd172a95d921690df84e7673584267fc61ccf00b58498b46935`.

**Pending H operation:** make SQLite-backup-API copy of `/Users/nick/Software/systems-trinkets/trinkets.db` at `/private/tmp/trinkets-data2-njttrz05/live-baseline.db`, then a separate rehearsal copy. Rebuild the reviewed CLI and create `canonical-reviewed.db` before comparing. Backup is the recovery artifact; live content remains unchanged until a later explicitly logged apply.

**E021 — 2026-09-12T22:10:55+00:00 — H backup/rehearsal — primary.** SQLite backup API successfully created `/private/tmp/trinkets-data2-njttrz05/live-baseline.db` from the read-only tracked source, including committed WAL state; backup integrity/FK checks passed and counts remain 28/3/84/0. Rebuilt reviewed CLI, seeded `canonical-reviewed.db`, and reconciled separate `live-rehearsal.db`. Read-only audit passed: exact 29/3/87/0, exact canonical content/order, only new order column, integrity ok/no FK violations, 19 pattern and 57 approach identities preserved, 27 retired approach IDs gone. Rehearsal report: `/private/tmp/trinkets-data2-njttrz05/rehearsal-apply.json`. Live data is still unchanged. Next: review live dry-run against this report, recheck source matches backup, then log/apply the exact reviewed command.

**E022 — 2026-09-12T22:11:14+00:00 — H apply intent (pending) — primary.** Live dry-run exactly matches rehearsal: 12 refreshes, 7 rename/refreshes, 10 inserts, 9 explicit retirements removing 27 approach IDs, zero conflicts or extra authored approaches. Complete live rows still match backup and main-file SHA-256 is unchanged. All A–G gates passed. About to execute `/private/tmp/trinkets-data2-njttrz05/trinkets --db ./trinkets.db seed --update --prune`, saving report as `live-apply.json` in the same temporary artifact directory. Target is the tracked repository database; recovery artifact is `/private/tmp/trinkets-data2-njttrz05/live-baseline.db`. Content transaction rollback is the first recovery path on failure; do not blindly restore over later user changes. Application outcome pending; inspect actual DB if interrupted here.

**E023 — 2026-09-12T22:12:00+00:00 — H apply result — primary.** The exact reviewed live apply command succeeded. The tracked database now audits as 29 patterns, 3 engines, 87 core approaches, zero attempts; all ordered identities and canonical content match the reviewed fresh seed, schema adds only curriculum_order, integrity is ok and foreign-key check has no rows. All 19 survivor pattern IDs/creation timestamps and 57 approach IDs/creation timestamps survived; 27 retired approach IDs are absent. Recovery backup remains `/private/tmp/trinkets-data2-njttrz05/live-baseline.db`; apply report is `live-apply.json` alongside it. Next: final live CLI checks and WAL checkpoint, then completion audit/log. Do not reapply unnecessarily if resuming here.

**E024 — 2026-09-12T22:14:11+00:00 — H completion audit / primary.** Final live list/matrix/show JSON agree on positions 1–29; FIFO shows order 7 and three approaches. Live apply actions exactly match reviewed preview. WAL checkpoint returned `(0, 0, 0)`; integrity remains ok, no FK violations, and no WAL/SHM files remain. Final tracked main-file SHA-256: `b045eb5a7d4e4569b115100e6255f06851a2aa482190c636c2b6c917507fa278`. Final formatting/whitespace, exact guide order/87 cells, relative links, SVG XML, and knowledge inventory checks passed. The verified current source passed root tests and vet before application; no harness code changed in this task.

Readable database change summary: **28 → 29 patterns; 3 → 3 engines; 84 → 87 core approaches; 0 → 0 attempts.** Twelve retained patterns refreshed, seven renamed in place, ten inserted, nine retired. All 19 surviving pattern and 57 surviving approach IDs/creation timestamps preserved; 27 retired approaches removed and 30 new approaches inserted. Canonical content matches the fresh reviewed seed. Backup: `/private/tmp/trinkets-data2-njttrz05/live-baseline.db`; rehearsal, preview/apply reports, and live JSON outputs are alongside it. All five independent-review findings were resolved and rechecked. Both schema pages and existing knowledge entries reflect implemented behavior. No commits were made; pre-existing unrelated work remains intact. All definition-of-done items below are now supported by these events and current artifacts.

**E025 — 2026-09-12T22:46:37+00:00 — commit/push authorization — primary.** User explicitly requested commit and push. Commit scope is the curriculum implementation, migrated tracked database, root CLI prerequisites, tests, schema references, and relevant documentation/knowledge. Separate pre-existing harness work and its ignore rules remain outside this commit. Fresh root `go test ./...` passed (0.538s), `go vet ./...` passed, and whitespace checks passed. Origin was fetched before publishing. The commit containing this event is the implementation handoff commit; its hash and push outcome are reported in the final handoff and discoverable with `git log -- data-2.md`.

## Curriculum scope

This plan replaces the current catalog with exactly the 29 exercises below,
adds a curriculum order column to the database, and
removes obsolete standalone definitions and their generated approaches.
Implementation includes updating the tracked `trinkets.db` after validation on
a temporary copy. Expanding this document does not itself execute that migration.

This is the recommended ordered curriculum for small Go HTTP servers backed by
Valkey, SQLite, PostgreSQL, or a combination of those stores. Other tools can be
added later.

Each implementation exposes its behavior through HTTP. The harness verifies
observable invariants without reading the backing database directly. Patterns
later in the list may require the planned process crash/restart support,
operation-history checking, or test-controlled failure injection.

The current catalog contains 28 seeded patterns and 84 approaches, but no
recorded attempts. This curriculum therefore evaluates the catalog definitions
and primitive map rather than results from completed implementations.

## Final ordered list

| # | Pattern / exercise | What the harness should prove |
|---:|---|---|
| 1 | **Atomic counter** | Concurrent increments are never lost and returned values are atomic. Keep this as the small introductory exercise already implemented. |
| 2 | **Optimistic concurrency / versioned writes** | An update succeeds only against the version it read; two conflicting updates cannot silently overwrite each other. |
| 3 | **Guarded state machine** | Only legal transitions occur, concurrent transitions serialize, and repeated transitions are handled consistently. |
| 4 | **Idempotency key** | Concurrent and sequential retries produce one effect and the same stored response, including after restart. |
| 5 | **Inbox / idempotent consumer** | Duplicate deliveries are recorded and processed once, while distinct messages are not accidentally collapsed. This absorbs plain dedup. |
| 6 | **Expiring reservation** | Only one client holds a resource, expiration releases abandoned reservations, and completion cannot be stolen. |
| 7 | **Reliable FIFO work queue** | No job is lost, only one worker holds a claim at a time, ordering is reasonable, and acknowledgement is explicit. |
| 8 | **Lease** | One live owner exists, renewal extends ownership, and abandoned ownership eventually expires. |
| 9 | **Fencing tokens** | A stale owner cannot mutate the protected resource after a newer owner has taken over. |
| 10 | **Task ownership and recovery** | Claimed work identifies its owner, completed work stays complete, and orphaned work becomes claimable again. |
| 11 | **Semaphore / bounded concurrency** | At most N permits are live, duplicate release is harmless, and permits held by crashed clients return. |
| 12 | **Delayed queue / scheduler** | Nothing is claimed early, every due job becomes claimable, and concurrent schedulers do not run it twice. |
| 13 | **Retry and dead-letter lifecycle** | Backoff is respected, attempt counts are atomic, poison jobs stop retrying, and dead-lettered jobs can be redriven safely. |
| 14 | **Priority queue with fairness** | Higher priority normally wins, equal-priority ordering is stable, and aging or quotas prevent indefinite starvation. |
| 15 | **Comparative rate limiter** | Implement fixed window, sliding window, and token bucket behind one contract; test limits, refill, burst behavior, and concurrent admission. |
| 16 | **Heartbeat and failure detector** | Active workers remain live, silent workers become suspected after a timeout, and recovery does not create duplicate identities. |
| 17 | **Leader election** | One leader is visible at a time, leadership is renewable, and another candidate takes over after expiry. Build it on leases and fencing. |
| 18 | **Ephemeral notification vs durable delivery** | Connected subscribers receive notifications, disconnected subscribers may miss ephemeral signals, and durable state remains recoverable. This replaces plain Pub/Sub. |
| 19 | **Transactional outbox** | Business state and the outgoing record commit together; committed messages are eventually delivered and safely redelivered. |
| 20 | **Saga / compensating transaction** | A multi-store workflow either completes or records and performs the required compensations after partial failure. |
| 21 | **Durable event log** | Appends survive restart, positions are ordered and unique, and clients can replay from a supplied position. |
| 22 | **Consumer checkpoints / offsets** | Progress survives restart; checkpoint advancement cannot skip unprocessed records; replay may duplicate but cannot lose records. |
| 23 | **Consumer group** | Partitions or messages are divided among workers, abandoned work is reassigned, and acknowledgements advance group progress safely. |
| 24 | **Incremental projection / read model** | Derived state corresponds to a known log position, duplicate application is harmless, and rebuilding produces the same result. This replaces the simple materialized-view entry. |
| 25 | **Snapshots and log compaction** | State can be restored from a snapshot plus remaining events, and compaction never removes data still required by consumers. |
| 26 | **Cache consistency and stampede protection** | Expired values are not treated as fresh, concurrent misses coalesce into one refresh, and refresh failure has defined stale-value behavior. This replaces plain TTL cache. |
| 27 | **Distributed ID generation** | IDs remain unique under concurrency and restart, preserve their promised ordering properties, and survive leased-range or clock edge cases. |
| 28 | **Sharded/hot-key counter** | Writes distribute across shards, concurrent totals remain correct under the chosen consistency model, and shard retries are idempotent. |
| 29 | **Partition assignment and rebalancing** | Registered workers receive non-overlapping assignments, membership changes redistribute work, and stale assignments are fenced. |

## Harness fit

- **Current concurrency harness:** 1-7, 12-15, and portions of 26-28.
- **Phase 2 process restart/crash control:** 8-13 and 16-29.
- **Phase 3 operation histories:** especially valuable for 1-4, 7-10, 17,
  and 21-23.
- **Multiple stores or test-controlled failures:** most important for outbox
  and saga. They can still use one public SUT URL; test-only endpoints can
  pause delivery, fail a step, or expose workflow state.

Every suite should have final-state or history-based assertions. Avoid tests
whose correctness depends on narrowly winning a timing race. Time-based
patterns can use eventual assertions with generous deadlines, while ownership
and concurrency claims should be checked from returned tokens, state, or
recorded operation intervals.

An engine making the happy path convenient does not automatically make a
pattern worthless. PostgreSQL `SKIP LOCKED`, Valkey Streams, or Valkey
expiration may provide the central primitive, but a worthwhile exercise should
still cover crashes, stale ownership, duplicates, retries, overload, or
recovery. If an exercise ends after demonstrating one database command, it is
too shallow.

## Remove as standalone patterns

- Session store
- Distributed/local coordination
- Plain lock
- Plain TTL cache
- Plain set membership/dedup
- Secondary index
- Time-ordered data
- Leaderboard
- Plain materialized view
- Separate fixed-window, sliding-window, and token-bucket rate limiters
- Separate dead-letter queue
- Plain Pub/Sub

Some of these concepts remain inside stronger exercises: dedup becomes part of
the inbox, TTL caching becomes cache consistency and stampede protection,
dead-lettering becomes part of the retry lifecycle, and materialized views
become incremental projections.

## Defer until the harness supports multiple independently failing nodes

- Consistent hashing
- Quorum registers
- Read repair
- Anti-entropy
- Vector clocks
- CRDTs
- Raft or another replicated-state-machine protocol

These are valuable distributed-systems topics, but a single SUT process behind
one base URL would either hide the important behavior or merely simulate it.
They become worthwhile when the harness can start, stop, partition, and address
multiple server processes independently.

Also defer circuit breakers, generic load shedding, and generic backpressure.
They are testable over HTTP, but primarily depend on controllable downstream
services rather than the current database-backed exercise model. Bounded
concurrency, queue capacity, retry policy, and rate limiting already cover the
closest database-oriented lessons.

## Suggested stopping points

- After **#7**: concurrency and atomicity fundamentals.
- After **#17**: ownership, expiry, recovery, and coordination.
- After **#25**: durable messaging, logs, and projections.
- After **#29**: scaling-oriented storage and worker-management building
  blocks.


## 1. Verified starting point and scope

Inspected on 2026-09-12 using a read-only SQLite connection and current source:

- `trinkets.db`: 28 patterns, three engines (`valkey`, `sqlite`, `postgres`),
  84 approaches, zero attempts, zero nonempty pattern notes, and zero nonempty
  approach writeups. Recheck these facts immediately before applying changes.
- `db.go`: four tables, idempotent column migrations, JSON-array fields for
  invariants/readings, cascading pattern deletion, and a composite foreign key
  from attempts to approaches. Opening the CLI database runs migrations.
- `seed.go`: explicit Go definitions, one `core map sketch` per pattern/engine;
  `seed --update` upserts but never removes retired rows. Seed preserves pattern
  notes and approach writeups. Merely replacing the seed list is insufficient.
- `store.go`: pattern lists order by family/slug, and matrix assembly has its
  own final sort. Both need curriculum ordering.
- `harness/test-plan.md`: phase 1 implemented; process lifecycle and operation
  histories are planned. Only the counter suite exists. A pattern that fits the
  current toolkit does not therefore have an implemented suite.

Keep the three engine records. Do not add fake combination engines, exercise
runtime tables, harness result columns, or per-pattern business schemas to this
catalog database. Multi-store exercises describe their composition in approach
primitives. Adding runtime support or the other 28 suites is separate work.
The deferred topics above stay in this document, outside the seeded catalog.

## 2. Exact target identities and reconciliation map

The ordered list above owns the display names and intended guarantees. This
map owns stable slugs and what happens to existing identities. Preserve IDs and
`created_at` for every retained or renamed row. Refresh its curriculum-owned
content even where the old slug remains unchanged.

| Order | Final slug | Existing row / action | Family |
|---:|---|---|---|
| 1 | `counter` | Retain; display name becomes Atomic counter | counting |
| 2 | `optimistic-concurrency` | Create | state |
| 3 | `state-machine` | Retain; strengthen guards/retries | state |
| 4 | `idempotency-key` | Retain; include stored response and restart | idempotency |
| 5 | `inbox` | Rename `dedup`; rewrite for transactional processing | idempotency |
| 6 | `expiring-reservation` | Rename `expiring-uniqueness` | idempotency |
| 7 | `fifo-queue` | Retain; include acknowledgement/recovery | queueing |
| 8 | `lease` | Retain | coordination |
| 9 | `fencing-tokens` | Create | coordination |
| 10 | `task-ownership` | Retain; add orphan recovery | coordination |
| 11 | `semaphore` | Retain; add permit recovery | coordination |
| 12 | `delayed-queue` | Retain | queueing |
| 13 | `retry-lifecycle` | Rename `retry-queue`; absorb `dead-letter-queue` content | queueing |
| 14 | `priority-queue` | Retain; add fairness | queueing |
| 15 | `rate-limiter` | Rename `fixed-window-rate-limiter`; absorb `sliding-window-rate-limiter` and `token-bucket` content | rate-limiting |
| 16 | `heartbeat` | Create | coordination |
| 17 | `leader-election` | Create | coordination |
| 18 | `notification-vs-delivery` | Rename `pubsub`; distinguish ephemeral and durable behavior | messaging |
| 19 | `outbox` | Retain; strengthen atomicity/redelivery | messaging |
| 20 | `saga` | Create | messaging |
| 21 | `durable-event-log` | Retain; specify positions/replay | log |
| 22 | `consumer-checkpoints` | Create | log |
| 23 | `consumer-groups` | Retain; strengthen reassignment/progress | log |
| 24 | `incremental-projection` | Rename `materialized-view`; rewrite for replay/rebuild | state |
| 25 | `snapshots-compaction` | Create | log |
| 26 | `cache-consistency` | Rename `ttl-cache`; add coalescing and failure policy | caching |
| 27 | `distributed-id` | Create | counting |
| 28 | `sharded-counter` | Create | counting |
| 29 | `partition-rebalancing` | Create | coordination |

Delete these nine legacy rows after target content is populated:
`session-store`, `distributed-coordination`, `lock`, `secondary-index`,
`time-ordered-data`, `leaderboard`, `dead-letter-queue`,
`sliding-window-rate-limiter`, and `token-bucket`.
Absorbing content means rewriting the surviving exercise, not automatically
reparenting old attempts or merging approach IDs.

Accounting: 28 old rows minus nine deleted rows plus ten new rows = 29.
Seven renames do not change the count. Preserve `counter` and `fifo-queue`
slugs because existing harness paths and plans use them.

For the verified baseline, retain/refresh the 57 generated approaches attached
to the 19 surviving patterns, cascade-delete 27 approaches attached to the nine
retired patterns, and insert 30 approaches for ten new patterns. Final result:
29 patterns, three engines, 87 core-map approaches, zero attempts.

## 3. Database representation

Reuse `name`, `family`, `explanation`, `use_cases`, `invariants`, and `readings`.
Store each independently testable guarantee as a separate invariant string.
Preserve `notes` for user prose. Add only `curriculum_order` to `patterns` in
both fresh DDL and the existing-database migration:

| Column | SQL declaration/default | Meaning and validation |
|---|---|---|
| `curriculum_order` | `INTEGER CHECK (curriculum_order IS NULL OR (typeof(curriculum_order) = 'integer' AND curriculum_order > 0))` | NULL for user-created noncurriculum patterns; canonical rows are exactly 1–29. Unique partial index for non-NULL values. |

Harness requirements and limitations remain prose in this document and the
catalog guide. Existing explanation and invariant fields describe each
exercise's guarantees. No additional metadata columns, tables, or changes to
approach/attempt foreign keys are needed. Exercise-specific runtime columns
(such as lease owners, fencing tokens, and checkpoints) belong to future
implementations, not this catalog database.

Stopping points are derived from order (7, 17, 25, 29). Prerequisite links
remain prose.

Migration rules:

1. Add the nullable order column without changing existing catalog content.
2. Create the order index only after the column exists on legacy databases;
   an index referencing a missing column in the initial `schemaSQL` would fail
   before `openDB` reaches migrations. Put index creation in the migration path
   that also runs for fresh databases.
3. Wrap the new multi-statement migration in a transaction. Keep it idempotent
   and leave existing legacy conversions intact.
4. Keep destructive catalog reconciliation out of `openDB`. Ordinary list/show
   operations must never retire catalog rows simply by opening the database.
5. During a curriculum refresh, clear managed order values before assigning
   all final positions inside the same transaction, avoiding transient unique
   collisions. Reject positions occupied by unrelated user rows.

## 4. Canonical content and approaches

Keep the Go seed catalog as the executable source of truth (`seed_catalog.go`
for typed data, `seed.go` for transaction mechanics); expand its typed definitions
for curriculum order and maintain the corresponding readable guide in
`docs/systems-patterns.md`. Do not introduce a Markdown parser. This document
owns the migration decisions; the guide must stop claiming the old map is the
active catalog.

For every target exercise supply a substantive explanation, real use cases,
separate invariants covering its full ordered-list contract, relevant readings,
curriculum order, and one reviewed core-map sketch per engine. Preserve
useful existing readings only when still relevant; verify any new technical
links against primary documentation during implementation. Do not copy weaker
legacy guarantees into renamed exercises.

Content review must resolve these boundaries explicitly:

- FIFO means a defined claim order among eligible jobs; completion order under
  concurrency is different. Specify fairness bounds or quotas, not “reasonable.”
- Claims, leases, and notifications do not promise exactly-once external side
  effects. State the atomicity boundary and duplicate handling.
- Idempotency/inbox entries bind the key/message to the business effect and
  stored outcome; a bare `SET NX` or unique insert is insufficient.
- Fencing must be checked by the protected resource, not just issued by a lease.
- Rate limiting contains all three algorithms in one exercise and all three
  algorithm sketches within each engine's primitives text. Keep 87 core-map
  rows, rather than recreating three standalone patterns.
- Outbox primitives identify where business state and outbox commit together;
  saga primitives identify the multiple stores, durable progress, and compensation.
- Logs distinguish allocated IDs from committed replay order; checkpoints must
  not skip holes. Snapshots/compaction respect the oldest required consumer state.
- Cache semantics specify allowed stale responses on refresh failure and the
  coalescing guarantee. IDs state their clock/restart assumptions. Sharded
  counters state their consistency and retry identity.
- SQL approaches implement expiry/ownership with guarded transactions; engine
  primitives alone do not establish the whole contract. State required Valkey
  persistence assumptions for restart guarantees.

## 5. Seed and reconciliation interface

Retain `seed` as non-destructive insert-missing behavior and `seed --update` as
refresh-existing behavior. Neither removes or renames legacy patterns.
Add explicit `seed --update --prune` for the reconciliation specified here, and
`seed --update --prune --dry-run` for its deterministic report. Reject `--prune`
without `--update`; support `--dry-run` only with this reconciliation combination
initially. Update help and `docs/trinkets-cli.md` accordingly.

Refactor seeding so a single transaction can rename legacy rows, refresh final
patterns, populate approaches, remove retired rows, and check postconditions.
Do not call a helper that starts another transaction or queries through `db`
while holding the only connection in `tx`.

The reconciliation report lists old/new slugs and IDs, inserts, refreshes,
removed approach IDs, and conflicts. Compute it from actual rows, using the
explicit mapping above. Never use `DELETE ... WHERE slug NOT IN (...)` across
an arbitrary database. Unknown patterns/engines/approaches are conflicts for
an exact replacement, not silently deleted. A same-slug target already present
alongside its rename source is a conflict, not an invitation to merge IDs.

For this migration, block before content mutation if any of the following has
appeared since the baseline: attempts on rows being renamed or removed,
nonempty notes/writeups on those rows, or non-core approaches attached to them.
Report the precise rows so a deliberate preservation decision can be made.
Retained patterns preserve notes, authored approaches, attempts, IDs, and
creation timestamps; only seed-owned fields are refreshed. Unexpected extra
rows prevent claiming the exact 29/87 baseline outcome and require a revised
report. The presently verified baseline needs no history-reparenting feature.

A real dry run must not persist schema or data writes. Because normal CLI startup
calls `openDB`, route this mode before opening the live database for migration:
use a read-only SQLite source and an SQLite backup into a temporary database,
then run migration/planning there. Build apply's report again under its write
transaction; do not trust an earlier dry-run snapshot after intervening edits.

Apply in one data transaction: validate conflicts, rename survivors, clear
managed ordering, upsert all targets/approaches, delete the nine explicit
retirements, verify exact target membership and relationships, then commit.
Preserve timestamps for unchanged rows so a second successful reconciliation
has an empty semantic diff. Failures roll back every content change. Schema
upgrade is a separate completed step and may remain after a failed content apply.

After reconciliation, plain `seed` and `seed --update` must never resurrect
legacy slugs because those definitions no longer exist in the executable seed.

## 6. CLI and presentation changes

Extend `Pattern`, `patternCols`, `scanPattern`, seed bindings, JSON output, and
all add/edit/show/list paths together. Use a nullable integer representation for
order so JSON emits a number or null, not a nullable-wrapper object.

Expose `--curriculum-order N` and `--clear-curriculum-order`, rejecting both
at once. Preserve partial-edit behavior: omission leaves existing fields
unchanged, and an edit with no fields still fails. Add validates before
inserting, edit before updating, and SQL enforces order uniqueness independently.
Existing invariant and reading list behavior stays the same.

Default list and matrix order: curriculum rows ascending, then NULL-order
custom rows by family/slug. Apply the same order after matrix assembly instead
of allowing its final sort to undo it. Include order in list/show/matrix table
and JSON output. Keep existing family filtering.

## 7. Work chunks and delegation

The primary agent owns integration and any write to the tracked database.
Delegates work on bounded files and temporary databases. They report changed
files, checks, and unresolved issues before the primary integrates their work.
Luna means `gpt-5.6-luna` with **high** reasoning; Terra means
`gpt-5.6-terra` with **high** reasoning. These exact levels apply to every
delegated chunk below. Include this file, its current checkpoint, the assigned
chunk, file ownership, dependencies, and required checks in each delegate task.
Sol means `gpt-5.6-sol` with **high** reasoning. Assign G to a fresh Sol high
agent for independent read-only review of migration atomicity, preservation,
and full-contract coverage. A (contract decisions), D (destructive
reconciliation), and H (live application) remain with the primary. B/C fit
Terra high; E/F fit Luna high. The primary validates every result and resolves
all findings before committing. No Opus assignment is intended; the user's
model choice was corrected to Sol (see E005).

| Chunk | Owner and reason | Dependencies / owned scope | Completion evidence |
|---|---|---|---|
| A. Freeze identities and contracts | **Primary**: curriculum meaning, ownership, and deletion decisions need one accountable owner | This plan; approve exact map, order, 29 contracts, and preservation policy | Every old slug accounted for; exact target identity set; no unresolved contract ambiguity |
| B. Schema and persistence | **Terra high**: bounded but interconnected SQL migration and Go scan work | A; `db.go`, pattern portions of `store.go`, migration/store tests | Fresh/legacy/repeated migration tests, constraints, JSON round trips, preserved IDs/FKs |
| C. Rewrite catalog definitions | **Terra high**: three-engine guarantee review needs reasoning beyond mechanical editing | A and agreed seed field types; seed data declarations only; no edits to reconciliation functions | 29 ordered records, 87 complete primitive sketches; old definitions absent |
| D. Reconciliation and seed integration | **Primary**: destructive behavior, transactions, ownership conflicts, and startup routing | B+C; seed command/helpers, `main.go` routing, reconciliation tests | Read-only dry run, exact baseline transition, rollback/conflict/idempotence tests |
| E. CLI curriculum ordering | **Luna high**: mechanical surface work once types and semantics are fixed | B; `cmd_pattern.go`, `cmd_matrix.go`, flag helpers and focused CLI tests; primary coordinates matrix store edits with B | Add/edit/clear validation, omission preservation, consistent list/matrix/JSON order |
| F. Documentation | **Luna high**: bounded synchronization against implemented behavior | B–E; `docs/systems-patterns.md`, `docs/trinkets-cli.md`, root README as needed, both schema HTML pages | Exactly the new catalog; correct column/index references and ERD; no planned harness capability presented as built |
| G. Independent migration review | **Sol high**: independent review of failure paths after integration | B–F; read-only review of code/diffs and temporary fixtures; primary fixes findings | No unexplained deletions, history loss, stale references, or migration ordering faults |
| H. Apply and handoff | **Primary**: shared database ownership and final verification | All earlier chunks pass | Consistent backup, successful rehearsal, tracked DB reconciled and checked, knowledge updated |

B and C can run concurrently after A fixes the data types; use disjoint edits
or have C deliver data for primary integration rather than sharing `seed.go`
with D. E follows B. F follows implemented behavior. G reviews the integrated
result before H. Do not launch agents merely to fill available slots.

### Primary validation and commit gate

The primary must validate every incoming delegate result before accepting the
chunk as Complete and before any commit. A delegate's claim that tests passed
is evidence to inspect, not a substitute for primary verification.

1. Inspect the complete diff and new files against the assigned scope, this
   plan, and current source. Check for unrelated edits, extra schema columns,
   weaker invariants, data loss, and overlapping agent edits.
2. Run the relevant focused checks on the integrated working tree. Review test
   quality and negative/failure cases as well as pass status. For documentation,
   compare claims to implemented behavior and verify links/schema references.
   For catalog content, check all 29 contracts and all 87 primitive sketches.
3. Fix issues locally or return a bounded correction to the delegate; verify
   changed areas again. Record acceptance or rejection, commands, outcomes,
   and remaining issues in this file. Apply the same checks to primary-owned work.
4. Before any commit, inspect the exact staged diff (including new files and
   the database summary), ensure relevant root checks and required database
   verification passed on that version, and ensure no unresolved review finding
   remains. Run harness checks separately if harness code changed. A subsequent
   edit invalidates affected earlier checks and must be validated again.
5. Stage only intended task changes; preserve pre-existing user work. Delegates
   must not commit, push, or write the tracked database. The primary owns any
   commit and records its hash and validation evidence in the event log. This
   gate applies to incremental commits as well as the final commit; database
   application additionally requires all chunk H prerequisites.

## 8. Verification and database application

Use temporary databases for all experiments and tests. Minimum meaningful
coverage for implementation:

1. Fresh schema; exact current legacy schema; earlier invariant/readings schema;
   repeated open. The order column/index exists and legacy content survives.
2. Constraints reject duplicate/nonpositive/noninteger orders; multiple NULL
   orders are allowed. CLI rejects invalid order values and simultaneous set/clear.
3. Reconcile a copy of the 28/84 baseline to exact ordered slugs, three engines,
   87 core sketches, and zero attempts. Verify all 19 retained/renamed IDs and
   their 57 approach IDs survive; no retired slugs or dangling references remain.
4. Inject failure after rename/upsert/delete stages: all data changes roll back.
   Test destination collisions, extra custom rows, authored writeups/notes, and
   attempts on affected rows; no silent cascade destroys historical work.
5. Test retained notes/writeups/attempts, including composite approach links,
   independently of the empty baseline. Run reconciliation twice; second run
   changes no rows or timestamps. Plain seed/update cannot recreate removals.
6. Dry-run on an old database leaves source schema and rows unchanged. Compare
   its planned identities/actions with apply on an unchanged copy.
7. Exercise CLI add/edit/clear/show/list/matrix and JSON with `--db` set to a
   temporary path. Assert omitted fields survive and order agrees across views.
8. Run root `go test ./...` and `go vet ./...`. Harness source is out of scope;
   if changed incidentally, read `harness/AGENTS.md` and `harness/test-plan.md`
   and run checks separately from `harness/`. Root checks do not cover it.

Application sequence for chunk H:

1. Recheck `git status` and live database inventory; preserve unrelated working
   changes. Make a consistent SQLite backup of `trinkets.db` to a reported path
   outside the repo using the backup API, including committed WAL state. Do not
   copy only the main file while WAL may contain committed changes.
2. Rehearse the entire migration/apply on that backup's temporary copy. Run
   `PRAGMA integrity_check` (expect `ok`) and `PRAGMA foreign_key_check` (no rows),
   compare exact slug/order sets, counts, preserved IDs, and curriculum content.
3. Review dry-run against the tracked database. Apply the same validated command
   explicitly with `--db ./trinkets.db`. The authorized replacement includes the
   nine listed retirements; no extra confirmation is needed for that baseline.
   A changed baseline is a concrete conflict to resolve before proceeding.
4. Repeat integrity, foreign-key, exact-set, and CLI checks on the result. Close
   connections and checkpoint WAL as needed so the tracked main database includes
   the committed state; do not commit transient WAL/SHM files.
5. Record the backup location and a readable before/after summary alongside the
   binary diff. If apply fails, transaction rollback is the first recovery path;
   restoring the backup must also account for active connections and sidecars.
   Do not overwrite newer unrelated user changes during recovery.
6. Follow `update-trinkets-knowledge`: update the existing persistence writeup
   with verified new seed/migration behavior, update its index description if
   needed, and keep both schema pages aligned with actual implementation.

## 9. Definition of done

- [x] All 29 exercises exist in the exact order and have substantive contracts,
  order values, readings, and three appropriate core-map sketches each.
- [x] The nine removed rows and seven obsolete renamed slugs are absent from
  the active database, executable seed, and active catalog guide.
- [x] Fresh and existing databases expose only the new order column safely; opening an
  old database never implicitly deletes catalog data.
- [x] Explicit reconciliation is previewable, atomic for data, repeatable, and
  protects authored/history records through the documented conflict policy.
- [x] CLI and matrix expose curriculum ordering consistently.
- [x] Tests and review pass; both schema pages and relevant knowledge reflect
  implemented behavior, with future harness support clearly labeled planned.
- [x] The tracked `trinkets.db` itself is updated and verified as 29 patterns,
  three engines, 87 core approaches, and zero attempts for the verified baseline.
  Updating source definitions alone does not complete this plan.
