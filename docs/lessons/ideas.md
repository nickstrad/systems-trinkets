# Lesson ideas

Candidate lessons, grouped by the core abstractions behind agentic platforms
(execution, lifecycle, scheduling, storage, networking, durable work). The
groups come from the [platform research](../agentic-platforms/README.md); each
idea is a proposed local analogy, not a measured result or a claim about how a
provider implements it. [projects.md](projects.md) shows how ideas combine into
small platforms and what integration work sits between them.

Every idea has a stable kebab-case slug, one question, a comparison, an
observable invariant, and a measurement. Every experiment analyzes its CSV with
DuckDB. `ready` means every required dependency is `Configured: yes` in the
[software catalog](../../software/software.md); `blocked` names what is missing.
Recheck the catalog before planning: the label here is a convenience. There is
no ranking; pick any ready idea, or start from a project in projects.md.
Entries without a Plan line are unplanned. Each entry's Options line lists
combinations of configured software that could implement it, separated by
`·`; "no setup" means every piece is `Configured: yes` today, otherwise it
names the catalog row that still needs setup or the row to add. Follow
[AGENTS.md](AGENTS.md) when editing and the [lifecycle](README.md) when
promoting an idea to a plan.

Redis-protocol store policy: an idea names Valkey or Redis by which feature set
suits it (Redis for JSON, search, vector sets, time series, Bloom filters or
`DELEX`; Valkey for `DELIFEQ`, cluster-mode multi-DB or an explicit
comparison) and defaults to Redis when it does not matter; ideas already
planned or completed on Valkey stay there. See the
[software catalog](../../software/software.md).

Runtime boundary in one line: Docker Desktop/Compose plus Go on macOS, Linux
probes inside containers, no KVM or VM runtimes; details in README.md.

Existing code-only lessons under `lessons/go/` (`background-job-queue`,
`cache-aside`, `completed-job-counter`, `pipelining-work`) supply baselines
that several ideas build on; those entries say so.

## Execution environments and isolation

Processes, container namespaces on a shared Linux kernel, permission
boundaries, and CPU/memory resource limits.
Research: [Daytona](../agentic-platforms/daytona.md),
[E2B](../agentic-platforms/e2b.md), [Modal](../agentic-platforms/modal.md),
[Vercel Sandbox](../agentic-platforms/vercel-sandbox.md),
[Muse](../agentic-platforms/meta-muse.md).

### worker-capabilities

**Can a worker reach resources outside its grant?**

- Compare: a Go probe process with broad fixture mounts versus explicit
  read-only/read-write mounts and a network-disabled container; fixed
  allow/deny probes.
- Invariant: allowed probes succeed; denied probes have no effect on protected
  fixtures.
- Measure: allowed/denied probe counts; operation latency ms.
- Software: Go; Docker Engine API and Docker isolation harness. Blocked: the
  Go Docker client and launcher are unconfigured (see the shared prerequisites
  in projects.md). Goroutines are not a security boundary.
- Options: Docker Compose services with read-only/read-write volumes and
  `network_mode: none`, launched from Go with `docker compose run` (no setup;
  static fixtures only) · Docker Engine API Go client plus isolation harness
  for dynamic create/exec (setup: catalog rows `Docker Engine API` and
  `Docker isolation harness` are `no`).

### focused-runtime-cell

**Can workspace code alter the supervisor?**

- Compare: broad fixture-only mounts/permissions versus a non-root worker with
  explicit mounts, private PID namespace, dropped capabilities,
  no-new-privileges and seccomp; harmless probes.
- Invariant: the worker cannot modify supervisor policy/state or inspect
  supervisor processes; approved tasks still work.
- Measure: forbidden successes; allowed task failures; startup ms.
- Software: Go; Docker Engine API and Docker isolation harness. Blocked:
  harness unconfigured.
- Options: Docker Compose with `user`, `cap_drop`, `security_opt` (no-new-
  privileges, seccomp), `pid` and explicit volumes (no setup; static
  topology) · Docker Engine API Go client plus isolation harness (setup: both
  catalog rows `no`).
- Builds on: `worker-capabilities` (same harness and probe style; this adds
  the supervisor as the protected party).

### container-namespace-boundary

**What can shared versus private namespaces expose?**

- Compare: two fixture containers with a shared PID namespace and shared
  workspace mount versus private PID namespaces and separate writable mounts;
  inspect visibility from each side.
- Invariant: the private variant hides the other fixture's processes and
  writable files; both variants still share the Docker host kernel.
- Measure: visibility probe outcomes; start-to-command ms.
- Software: Go; Docker Compose. Ready: Compose expresses both variants with
  `pid: "service:<name>"` and named volumes; no Engine API needed. No
  separate-kernel claim.
- Options: Docker Compose with `pid: "service:<name>"` and named volumes,
  probes in Go (no setup) · Docker Engine API Go client if containers must be
  created dynamically (setup: catalog row `no`).
- Follow-up: the same probe under runc versus gVisor's `runsc` through the
  launcher's runtime-handler parameter, to see what a user-space kernel hides
  (reported kernel, synthetic `/proc`, accounting-only in-sandbox cgroups).
  Blocked: catalog row `gVisor (runsc)` is `no`; runsc must be installed
  inside the Docker Desktop VM, which is unsupported today. Not a replacement
  cell runtime: the Muse-influenced broker lessons assume a shared kernel.

### noisy-neighbor-limits

**Does a resource cap protect another workload?**

- Compare: two bounded workloads in the same Docker VM, with and without
  CPU/memory caps; read cgroup counters inside the containers.
- Invariant: the configured hard memory limit is enforced; allocation failures
  and OOM outcomes are recorded.
- Measure: neighbor p95 ms; throttled ms; peak bytes; OOM count.
- Software: Go; Docker Engine API and cgroups v2. Blocked: Go Docker client
  unconfigured. No macOS host-limit claim.
- Options: Docker Compose `mem_limit`/`cpus` with Go reading `/sys/fs/cgroup`
  inside each container (no setup once the cgroup v2 files are verified in
  Docker Desktop; flip the `cgroups v2` row to `yes`) · Docker Engine API
  stats and OOM events (setup: catalog row `no`).

## Sandbox identity and lifecycle

Durable sandbox identity, sessions, desired versus observed state,
reconciliation, stop/pause/resume, idle timeout, and teardown.
Research: [Daytona](../agentic-platforms/daytona.md),
[E2B](../agentic-platforms/e2b.md),
[Vercel Sandbox](../agentic-platforms/vercel-sandbox.md).

### lifecycle-reconcile

**Does desired state converge after an acknowledgement is lost?**

- Compare: a one-shot mock-runner command versus persistent desired/observed
  state with reconciliation.
- Invariant: after bounded recovery, desired equals observed and each sandbox
  has at most one allocation.
- Measure: convergence ms; retries; orphan allocations.
- Software: Go; PostgreSQL. Ready; simulated runner.
- Options: PostgreSQL desired/observed tables (no setup) · etcd KV with
  revisions as the observed store (no setup) · NATS JetStream KV bucket (no
  setup).

### session-generation

**Can a previous session update a resumed sandbox?**

- Compare: reuse the sandbox ID alone versus sandbox ID plus a generation
  checked on every update.
- Invariant: an old session cannot change the current session's state.
- Measure: stale updates rejected; resume transition ms.
- Software: Go; PostgreSQL. Ready; simulated sessions.
- Options: PostgreSQL row with generation column and conditional update (no
  setup) · Redis Lua compare-and-set (no setup) · etcd transaction comparing
  mod revision (no setup).
- Builds on: the compare-and-swap core from `stale-owner-fencing`.

### deletion-tombstone

**Can a delayed create completion resurrect a deleted sandbox?**

- Compare: delete the row immediately versus retain a generation-aware
  tombstone until cleanup.
- Invariant: a late completion cannot restore a deleted generation;
  allocations are eventually cleaned.
- Measure: resurrections; cleanup ms; tombstones retained.
- Software: Go; PostgreSQL. Ready; simulated runner.
- Options: PostgreSQL tombstone rows with cleanup job (no setup) · Redis
  tombstone key with TTL and keyspace-notification cleanup (no setup).
- Builds on: `session-generation`.

### watch-recovery

**Can a controller recover after missing change notifications?**

- Compare: watch-only local state versus a revisioned snapshot plus watch;
  force a disconnect and a compaction.
- Invariant: recovered controller state equals the authoritative live keys.
- Measure: missing keys; resync ms; revisions replayed.
- Software: Go; etcd. Ready.
- Options: etcd watch plus revisioned snapshot, Toxiproxy to force the
  disconnect (no setup) · NATS JetStream KV watch with stream sequence (no
  setup) · PostgreSQL logical replication slot as the watch (no setup).

### idle-sandbox-reaper

**Does an idle timeout fire once per sandbox when workers restart?**
(Added 2026-09-29 from the configured catalog.)

- Compare: a poller scanning last-activity timestamps versus a Redis key
  with TTL whose expiry event (`__keyevent@0__:expired`) triggers the stop;
  restart the reaper mid-run and let activity extend the TTL.
- Invariant: every idle sandbox is stopped exactly once after its timeout;
  activity within the window prevents the stop; a missed event is caught by
  reconciliation.
- Measure: stop lateness ms; duplicate stops; missed expiries after restart.
- Software: Go; Redis keyspace notifications and PostgreSQL. Ready.
- Options: Redis TTL key plus `__keyevent@0__:expired` subscriber with
  PostgreSQL as truth (no setup) · PostgreSQL due-time poller as the baseline
  (no setup) · Temporal timer per sandbox as a third variant (no setup).
- Builds on: `lease-reclaim` (TTL as liveness) and `lifecycle-reconcile`.

## Ownership and coordination

Leases, heartbeats, ownership tokens, fencing, and coordination of concurrent
or stale actors. Fencing is a lesson-design abstraction, not a claim that every
provider implements it.
Research: [Daytona](../agentic-platforms/daytona.md),
[E2B](../agentic-platforms/e2b.md),
[synthesis](../agentic-platforms/README.md).

### lease-reclaim

**Can another worker take over when the owner disappears?**

- Compare: a permanent owner key versus an expiring, token-checked lease.
- Invariant: only matching tokens renew or release; a replacement acquires
  after expiry; no downstream fencing claim.
- Measure: recovery ms; failed acquisition count.
- Software: Go; Valkey. Ready.
- Options: Valkey `SET NX PX` with Lua token renewal (no setup; the planned
  guide) · etcd lease with keepalive (no setup) · PostgreSQL row with
  `expires_at` (no setup).
- Plan: [planned/lease-reclaim.md](planned/lease-reclaim.md).

### stale-owner-fencing

**Can an expired owner still write?**

- Compare: a lease check alone versus an increasing token checked atomically
  by the destination; delay the old writer.
- Invariant: once a newer token is accepted, all older-token writes are
  rejected.
- Measure: stale writes accepted/rejected; takeover ms.
- Software: Go; PostgreSQL. Ready.
- Options: PostgreSQL destination checking a token column (no setup) · etcd
  transaction on token (no setup) · NATS JetStream publish with expected last
  sequence (no setup).
- Builds on: `lease-reclaim` (this is the missing downstream half).

### partitioned-owner-lease

**Does a partitioned owner behave like a crashed one?**
(Added 2026-09-29 from the configured catalog.)

- Compare: the owner talks to Valkey (as in `lease-reclaim`) directly versus
  through a Toxiproxy proxy that is cut, delayed, or reset mid-lease; observe
  both the owner's view and the contender's.
- Invariant: during a partition the owner's renewals fail visibly and the
  contender acquires only after expiry; when the partition heals the old
  owner does not regain ownership without reacquiring.
- Measure: renewal failures; takeover ms; split-ownership intervals.
- Software: Go; Valkey and Toxiproxy. Ready.
- Options: Valkey behind a Toxiproxy proxy on 22000-22009 (no setup) · etcd
  or PostgreSQL behind the same proxy (no setup).
- Builds on: `lease-reclaim`; the same injection applies to `watch-recovery`
  and `outbox-redelivery` later.

## Placement, capacity, and warm pools

Host selection, resource inventories, admission, quotas, warm capacity,
autoscaling, connection pooling, and cache locality.
Research: [E2B](../agentic-platforms/e2b.md),
[Daytona](../agentic-platforms/daytona.md),
[Modal](../agentic-platforms/modal.md).

### atomic-capacity-reservation

**Can concurrent admissions oversubscribe a host?**

- Compare: read-then-insert versus capacity check and reservation in one
  transaction.
- Invariant: committed allocations never exceed declared slots; retries
  preserve request identity.
- Measure: oversubscriptions; rejections; admission ms.
- Software: Go; PostgreSQL. Ready; modeled hosts.
- Options: PostgreSQL single transaction with row lock (no setup) · etcd
  transaction (no setup) · Redis Lua decrement-if-available (no setup) · add
  PgBouncer in front for many admitters (no setup).

### warm-pool-claims

**How much ready inventory absorbs a burst?**

- Compare: simulated cold provisioning versus an atomic claim from a bounded
  exact-match pool.
- Invariant: a ready instance is claimed at most once; inventory stays within
  its cap.
- Measure: ready latency ms; pool misses; idle slot-ms.
- Software: Go; PostgreSQL. Ready; modeled provisioning.
- Options: PostgreSQL `for update skip locked` claim (no setup) · Redis list
  pop as the pool (no setup) · NATS JetStream work-queue stream (no setup).
- Builds on: `atomic-capacity-reservation`.

### sampled-placement

**How does stale load affect host selection?**

- Compare: random placement versus best-of-two sampled hosts; the host
  atomically enforces its own capacity.
- Invariant: no accepted placement exceeds host capacity despite stale
  reports.
- Measure: load skew; retries; rejected placements.
- Software: Go. Ready; deterministic host/load simulation.
- Options: Go-only simulation (no setup) · PostgreSQL per-host capacity rows
  for the atomic check (no setup) · etcd transaction per host (no setup).

### pooled-connection-modes

**What breaks when many runners share one PostgreSQL through a pooler?**
(Added 2026-09-29 from the configured catalog.)

- Compare: direct connections versus PgBouncer in transaction mode versus
  session mode, with a runner count above the pool size; include prepared
  statements and a session-level setting.
- Invariant: every runner's transaction commits or fails explicitly; no
  runner observes another runner's session state.
- Measure: wait-for-connection ms; server connections used; failures by kind.
- Software: Go; PgBouncer and PostgreSQL. Ready.
- Options: PgBouncer on 6432 in transaction then session mode against
  PostgreSQL (no setup).

## Images, templates, and snapshots

Prepared environments, immutable bases, image layers, copy-on-write,
filesystem versus memory snapshots, lazy reads, restore pipelines, registries
and caches.
Research: [E2B](../agentic-platforms/e2b.md),
[Vercel Sandbox](../agentic-platforms/vercel-sandbox.md),
[Daytona](../agentic-platforms/daytona.md),
[Modal](../agentic-platforms/modal.md).

### lazy-snapshot-chunks

**What does fetching only the working set save?**

- Compare: eager download versus on-demand fetch of immutable, verified
  chunks.
- Invariant: all reads match the reference bytes; corrupt chunks are rejected.
- Measure: first-read ms; fetched bytes; miss p95 ms.
- Software: Go; SeaweedFS and a Go S3 client. Blocked: no Go S3 helper in the
  module yet (see projects.md shared prerequisites). Data-path model, not VM
  paging.
- Options: SeaweedFS via a Go S3 SDK (setup: catalog row `Go S3 client` is
  `no`; `go get` one SDK and add an `internal/lab` helper) · OCI registry
  blobs through `go-containerregistry` as digest-addressed chunks (no setup)
  · NATS JetStream object store as the chunk store (no setup).

### prepared-container-start

**What startup work can preparation remove?**

- Compare: the same Go workload initialized at container start versus a
  baked prepared image versus a one-use warm container claim; record
  image-cache state.
- Invariant: each instance runs the same probe with independent writable
  state; a warm instance is claimed at most once.
- Measure: ready ms; first-command ms; preparation ms; idle container-ms;
  failures.
- Software: Go; Docker Engine API and BuildKit. Blocked: Go Docker client
  unconfigured. No memory checkpoint/restore claim.
- Options: Docker Engine API plus BuildKit through Go (setup: both catalog
  rows `no`) · partial: Compose `build:` for the baked image and the OCI
  registry to push/pull it, timing via `docker compose up` from Go (no setup;
  no dynamic warm claims).
- Builds on: `warm-pool-claims`.

### parallel-image-restore

**When does parallel restore help?**

- Compare: serial versus bounded-parallel Range reads of independently
  compressed chunks.
- Invariant: the restored image hash matches the reference, including after
  interrupted retries.
- Measure: restore ms; transferred bytes; peak buffer bytes.
- Software: Go; SeaweedFS and a Go S3 client. Blocked: Go S3 helper
  unconfigured; verify Range support when planning.
- Options: SeaweedFS Range reads via a Go S3 SDK (setup: `Go S3 client` row
  `no`) · OCI registry blob Range requests via `go-containerregistry` (no
  setup) · NATS JetStream object store chunks (no setup).
- Builds on: `lazy-snapshot-chunks`.

### image-layer-pull-cost

**How much of a pull is avoided when layers are shared?**
(Added 2026-09-29 from the configured catalog.)

- Compare: pulling two images that share no layers versus two that share a
  base layer from the local registry, with a cold and a warm local cache.
- Invariant: manifests and layer digests verify; a shared layer is stored and
  transferred once.
- Measure: pull ms; bytes transferred; layers reused.
- Software: Go (`go-containerregistry`); OCI registry. Ready.
- Options: OCI registry on 5050 with `go-containerregistry` (no setup;
  library `go get`).

## Workspaces, volumes, and artifacts

Private writable state, shared persistent volumes, object storage, metadata
pointers, artifact publication, retention, and cleanup.
Research: [Daytona](../agentic-platforms/daytona.md),
[E2B](../agentic-platforms/e2b.md),
[Vercel Sandbox](../agentic-platforms/vercel-sandbox.md).

### artifact-generation-publish

**Can readers observe a partially published result?**

- Compare: overwrite visible artifacts versus an immutable generation plus an
  atomic metadata-pointer swap.
- Invariant: readers see a complete old or new generation, never mixed
  members.
- Measure: mixed reads; publication ms; unreferenced objects.
- Software: Go; PostgreSQL, SeaweedFS and a Go S3 client. Blocked: Go S3
  helper unconfigured.
- Options: PostgreSQL pointer plus SeaweedFS objects via a Go S3 SDK (setup:
  `Go S3 client` row `no`) · PostgreSQL plus NATS JetStream object store (no
  setup) · PostgreSQL plus OCI registry blobs, digest-addressed and immutable
  (no setup).

### artifact-gc-grace

**Can cleanup race with an upload becoming referenced?**

- Compare: immediate orphan deletion versus a grace period and metadata
  recheck.
- Invariant: no artifact referenced by a committed generation is deleted.
- Measure: live deletions; retained orphan bytes; reclaim delay ms.
- Software: Go; PostgreSQL, SeaweedFS and a Go S3 client. Blocked: Go S3
  helper unconfigured.
- Options: Same stores as `artifact-generation-publish`: SeaweedFS (setup:
  `Go S3 client` row `no`) · JetStream object store (no setup) · OCI registry
  with delete enabled (no setup).
- Builds on: `artifact-generation-publish`.

### workspace-write-conflicts

**What happens when two writers edit the same file?**

- Compare: blind last-write-wins versus expected-version compare-and-swap.
- Invariant: conflicting edits are explicit rather than silently lost.
- Measure: lost edits; conflicts; write ms.
- Software: Go; PostgreSQL. Ready; models file metadata, not FUSE/POSIX.
- Options: PostgreSQL expected-version update (no setup) · etcd transaction
  on mod revision (no setup) · NATS JetStream KV update with revision (no
  setup).
- Builds on: the compare-and-swap core from `stale-owner-fencing`.

## Container control and interactive I/O

Runner-to-container protocols, sandbox daemons, command execution, process
lifetime, file operations, terminals, and log streams.
Research: [Daytona](../agentic-platforms/daytona.md),
[E2B](../agentic-platforms/e2b.md),
[Vercel Sandbox](../agentic-platforms/vercel-sandbox.md),
[Muse](../agentic-platforms/meta-muse.md).

### peer-authenticated-tool-broker

**Can one connector impersonate another?**

- Compare: caller-supplied identity versus Unix peer UID/GID plus a
  method/worker ACL; separate fixed-UID workers inside Docker with
  UID-changing capabilities removed.
- Invariant: a forged request field cannot expand the authenticated peer's
  method or credential scope.
- Measure: unauthorized accepts; ACL denials; dispatch ms.
- Software: Go; Docker Unix peer-identity harness. Blocked: harness
  unconfigured; broker and sockets stay inside Docker Linux.
- Options: Go `SO_PEERCRED` broker and fixed-UID workers as Compose services
  sharing a socket volume with `cap_drop` (setup: catalog row `Docker Unix
  peer-identity harness` is `no`; Compose can host it, the missing piece is
  the launcher and UID validation) · OpenBao per-worker tokens as a weaker
  identity baseline (no setup).

### exec-request-dedup

**What if exec starts but its reply is lost?**

- Compare: a mock daemon that accepts each retry versus a durable execution
  ID with status lookup.
- Invariant: one execution ID launches at most one modeled operation within
  retained history.
- Measure: duplicate starts; outcome lookup ms; ambiguous results.
- Software: Go; PostgreSQL. Ready; no real container daemon.
- Options: PostgreSQL execution table (no setup) · Redis `SET NX` execution
  ID with TTL (no setup) · NATS JetStream `Nats-Msg-Id` dedup window (no
  setup).
- Builds on: `completed-job-counter` (idempotency key) and
  `step-checkpoints`; this applies the same core at the daemon boundary.

### log-stream-backpressure

**Can a slow reader exhaust daemon memory?**

- Compare: an unbounded enqueue baseline (with a hard experiment cap) versus
  a bounded queue with an explicit drop/block policy.
- Invariant: the buffer stays bounded; each emitted sequence is delivered or
  counted as dropped.
- Measure: peak buffered bytes; drops; producer blocked ms.
- Software: Go goroutines/channels. Ready; message-stream model, not
  process isolation or PTY/WebSocket.
- Options: Go channels only (no setup) · Redis Streams with `MAXLEN` as the
  bounded buffer (no setup) · NATS JetStream stream limits with discard
  policy (no setup).

### exec-cancel-reap

**Does cancelling a command also stop its descendants?**

- Compare: signal the parent only versus process-group cancellation with a
  deadline and parent-owned waits; a Go fixture process tree inside one Linux
  container.
- Invariant: all fixture descendants that remain in the group exit by the
  cleanup deadline and their parents collect exit status.
- Measure: surviving processes; cancellation ms; missing exit statuses.
- Software: Go; Docker Compose. Ready: one Compose service running the Go
  fixture tree is enough; escaped sessions are outside this fixture.
- Options: One Docker Compose service running the Go fixture tree (no setup)
  · Docker Engine API exec for a dynamic variant (setup: catalog row `no`).

## Networking and access boundaries

Lifecycle APIs versus workload traffic, sandbox routing, preview proxies,
ingress/egress, authentication, authorization, credential mediation, and
per-environment access policy. Identity and secrets mechanisms need further
research before specific provider claims.
Research: [Daytona](../agentic-platforms/daytona.md),
[E2B](../agentic-platforms/e2b.md), [Modal](../agentic-platforms/modal.md),
[Muse](../agentic-platforms/meta-muse.md).

### tenant-resource-authorization

**Is a valid sandbox ID sufficient permission?**

- Compare: resource-ID lookup versus authenticated owner plus resource
  authorization on each operation.
- Invariant: no cross-tenant read/write passes, including guessed IDs.
- Measure: unauthorized successes; denied requests; check ms.
- Software: Go; PostgreSQL. Ready; trusted caller identity fixture, not an
  authentication implementation. Every project in projects.md needs this.
- Options: PostgreSQL ownership join on every operation (no setup) · OpenBao
  tokens and policies as the caller-identity fixture (no setup).

### scoped-approval-consumption

**Can a grant authorize a changed or repeated action?**

- Compare: a boolean approved flag versus an immutable action digest with
  expiry, revocation and an atomic one-use claim; change the recipient/body
  and race two claims.
- Invariant: no mismatched, expired or revoked grant starts dispatch; a
  one-use grant admits at most one operation ID.
- Measure: invalid accepts; replay rejects; approval-to-dispatch ms.
- Software: Go; PostgreSQL. Ready; fixture approval issuer, no UI or live
  connector.
- Options: PostgreSQL grant row with atomic claim (no setup) · Redis Lua
  one-use claim or `DELEX <key> IFEQ <token>` (no setup) · Temporal signal delivering the grant to a
  workflow-side claim (no setup).

### surrogate-credential-broker

**Can a worker use a credential without receiving its value?**

- Compare: a synthetic token in the worker's input versus an opaque handle
  the worker presents to a separate Go broker, which redeems it against
  OpenBao for one fixed mock action.
- Invariant: the worker cannot redeem the handle for another action or read
  the raw token; the token is absent from worker-visible output and errors.
- Measure: token exposure probes; scope violations; broker ms.
- Software: Go; OpenBao and PostgreSQL. Ready with the worker as a plain Go
  process. The restricted-container worker is a later step that needs the
  Docker isolation harness (blocked).
- Options: OpenBao secrets redeemed by a Go broker, handles in PostgreSQL,
  worker as a Go process (no setup) · restricted container worker (setup:
  catalog row `Docker isolation harness` is `no`).

### enforced-egress-path

**Can a job bypass its outbound policy check?**

- Compare: a proxy environment variable alone versus Docker
  `network_mode: none` plus a Go Unix-socket action broker; probe direct IP,
  host gateway, alternate ports and redirects.
- Invariant: blocked fixture destinations receive zero requests through every
  tested path; the approved broker action works.
- Measure: bypass successes; blocked requests; broker overhead ms.
- Software: Go; Docker isolation harness and Unix-socket egress broker.
  Blocked: harnesses unconfigured. Narrow actions, not a transparent proxy.
- Options: Compose worker with `network_mode: none` plus a broker service on
  a shared socket volume (setup: catalog rows `Docker isolation harness` and
  `Unix-socket egress broker` are `no`; Compose can host both) · Toxiproxy as
  the only reachable fixture upstream to prove the block (no setup).
- Builds on: `worker-egress-grants` (the policy half, ready today).

### worker-egress-grants

**How narrowly can a broker authorize destinations?**

- Compare: broad versus explicit per-worker destination grants in a Go
  broker against controlled local HTTP fixtures; every redirect rechecked.
- Invariant: through the broker API, allowed destinations work and denied
  destinations receive no request.
- Measure: allowed/denied requests; broker ms.
- Software: Go standard library. Ready; application-policy experiment.
  Network bypass enforcement is `enforced-egress-path`.
- Options: Go standard library broker and local HTTP fixtures (no setup) ·
  Toxiproxy proxies as controlled destinations (no setup).

### preview-route-generation

**Can a stale preview route reach a replacement session?**

- Compare: a sandbox-only cached route versus generation-aware validation
  and invalidation.
- Invariant: a route cannot deliver to a different tenant or an obsolete
  generation.
- Measure: misroutes; stale rejections; route lookup ms.
- Software: Go; PostgreSQL and Redis. Ready; route resolution model, no
  proxy.
- Options: PostgreSQL routes cached in Redis, optionally as JSON documents (no setup) · etcd watch pushing
  invalidations (no setup) · Redis keyspace notifications on route expiry
  (no setup) · a real proxy would use `httputil.ReverseProxy` (catalog row
  `no`, standard library; mark `yes` when used).
- Builds on: `session-generation`.

### data-taint-policy-model

**What should happen when output provenance is unknown?**

- Compare: assume clean by default versus explicit clean/sensitive/unknown
  labels carried through a fixed task graph.
- Invariant: sensitive and unknown outputs never enter the clean automatic
  dispatch path; manual policy may separately authorize.
- Measure: incorrect automatic allows; approval count; policy latency ms.
- Software: Go. Ready; explicit-label simulation, not eBPF or byte-level
  taint tracking.
- Options: Go-only label graph (no setup) · PostgreSQL storing labels and
  decisions (no setup).

## Events, queues, and flow control

Event ingestion, trigger matching, enqueue/delivery acknowledgements,
redelivery, change feeds, per-key concurrency, fairness, backpressure, and
queue-to-executor handoff.
Research: [Inngest](../agentic-platforms/inngest.md),
[Vercel Workflow](../agentic-platforms/vercel-workflow.md),
[Temporal and Restate](../agentic-platforms/durable-execution.md).

### outbox-redelivery

**Can committed work survive a publish/ack interruption?**

- Compare: direct write-then-publish versus a transactional outbox with a
  duplicate-safe consumer.
- Invariant: every committed event eventually applies once despite repeated
  deliveries.
- Measure: missing effects; duplicate attempts; delivery lag ms.
- Software: Go; PostgreSQL and NATS JetStream. Ready.
- Options: PostgreSQL outbox plus NATS JetStream consumer (no setup) ·
  PostgreSQL plus Redis Streams consumer group (no setup) · PostgreSQL
  logical replication reading the outbox table (no setup; see `wal-change-
  feed`).
- Builds on: `completed-job-counter`; extends its idempotent apply to a
  broker boundary.

### tenant-fair-queue

**Can one tenant monopolize workers?**

- Compare: global FIFO versus round-robin tenant queues with a fixed
  active-step cap.
- Invariant: no job is lost or applied twice; each nonempty tenant progresses
  under bounded input.
- Measure: per-tenant wait p95 ms; starvation intervals; throughput jobs/s.
- Software: Go; PostgreSQL. Ready; controlled arrivals.
- Options: PostgreSQL per-tenant queues (no setup) · Redis Streams per
  tenant (no setup) · NATS JetStream per-tenant subjects with max ack pending
  (no setup).
- Builds on: `background-job-queue`.

### bounded-admission

**What happens when arrivals exceed execution capacity?**

- Compare: an unbounded backlog baseline (with a safety cap) versus a bounded
  queue with explicit rejection.
- Invariant: accepted jobs complete after drain; the queue never exceeds its
  configured bound in the variant.
- Measure: queue depth; rejected jobs; wait ms.
- Software: Go; Redis Streams. Ready; bounded overload experiment.
- Options: Redis Streams `MAXLEN` (no setup) · NATS JetStream max messages
  with discard-new (no setup) · PostgreSQL count check in the admitting
  transaction (no setup).
- Builds on: `background-job-queue`.

### wal-change-feed

**Can a cache follow committed changes without an application publish step?**
(Added 2026-09-29 from the configured catalog.)

- Compare: the outbox consumer from `outbox-redelivery` versus a logical
  replication slot (`wal_level=logical`) feeding the same Redis cache; kill
  the consumer mid-stream and restart.
- Invariant: after restart the cache reflects every committed row exactly
  once; no change is skipped or applied out of commit order.
- Measure: apply lag ms; replayed changes after restart; slot retained bytes.
- Software: Go; PostgreSQL logical replication and Redis. Ready.
- Options: PostgreSQL logical replication (`pgoutput`) decoded in Go, applied
  to Redis (no service setup; add a replication client library such as
  `pglogrepl` to the module and note it in the catalog row) · compare against
  the JetStream outbox consumer (no setup).
- Builds on: `outbox-redelivery` and `cache-aside`.

## Durable execution and recovery

Run and step identity, event histories, journals, checkpointed results,
replay, retries, idempotent effects, keyed state, and code-version
compatibility.
Research: [Vercel Workflow](../agentic-platforms/vercel-workflow.md),
[Inngest](../agentic-platforms/inngest.md),
[Temporal and Restate](../agentic-platforms/durable-execution.md).

### step-checkpoints

**Which work repeats after a worker restart?**

- Compare: whole-job retry versus persisted results keyed by run and step.
- Invariant: recorded outputs are reused; retries do not duplicate the
  idempotent business effect.
- Measure: step attempts; duplicate effects; recovery ms.
- Software: Go; PostgreSQL. Ready; three-step job.
- Options: PostgreSQL run/step results (no setup) · NATS JetStream KV per run
  and step (no setup) · Temporal as the engine-owned comparison, which is
  `activity-ack-ambiguity`.
- Builds on: `completed-job-counter`.

### activity-ack-ambiguity

**Can a durable workflow duplicate an external effect?**

- Compare: a Temporal Activity with an unconditional effect versus one with a
  business idempotency key; interrupt after the effect commits.
- Invariant: the idempotent variant applies one effect per operation despite
  repeated Activity attempts.
- Measure: attempts/effects; retry delay ms; completion ms.
- Software: Go; Temporal and PostgreSQL. Ready.
- Options: Temporal dev server with the effect in PostgreSQL (no setup).
- Builds on: `step-checkpoints` (same question, engine-owned retries).

### workflow-replay-compatibility

**What does a code change do to an in-flight history?**

- Compare: change command order incompatibly versus a version-aware workflow
  branch.
- Invariant: recorded history replays compatibly or fails visibly; it never
  silently changes prior decisions.
- Measure: replay failures; replay ms; history events.
- Software: Go; Temporal. Ready; bounded recorded workflow.
- Options: Temporal dev server (no setup).

## Timers, signals, and orchestration

Persisted waits, deadlines, hooks, external signals, fan-out/join, and
resumption without keeping a worker occupied.
Research: [Vercel Workflow](../agentic-platforms/vercel-workflow.md),
[Inngest](../agentic-platforms/inngest.md),
[Temporal and Restate](../agentic-platforms/durable-execution.md).

### durable-deadlines

**Do timers survive all workers being offline?**

- Compare: an in-process timer versus a persisted due time with overdue
  reconciliation.
- Invariant: every due action applies once after restart, including duplicate
  wakeups.
- Measure: missed actions; wakeup lateness ms; idle workers.
- Software: Go; PostgreSQL. Ready.
- Options: PostgreSQL due-time rows (no setup) · Redis TTL keys with
  keyspace notifications as wakeups (no setup) · Temporal timers as the
  engine-owned variant (no setup).

### signal-before-wait

**Is an approval lost if it arrives before the waiter?**

- Compare: an ephemeral notification versus a persisted signal ID with
  consume state.
- Invariant: each valid signal resolves its intended run at most once,
  regardless of arrival order.
- Measure: lost/duplicate signals; resume ms.
- Software: Go; PostgreSQL `LISTEN`/`NOTIFY`. Ready; the durable row is
  authoritative.
- Options: PostgreSQL row plus `LISTEN`/`NOTIFY` (no setup) · Temporal
  signal-with-start (no setup) · NATS core publish as the ephemeral baseline
  (no setup).

### fanout-selective-retry

**Must one failed item repeat all completed work?**

- Compare: retry the whole batch versus independent persisted item results
  with a join.
- Invariant: the join completes only with one successful result per item;
  successful effects are not repeated.
- Measure: repeated items; completion ms; maximum active items.
- Software: Go; Temporal and PostgreSQL. Ready.
- Options: Temporal child workflows or activities with PostgreSQL effects (no
  setup) · PostgreSQL-only with per-item checkpoints from `step-checkpoints`
  (no setup).

## Observability and resource accounting

Lifecycle events, execution attempts, queue time, startup/readiness
boundaries, resource usage, and operational counters. Billing and pricing
semantics need more evidence; the research mainly covers runtime
observations.
Research: [E2B](../agentic-platforms/e2b.md),
[Modal](../agentic-platforms/modal.md),
[Inngest](../agentic-platforms/inngest.md).

### attempt-aware-latency

**Does a fast successful attempt hide slow user completion?**

- Compare: successful-attempt-only timing versus an operation timeline that
  includes queueing and retries.
- Invariant: every accepted operation is accounted for as complete, failed,
  or censored.
- Measure: end-to-end and attempt p95 ms; queue ms; censored count.
- Software: Go. Ready; bounded queue/retry model with CSV events. This is
  measurement method more than mechanism; consider folding it into
  `knowledge/performance-labs.md` instead of a lesson.
- Options: Go model writing CSV (no setup) · replay any project's PostgreSQL
  event log into DuckDB (no setup).

### lifecycle-event-audit

**Can events explain an orphaned allocation?**

- Compare: current-state rows alone versus correlated lifecycle events plus
  current state.
- Invariant: every allocation has a traceable request/generation and a
  terminal or live disposition.
- Measure: unexplained allocations; recovery timeline ms; event count.
- Software: Go; PostgreSQL. Ready; application event log, not OpenTelemetry.
- Options: PostgreSQL event table joined to current state (no setup) · NATS
  JetStream stream as the replayable event log (no setup).

### usage-interval-reconciliation

**Can duplicate or missing stop events distort usage?**

- Compare: summing reported durations versus deduplicated intervals
  reconciled with lifecycle state.
- Invariant: no interval is counted twice; unknown end times stay explicit
  rather than billed as complete.
- Measure: duplicate intervals; unknown intervals; resource-ms discrepancy.
- Software: Go; PostgreSQL. Ready: the usage ledger is tables the lesson
  creates on the configured PostgreSQL, not separate software.
- Options: PostgreSQL ledger tables (no setup; the catalog row `Usage ledger
  in PostgreSQL` is lesson tables, now marked `yes`) · DuckDB over exported
  intervals for the reconciliation query (no setup).
- Builds on: `lifecycle-event-audit`.
