# Potential projects

Five small backend platforms assembled from the lessons in [ideas.md](ideas.md).
Each section walks from the architecture to the lessons that supply its
mechanisms, then to follow-up lessons, then to the explicit chunks of
integration work that no lesson covers. Nothing here is implemented or
planned; a completed lesson proves its small mechanism, not the integration.

Where a Redis-protocol store appears, a lesson picks Valkey or Redis by which
feature set suits it (Redis for JSON, search, vector sets, time series, Bloom
filters or `DELEX`; Valkey for `DELIFEQ` or cluster-mode multi-DB) and
defaults to Redis when it does not matter; lessons already planned or
completed on Valkey stay there. See the
[software catalog](../../software/software.md).

Read a project like this:

- **Architecture** is a proposed diagram plus one paragraph on the decision
  points. Sources live in [diagrams/](diagrams/) and are regenerated with the
  repo's [draw-visual skill](../../.claude/skills/draw-visual/SKILL.md).
- **Components** map each box to the lessons whose mechanism it applies and
  the work chunk that wires it in.
- **Lessons needed** is the minimum set to build the first slice, in a
  sensible build order (readiness and dependencies, not importance).
- **Follow-up lessons** deepen the platform once the slice works.
- **Work to bring it together** lists the integration chunks. Each names what
  it connects and when it is done. Client screens and CLI commands are outside
  the learning scope and assumed to be fully AI-generated; their backend
  contracts, caller identity, authorization and recovery remain in scope.
- **Exit criterion** is the end-to-end behavior that says the slice is done.

**Start here:** the Durable approval-based automation service. Every lesson it
needs is ready today and it exercises the checkpoint, timer, signal and
approval cores that the other four projects reuse. The Focused personal agent
computer is the most complete study of the Muse architecture; its shared
Docker prerequisites below are now built. Begin with one
deterministic task fixture; an LLM can become another caller later without
being needed to test correctness.

## Shared prerequisites

Blocked lessons in ideas.md point at these chunks. Each is configuration work
in the root module, not a lesson; finishing one flips the matching catalog
rows in [software/software.md](../../software/software.md) to `Configured: yes`.
The three Docker chunks are done (2026-10-01): they live in
`internal/lab/docker`, built from the work-item plan in
[docs/plans/docker-harness-prereqs.md](../plans/docker-harness-prereqs.md),
and their catalog rows are `yes`. The evidence is from Linux amd64; the
suite has not yet run on Docker Desktop. The Go S3 helper is still open.

- **Go Docker client and isolation harness (done).** Add the Docker Engine API
  client to the root module and an `internal/lab` launcher that creates,
  execs, stops and removes fixture containers through the active Docker
  context with non-root users, explicit mounts, private PID namespaces,
  dropped capabilities, no-new-privileges, seccomp, `network_mode: none` and
  cgroup limits. Take the OCI runtime handler (`HostConfig.Runtime`) as a
  launcher parameter from day one; it is a no-op under runc and keeps a
  later `runsc` variant a one-field change instead of a redesign. Never
  mount the Docker socket into an untrusted worker.
  Unblocks `worker-capabilities`, `focused-runtime-cell`,
  `noisy-neighbor-limits`, `prepared-container-start` (with BuildKit) and the
  container half of `surrogate-credential-broker` and `enforced-egress-path`.
- **Go S3 helper.** Add one S3 SDK to the module and an `internal/lab`
  connect helper for path-style SeaweedFS at `http://localhost:8333` with
  access key `trinkets` and secret key `trinkets-secret`; translate
  not-found, verify Range and batch delete, and note the whole-second
  `LastModified` from `knowledge/seaweedfs-s3.md`. Unblocks
  `lazy-snapshot-chunks`, `parallel-image-restore`,
  `artifact-generation-publish` and `artifact-gc-grace`.
- **Unix peer-identity harness (done).** A Go broker and fixed-UID Go
  workers, launched through the harness, sharing a dedicated socket directory
  on a named volume, with UID-changing capabilities removed and a check that
  the UID the broker sees is the UID assigned. Unblocks
  `peer-authenticated-tool-broker`.
- **Unix-socket egress broker (done).** The same socket volume plus a broker service
  that alone has network access to fixture services; the destination and
  redirect policy it enforces is the lesson's (`worker-egress-grants`), not
  the harness's. Workers run with `network_mode: none`. Unblocks
  `enforced-egress-path`. Depends on the peer-identity harness.

## Durable approval-based automation service

**Value:** run a small three-step business workflow that waits for approval,
survives worker restarts, and applies its final action once. Use a synthetic
request-review-publish workflow, not a workflow language.

### Architecture

<!-- draw-visual: diagrams/ideas-durable-automation.mmd -->
```text
┌──────────────────────────┐
│      G1 run intake       │
└─────────────┬────────────┘
              ▼
┌──────────────────────────┐
│Run state, timers, signals│
└─────────────┬────────────┘
              ▼
┌──────────────────────────┐
│   G2 wakeup dispatcher   │
└─────────────┬────────────┘
              ▼
┌──────────────────────────┐
│       Step worker        ├────────────┐
└─────────────┬────────────┘            │
              ▼                         ▼
┌──────────────────────────┐   ┌─────────────────┐
│   G4 checkpoint writer   │   │G3 effect adapter│
└─────────────┬────────────┘   └────────┬────────┘
              ▼                         ▼
┌──────────────────────────┐   ┌─────────────────┐
│     Run state update     │   │ Fixture service │
└──────────────────────────┘   └─────────────────┘
```

Run state and step checkpoints are tables in one PostgreSQL database; the
diagram shows the write path separately to keep the persistence loop clear,
not a second store. The worker owns execution and the dispatcher owns wakeups.
A pending approval releases the worker, and a signal means "recheck
authorization", not "permission granted". The effect adapter uses one stable
operation ID across all attempts. End-to-end timing starts at acceptance, not
at the last retry.

### Components

- **G1 run intake** validates a versioned run request, deduplicates the
  submission and binds it to its owner. Lessons: `step-checkpoints`. Work:
  shared contracts, engine assembly.
- **Run state, timers, signals** records pending work, due times, approvals
  and completed steps; it is authoritative when wakeups are missed. Lessons:
  `step-checkpoints`, `durable-deadlines`, `signal-before-wait`. Work: engine
  assembly, approval ingestion.
- **G2 wakeup dispatcher** finds due or signaled work and claims bounded
  attempts, rechecking authorization instead of trusting the signal.
  Lessons: `durable-deadlines`, `signal-before-wait`,
  `scoped-approval-consumption`. Work: engine assembly, approval ingestion.
- **Step worker** runs the next step, reuses recorded results and releases
  capacity while waiting; attempts share one business operation ID. Lessons:
  `step-checkpoints`, `attempt-aware-latency`. Work: engine assembly.
- **G3 effect adapter** executes one external operation and reconciles
  timeouts; its protocol is reusable in the Temporal comparison. Lessons:
  `activity-ack-ambiguity`. Work: effect adapter.
- **G4 checkpoint writer** commits attempt results with guarded state
  transitions and updates runnable state for the dispatcher. Lessons:
  `step-checkpoints`. Work: engine assembly, failure harness.
- **Fixture service** offers an idempotent mock action and a status query so
  duplicate attempts can be told from duplicate effects. Fixture. Work:
  effect adapter, failure harness.

### Lessons needed

All ready today.

1. `step-checkpoints` and `durable-deadlines` for one three-step run.
2. `signal-before-wait` and `scoped-approval-consumption`, so an early
   approval is retained but cannot authorize a different action.
3. `tenant-resource-authorization`, so a run is bound to its owner before any
   approval or effect.

### Follow-up lessons

- `activity-ack-ambiguity` as the Temporal comparison of the same three-step
  run against the same fixture service; keep it a second backend, not a mix.
- `workflow-replay-compatibility` and `fanout-selective-retry` once the
  Temporal version exists.
- `attempt-aware-latency` to report end-to-end time from acceptance.
- `lifecycle-event-audit` to explain any run that stalls.
- `outbox-redelivery` to emit run events to NATS JetStream for other
  services, with `wal-change-feed` as the replication-based alternative.

### Work to bring it together

- **Shared contracts.** Run, step, operation and grant identities; typed
  requests, errors and versioning; trusted caller context. Done when one
  operation can be traced across intake, dispatcher, worker and adapter, and
  spoofed ownership or an incompatible request fails explicitly.
- **Engine assembly.** One run/step schema, the timer/signal dispatcher, the
  checkpoint writer, cancellation, and version rules. Done when every injected
  restart boundary ends in a terminal or explicitly waiting state with no lost
  progress.
- **Approval ingestion.** Independent authenticated approval input that
  binds an immutable action digest, persists as a durable signal and supports
  expiry and revocation. Done when duplicate or early approvals survive
  recovery and a changed action or expired grant cannot dispatch.
- **Effect adapter.** One typed action to one fixture service with a stable
  operation ID, timeout/status reconciliation and bounded retries. Done when
  external success followed by a lost reply is reconciled without a second
  effect.
- **Failure harness.** Reproducible fixtures, restart cut points at every
  transition, invariant checks and DuckDB reporting. Done when the exit
  criterion holds under success, delay, duplicate delivery and restart.

**Exit criterion:** restart across every step and approval boundary, then
drain to one final effect per accepted run with no lost early approval.

## Small multi-tenant job platform

**Value:** accept short computational jobs from two tenants, bound capacity,
and keep a busy tenant from starving the other. Execute fixed Go job binaries
in restricted Docker containers; arbitrary shell jobs are out of scope.

### Architecture

<!-- draw-visual: diagrams/ideas-job-platform.mmd -->
```text
┌───────────────────┐
│     G1 job API    │
└─────────┬─────────┘
          ▼
┌───────────────────┐
│ Tenant job queues │
└─────────┬─────────┘
          ▼
┌───────────────────┐
│    G2 scheduler   │
└─────────┬─────────┘
          │
          ├──────────────────────┐
          │                      │
       reserve               dispatch
          │                      │
          ▼                      ▼
┌───────────────────┐   ┌─────────────────┐
│Capacity and leases│   │G3 worker adapter│
└───────────────────┘   └────────┬────────┘
                                 │
┌───────────────────┐            │
│ Restricted Worker │◄───────────┘
└─────────┬─────────┘
          ▼
┌───────────────────┐
│  G4 result writer │
└───────────────────┘
```

PostgreSQL is authoritative for admitted jobs, reservations and result
generations. Valkey may hold liveness leases as in the lease lesson, but the
result store still validates fencing tokens. Queue fairness and capacity
reservation are different decisions. A worker's lease expiring does not stop
that worker, so duplicate attempts must not publish competing results. Logs
have bounded buffering and explicit loss accounting.

### Components

- **G1 job API** accepts a validated job for a trusted tenant and exposes
  status and cancellation without caller-controlled ownership. Lessons:
  `tenant-resource-authorization`. Work: shared contracts, scheduler
  integration.
- **Tenant job queues** hold accepted jobs grouped for fair selection, as
  PostgreSQL records rather than another broker. Lessons:
  `tenant-fair-queue`. Work: scheduler integration.
- **G2 scheduler** selects a tenant fairly, reserves capacity, dispatches a
  uniquely identified attempt and recovers abandoned reservations. Lessons:
  `tenant-fair-queue`, `atomic-capacity-reservation`. Work: scheduler
  integration.
- **Capacity and leases** track committed slots and worker liveness; expiry
  permits recovery but publication still needs a current fencing token.
  Lessons: `atomic-capacity-reservation`, `lease-reclaim`,
  `stale-owner-fencing`. Work: scheduler integration.
- **G3 worker adapter** turns the control-plane job into a restricted Go
  container, handles cancellation and captures exits and bounded logs.
  Lessons: `worker-capabilities`, `log-stream-backpressure`. Work: shared
  contracts, scheduler integration, result and log delivery.
- **Restricted Worker** executes the fixed job function with explicit
  resource grants and cannot declare its own result authoritative. Lessons:
  `worker-capabilities`. Work: scheduler integration.
- **G4 result writer** atomically checks ownership and generation before
  publishing the terminal result and exposes retained results and log
  references. Lessons: `stale-owner-fencing`, `log-stream-backpressure`.
  Work: result and log delivery.

### Lessons needed

1. `tenant-resource-authorization`, `atomic-capacity-reservation` and
   `tenant-fair-queue` over deterministic jobs (all ready).
2. `lease-reclaim` (planned) and `stale-owner-fencing`, connecting failure
   detection to safe result publication.
3. `worker-capabilities` on the shared Docker harness
   (`internal/lab/docker`); before that lesson, run the job function as a
   plain Go process.

### Follow-up lessons

- `log-stream-backpressure` for bounded log transport.
- `bounded-admission` for explicit overload rejection at the API.
- `partitioned-owner-lease` to test the lease path under Toxiproxy partitions
  rather than clean stops.
- `noisy-neighbor-limits` and `exec-cancel-reap` once containers are real.
- `pooled-connection-modes` when the worker count grows past a handful.
- `lifecycle-reconcile` and `watch-recovery` for worker registration as
  desired versus observed state, with etcd as the registry.

### Work to bring it together

- **Shared contracts.** Tenant, job, attempt and generation identities;
  typed requests and errors; trusted caller context. Done when a job can be
  traced from API to result and spoofed ownership fails explicitly.
- **Scheduler and worker integration.** Worker registration, the Go
  control-plane protocol, fair selection, reservations, lease issuance and
  reclaim, and cancellation. Done when lost workers do not leak capacity and
  stale completions cannot overwrite current results.
- **Docker worker adapter.** Real restricted Go containers driven through the
  shared Docker harness, with fixtures for stale completions and storage
  cleanup. Done when a burst of both tenants runs as bounded containers.
- **Result and log delivery.** Fenced result commit, bounded log transport,
  terminal records and retention cleanup. Done when a slow client cannot grow
  memory indefinitely and a job outcome stays queryable after the worker
  exits.
- **Failure harness.** As above, with worker interruption as the main cut
  point.

**Exit criterion:** burst both tenants, interrupt one worker, and observe
bounded capacity, progress for both tenants, and one accepted result per job.
Multiple physical hosts and a VM fleet are out of scope.

## Focused personal agent computer

**Value:** give one owner a persistent workspace that can run useful code and
perform a narrow set of external actions while policy and credentials stay
outside the workspace's control. Start with one synthetic calendar connector,
one permitted action and one approval type, not a general assistant. This is
the main composition motivated by the
[Muse study](../agentic-platforms/meta-muse.md).

### Architecture

<!-- draw-visual: diagrams/ideas-focused-computer.mmd -->
```text
┌──────────────────────┐
│Untrusted runtime cell│
└───────────┬──────────┘
            │
         request
            ▼
┌──────────────────────┐
│    G1 peer broker    │
└───────────┬──────────┘
            ▼
┌──────────────────────┐
│    G2 action gate    │
└───────────┬──────────┘
            │
            ├───────────────────────────┐
            │                           │
            │                         claim
            │                           │
            ▼                           ▼
┌──────────────────────┐   ┌─────────────────────────┐
│     G3 connector     │   │Approval grants and audit│
└───────────┬──────────┘   └─────────────────────────┘
            │
            ├───────────────────────────┐
            │                           │
            │                        redeem
            │                           │
            ▼                           ▼
┌──────────────────────┐   ┌─────────────────────────┐
│   Fixture service    │   │   G4 credential store   │
└──────────────────────┘   └─────────────────────────┘
```

The runtime is the untrusted side; G1 through G4 and the records they control
are outside it. G2 is the decision point. G3 is the only fixture-service
client and supplies the stable operation ID. Workspace access to G4, the grant
database or a direct network path must be denied by enforcement, not merely
omitted from documentation. A one-use approval permits dispatch; it does not
make the downstream effect atomic with recording completion.

### Components

- **Untrusted runtime cell** runs task code and owns its workspace with no
  authority over supervisor policy or credentials. Lessons:
  `focused-runtime-cell`, `worker-capabilities`. Work: supervisor packaging.
- **G1 peer broker** accepts typed action requests and derives caller
  identity from the connection before checking method permissions. Lessons:
  `peer-authenticated-tool-broker`. Work: shared contracts, supervisor
  packaging.
- **G2 action gate** matches a request to its immutable grant and claims
  dispatch permission; the independent approval API that feeds the grant
  store is omitted from the diagram. Lessons: `scoped-approval-consumption`.
  Work: approval and grant lifecycle.
- **Approval grants and audit** persist grant scope, claim state and action
  history outside the runtime and record ambiguous outcomes for recovery.
  Lessons: `scoped-approval-consumption`, `lifecycle-event-audit`. Work:
  approval and grant lifecycle, failure harness.
- **G3 connector** maps an allowed action to one fixture request with a
  stable operation ID and resolves lost replies without blind redispatch.
  Lessons: `exec-request-dedup`. Work: connector adapter.
- **G4 credential store** redeems scoped handles only for the trusted
  connector and supplies synthetic test credentials from OpenBao. Lessons:
  `surrogate-credential-broker`. Work: credential lifecycle.
- **Fixture service** pretends to be one external calendar service with
  observable effects and status lookup. Fixture. Work: connector adapter,
  failure harness.

### Lessons needed

1. `scoped-approval-consumption`, `surrogate-credential-broker` (broker half
   with OpenBao and a plain Go worker) and `worker-egress-grants`; all ready
   and they define the gate, the credential handle and the policy without a
   container.
2. `container-namespace-boundary` (ready with Compose) to learn what the cell
   must hide before building the cell.
3. On the shared Docker, peer-identity and egress prerequisites (built):
   `worker-capabilities`, `focused-runtime-cell`,
   `peer-authenticated-tool-broker` and `enforced-egress-path`.

### Follow-up lessons

- `exec-request-dedup` and `lifecycle-event-audit` for the connector's lost
  reply and the recovery story.
- `data-taint-policy-model` as a later policy experiment on top of the gate.
- `idle-sandbox-reaper` to stop an idle cell.

### Work to bring it together

- **Shared contracts.** Owner, session, operation and grant identities across
  cell, broker, gate, connector and store. Done when one action can be traced
  end to end and a forged field fails explicitly.
- **Supervisor and runtime packaging.** Compose ordering, separate identities,
  mount/socket/network policy, workspace persistence and restart/update
  behavior around the cell. Run the workspace as a non-root,
  capability-free container with no network and no Docker socket; share only
  a dedicated socket directory through a named volume; the host-side Go
  launcher owns the Docker API. Done when a restart restores the intended
  boundaries and workspace and replacing workspace code cannot change
  supervisor policy. Builds on the shared Docker, peer-identity and egress
  prerequisites, and on the four lessons that use them.
- **Connector adapter.** One typed action to the fixture service with
  timeout/status reconciliation and bounded retries. Done when external
  success plus a lost reply is reconciled without a second effect.
- **Approval and grant lifecycle.** Independent authenticated approval input,
  immutable action binding, durable signals, expiry/revocation and dispatch
  claims. Done when duplicate or early approvals survive recovery and changed
  or expired grants cannot dispatch.
- **Credential lifecycle.** Handle binding, broker-only OpenBao access,
  rotation/revocation and safe error/log handling. Done when a revoked handle
  stops working and no synthetic credential reaches runtime-visible output.
- **Failure harness.** As above, with restart across grant claim and external
  success as the main cut points.

**Exit criterion:** a deliberately noncompliant workspace cannot bypass the
gate, and a lost reply does not duplicate the fixture action. This does not
establish protection against all kernel or prompt-injection attacks.

## Versioned artifact workspace

**Value:** let a few jobs publish related files as one consistent workspace
revision while concurrent readers keep seeing a complete revision. Scope it
to an object-backed document bundle; no POSIX filesystem or collaborative
editor.

### Architecture

<!-- draw-visual: diagrams/ideas-artifact-workspace.mmd -->
```text
┌──────────────────┐   ┌────────────────┐   ┌───────────────┐
│G1 publish session│   │G3 read resolver│   │  G4 collector │
└─────────┬────────┘   └────────┬───────┘   └───────┬───────┘
          ▼                     ▼                   ▼
┌──────────────────┐   ┌────────────────┐   ┌───────────────┐
│  Upload objects  │   │ Revision read  │   │ Retention scan│
└─────────┬────────┘   └────────┬───────┘   └───────┬───────┘
          ▼                     ▼                   ▼
┌──────────────────┐   ┌────────────────┐   ┌───────────────┐
│G2 manifest commit│   │  Read objects  │   │Expired objects│
└─────────┬────────┘   └────────────────┘   └───────────────┘
          ▼
┌──────────────────┐
│  Revision write  │
└──────────────────┘
```

The three lanes are publication, reads and cleanup. Object nodes are views of
one SeaweedFS store; revision and retention nodes are views of one PostgreSQL
database. Upload finishes before manifest commit proceeds. SeaweedFS holds
immutable objects; PostgreSQL holds ownership, manifests, references and the
visible-head pointer. An expected revision protects concurrent writes. A
reader pins one manifest before loading its files. Garbage collection must
consider in-flight publication and active readers, not just absence from the
current head.

### Components

- **G1 publish session** assigns tenant-scoped upload IDs, validates limits
  and hashes and prepares an immutable candidate revision. Lessons:
  `artifact-generation-publish`, `tenant-resource-authorization`. Work:
  shared contracts, workspace API.
- **Upload objects** stores immutable bytes before a manifest makes them
  visible; unfinished uploads need abandonment cleanup. Lessons:
  `artifact-generation-publish`. Work: workspace API.
- **G2 manifest commit** checks the expected revision and atomically advances
  the visible head with object references. Lessons:
  `artifact-generation-publish`, `workspace-write-conflicts`. Work:
  workspace API.
- **Revision write** stores manifests, ownership, reader pins and the head;
  shared by readers and cleanup. Lessons: `workspace-write-conflicts`,
  `lifecycle-event-audit`. Work: workspace API.
- **G3 read resolver** authenticates the tenant and pins one manifest before
  resolving object keys. Lessons: `tenant-resource-authorization`,
  `artifact-generation-publish`. Work: workspace API.
- **Revision read** and **Read objects** are the read-side views of the same
  stores; every lookup is tied to one retained revision. Lessons:
  `artifact-generation-publish`. Work: workspace API.
- **G4 collector**, **Retention scan** and **Expired objects** find
  unreferenced objects after grace and pin checks and retry bounded
  deletions. Lessons: `artifact-gc-grace`. Work: retention, failure harness.

### Lessons needed

1. `tenant-resource-authorization` and `workspace-write-conflicts` (ready)
   for ownership and the expected-revision check on PostgreSQL alone.
2. After the shared Go S3 helper: `artifact-generation-publish` for one small
   bundle format, then `artifact-gc-grace`.

### Follow-up lessons

- `lifecycle-event-audit` to explain an orphaned object or a stuck upload.
- `lazy-snapshot-chunks` if bundles grow large enough for partial reads.
- `usage-interval-reconciliation` for storage accounting per tenant.

### Work to bring it together

- **Shared contracts.** Tenant, upload, revision and object identities;
  typed errors for conflict, missing and expired. Done when a publish can be
  traced from upload to head change.
- **Workspace API.** Upload sessions, validated manifests, atomic head
  changes, reader pins with expiry, and abandonment rules. Done when
  concurrent reading and publication preserve complete pinned revisions.
- **Retention.** Batched deletion with grace and pin checks, object-store
  error translation and bounded scans. Done when abandoned uploads become
  reclaimable and no pinned artifact is deleted.
- **Failure harness.** Concurrent publish, read and cleanup with restart cut
  points in each lane.

**Exit criterion:** concurrent publish/read/cleanup never produces a mixed
revision or deletes a pinned artifact; abandoned uploads eventually become
reclaimable.

## Prepared-environment runner

**Value:** start repeated instances of one fixed toolchain quickly and show
where startup time moves as prepared state and caching increase. Keep one
image family and two modeled hosts before attempting general placement.

### Architecture

<!-- draw-visual: diagrams/ideas-prepared-runner.mmd -->
```text
┌───────────────────┐
│G1 template catalog│
└─────────┬─────────┘
          ▼
┌───────────────────┐
│ G2 pool controller│
└─────────┬─────────┘
          │
        claim
          ▼
┌───────────────────┐
│  G3 host adapter  │
└─────────┬─────────┘
          │
          ├──────────────────────┐
          │                      │
       create               read chunks
          │                      │
          ▼                      ▼
┌───────────────────┐   ┌─────────────────┐
│ Two modeled hosts │   │  G4 chunk cache │
└─────────┬─────────┘   └────────┬────────┘
          │                      │
          │               missing│chunks
          ▼                      ▼
┌───────────────────┐   ┌─────────────────┐
│Session and command│   │ SeaweedFS chunks│
└───────────────────┘   └─────────────────┘
```

The two modeled hosts are scheduling fixtures; the complete project uses one
local Docker Engine with two logical capacity buckets, not two failure
domains. The host adapter launches real Go containers. Verified chunks
materialize a versioned workspace bundle; the prepared OCI image supplies the
runtime. Template identity is immutable and separate from each writable
session. The pool owns capacity; the host adapter owns creation and
readiness. A cache hit never substitutes for checking the template's content
identity. Record both time to ready and time to first successful command,
since lazy input materialization shifts latency between them.

### Components

- **G1 template catalog** records immutable template hashes, runtime
  compatibility and preparation status so partial templates cannot be
  claimed. Lessons: `lazy-snapshot-chunks`, `prepared-container-start`.
  Work: template and host adapter.
- **G2 pool controller** claims exact-match inventory within capacity and
  replenishes a bounded pool, allowing for stale host reports. Lessons:
  `warm-pool-claims`, `atomic-capacity-reservation`, `sampled-placement`.
  Work: cache and pool coordination.
- **G3 host adapter** implements create/inspect/stop with stable request IDs
  and session generations and maps backend failures to lifecycle state.
  Lessons: `session-generation`, `prepared-container-start`. Work: shared
  contracts, template and host adapter.
- **Two modeled hosts** provide controlled capacity and startup delay; the
  complete project maps them to real local containers. Lessons:
  `sampled-placement`, `atomic-capacity-reservation`. Work: template and host
  adapter.
- **Session and command** is one writable instance and its readiness and
  first-command probe; model and container results stay separate. Lessons:
  `session-generation`, `prepared-container-start`. Work: template and host
  adapter, failure harness.
- **G4 chunk cache** fetches and verifies immutable chunks, bounds storage
  and coordinates concurrent reads and eviction. Lessons:
  `lazy-snapshot-chunks`, `parallel-image-restore`. Work: cache and pool
  coordination.
- **SeaweedFS chunks** stores versioned workspace chunks for the data-path
  experiment; the Docker image supplies the runtime. Lessons:
  `lazy-snapshot-chunks`, `parallel-image-restore`. Work: template and host
  adapter.

### Lessons needed

1. `atomic-capacity-reservation`, `warm-pool-claims` and `session-generation`
   (ready) as the control-plane model before any real runtime.
2. `image-layer-pull-cost` (ready) to measure the registry side of
   preparation.
3. After the shared Go S3 helper: `lazy-snapshot-chunks` and
   `parallel-image-restore`.
4. After BuildKit is configured (the shared Docker client is built):
   `prepared-container-start`.

### Follow-up lessons

- `sampled-placement` once correctness with a fixed host is clear.
- `lifecycle-reconcile` so the host adapter converges desired and observed
  containers after a lost acknowledgement.
- `deletion-tombstone` for cleanup after failed creation.
- `preview-route-generation` when a session exposes a port and routes must
  follow the current generation.
- `idle-sandbox-reaper` to return idle warm instances to the pool.
- `noisy-neighbor-limits` when two instances share the engine.

### Work to bring it together

- **Shared contracts.** Template, session, request and generation
  identities. Done when a claim can be traced from pool to container.
- **Template and host adapter.** OCI build/import manifest, content identity,
  runtime compatibility, the Docker create/inspect/stop contract and a
  container readiness probe. Done when one template creates an independently
  writable container and a failed start is cleaned up. Blocked on BuildKit;
  the shared Docker client is built.
- **Cache and pool coordination.** Concurrent fetch deduplication, cache
  limits, pin/evict rules, exact-match pool claims and bounded replenishment.
  Done when corrupt or partial templates never become ready and bursts and
  host loss respect capacity. The model half is ready now; the real runtime
  depends on the adapter.
- **Failure harness.** Interrupted fetches, failed creates and pool
  exhaustion as cut points; report preparation, cold creation and warm claims
  separately.

**Exit criterion:** reconstruct verified input bytes, launch independently
writable containers from one prepared image, claim warm instances once,
execute the probe, and reclaim failed or finished runs. No VM boot or
memory-restore performance claim is made.
