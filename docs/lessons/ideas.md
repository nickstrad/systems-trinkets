# Lesson ideas

Candidate lessons organized by the core abstractions behind agentic platforms.
The initial three-per-aspect pass is extended with six ideas from the
[Muse research](../agentic-platforms/meta-muse.md).
The sections synthesize the existing [platform research](../agentic-platforms/README.md);
they do not imply that every provider uses the same design. These experiments
are proposed local analogies, not measured results or provider implementation claims.

Follow [AGENTS.md](AGENTS.md) when adding rows and the [lifecycle](README.md)
when promoting them to plans. Each row captures a stable idea slug and
question, its comparison, correctness invariant, measurement, software readiness,
research, and lifecycle status. `ready` means every required dependency is marked
`Configured: yes` in the [software catalog](../../software/software.md);
`blocked` names what is missing. Readiness must be rechecked before planning.

`Order` is a single global impact ranking: 1 is highest, with no ties or gaps
across all 42 ideas. It prioritizes preventing tenant-boundary and correctness
failures, then broadly reusable recovery/control-plane mechanisms, then capacity
and performance refinements. Close choices favor concepts that unlock more
later lessons, then stable slug order. This is an editorial first pass, not a
measured score or a strict prerequisite sequence. Readiness and completion do
not change impact rank; choose the lowest-ranked-number eligible unplanned idea
when building the next lesson. Rows within each aspect are sorted by Order.

Every experiment uses DuckDB for analysis. Each row names its runtime and
additional software; `ready` means catalog eligibility, not a validated plan.
Blocked ideas retain their impact rank. Further research is needed for the
usage-ledger proposal, which is an accounting extension of lifecycle observations.
The existing lease guide is included as a planned entry, not duplicated.

The Muse additions put constrained runtime authority, trusted broker identity,
and action authorization ahead of throughput optimizations. Existing ideas
retain their relative order; ranks shift to make room. Projects at the bottom
reference stable slugs and do not participate in the lesson ranking.

## Local execution requirement

Every idea and complete project must have a local macOS path using Docker
Desktop/Compose and Go. Lesson and platform code is Go only; Deno/TypeScript
is reserved for k6 tooling. Backing services run in Docker; DuckDB and k6
remain the existing analysis/load tools. No KVM, Firecracker, nested
virtualization, separately managed Linux VM, Apple container, or alternate VM runtime is required or planned.
Containerd/nerdctl may be used for a specific learning benefit after its local
Linux-engine integration is configured; backing services still use Docker. Linux-specific probes run inside
Docker containers, with ARM64-compatible images on Apple silicon.

Portability is a design requirement, not a claim that an unimplemented harness
has passed. Keep missing Docker/Go integrations marked blocked until configured.
Full projects must finish with real container/process execution where promised;
simulated runners are intermediate steps, not substitutes for their exit criteria.

Two ideas were deliberately replaced to fit this boundary:
`guest-kernel-boundary` → `container-namespace-boundary`, and
`microvm-snapshot-restore` → `prepared-container-start`.
The first teaches namespace visibility on a shared kernel; the second teaches
preparation and warm reuse, without claiming VM-memory restoration. The global
ranking was reconsidered: boundary literacy and prepared startup retain their
relative positions (17 and 32); the 42-rank order otherwise remains unchanged.

## Execution environments and isolation

Processes, container namespaces on a shared Linux kernel, permission boundaries, and CPU/memory resource limits.

Research: [Daytona](../agentic-platforms/daytona.md), [E2B](../agentic-platforms/e2b.md), [Modal](../agentic-platforms/modal.md), [Vercel Sandbox](../agentic-platforms/vercel-sandbox.md). Additional context: [Muse](../agentic-platforms/meta-muse.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 1 | `worker-capabilities` — Can a worker reach resources outside its grant? | Go probe process with broad fixture mounts versus explicit read-only/read-write mounts and a network-disabled container; fixed allow/deny probes | Allowed probes succeed; denied probes have no effect on protected fixtures | Allowed/denied probe counts; operation latency ms | Go; Docker Engine API and Docker isolation harness — blocked: integrations unconfigured; goroutines are not a security boundary | [Modal](../agentic-platforms/modal.md) | idea |
| 2 | `focused-runtime-cell` — Can workspace code alter the supervisor? | Broad fixture-only mounts/permissions versus non-root worker, explicit mounts, private PID namespace, dropped capabilities, no-new-privileges and seccomp; harmless probes | Worker cannot modify supervisor policy/state or inspect supervisor processes; approved tasks still work | Forbidden successes; allowed task failures; startup ms | Go; Docker Engine API and Docker isolation harness — blocked: integrations unconfigured | [Muse](../agentic-platforms/meta-muse.md) | idea |
| 17 | `container-namespace-boundary` — What can shared versus private namespaces expose? | Two fixture containers with shared PID namespace/workspace mount versus private PID namespaces and separate writable mounts; inspect visibility | Private variant hides the other fixture process and writable files; both variants still share the Docker host kernel | Visibility probe outcomes; start-to-command ms | Go; Docker Engine API and Docker isolation harness — blocked: integrations unconfigured; no separate-kernel claim | [E2B](../agentic-platforms/e2b.md) | idea |
| 34 | `noisy-neighbor-limits` — Does a resource cap protect another workload? | Two bounded workloads in the same Docker VM, with and without CPU/memory caps; inspect cgroup counters inside containers | Configured hard memory limit is enforced; record allocation failures/OOM outcomes | Neighbor p95 ms; throttled ms; peak bytes; OOM count | Go; Docker Engine API and cgroups v2 — blocked: integrations unconfigured; no macOS host-limit claim | [Modal](../agentic-platforms/modal.md) | idea |

## Sandbox identity and lifecycle

Durable sandbox identity, running sessions, desired and observed state, reconciliation, stop/pause/resume, and teardown.

Research: [Daytona](../agentic-platforms/daytona.md), [E2B](../agentic-platforms/e2b.md), [Vercel Sandbox](../agentic-platforms/vercel-sandbox.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 3 | `lifecycle-reconcile` — Does desired state converge after an acknowledgement is lost? | One-shot mock-runner command versus persistent desired/observed state reconciliation | After bounded recovery, desired equals observed and each sandbox has at most one allocation | Convergence ms; retries; orphan allocations | Go; PostgreSQL — ready; simulated runner | [Daytona](../agentic-platforms/daytona.md) | idea |
| 20 | `session-generation` — Can a previous session update a resumed sandbox? | Reuse sandbox ID alone versus sandbox ID plus generation checked on updates | An old session cannot change the current session state | Stale updates rejected; resume transition ms | Go; PostgreSQL — ready; simulated sessions | [Vercel Sandbox](../agentic-platforms/vercel-sandbox.md) | idea |
| 30 | `deletion-tombstone` — Can delayed create completion resurrect a deleted sandbox? | Delete row immediately versus retain a generation-aware tombstone until cleanup | Late completion cannot restore a deleted generation; allocations eventually cleaned | Resurrections; cleanup ms; tombstones retained | Go; PostgreSQL — ready; simulated runner | [Daytona](../agentic-platforms/daytona.md) | idea |

## Ownership and coordination

Leases, heartbeats, ownership tokens, fencing, and coordination of concurrent or stale actors. Fencing is a lesson-design abstraction, not a claim that every provider implements it.

Research: [Daytona](../agentic-platforms/daytona.md), [E2B](../agentic-platforms/e2b.md), [Cross-platform synthesis](../agentic-platforms/README.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 4 | `lease-reclaim` — Can another worker take over when the owner disappears? | Permanent owner key versus expiring token-checked lease | Only matching tokens renew/release; replacement acquires after expiry; no downstream fencing claim | Recovery ms; failed acquisition count | Go; Valkey — ready | [Daytona](../agentic-platforms/daytona.md) | [planned](planned/lease-reclaim.md) |
| 5 | `stale-owner-fencing` — Can an expired owner still write? | Lease check alone versus increasing token checked atomically by destination; delay old writer | Once a newer token is accepted, all older-token writes are rejected | Stale writes accepted/rejected; takeover ms | Go; PostgreSQL — ready | [Synthesis](../agentic-platforms/README.md) | idea |
| 28 | `watch-recovery` — Can a controller recover after missing change notifications? | Watch-only local state versus revisioned snapshot plus watch; force disconnect/compaction | Recovered controller state equals authoritative live keys | Missing keys; resync ms; revisions replayed | Go; etcd — ready | [E2B](../agentic-platforms/e2b.md) | idea |

## Placement, capacity, and warm pools

Host selection, resource inventories, admission, quotas, warm capacity, autoscaling, and cache locality.

Research: [E2B](../agentic-platforms/e2b.md), [Daytona](../agentic-platforms/daytona.md), [Modal](../agentic-platforms/modal.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 12 | `atomic-capacity-reservation` — Can concurrent admissions oversubscribe a host? | Read-then-insert versus capacity check and reservation in one transaction | Committed allocations never exceed declared slots; retries preserve request identity | Oversubscriptions; rejections; admission ms | Go; PostgreSQL — ready; modeled hosts | [Daytona](../agentic-platforms/daytona.md) | idea |
| 23 | `warm-pool-claims` — How much ready inventory absorbs a burst? | Simulated cold provisioning versus atomic claim of a bounded exact-match pool | A ready instance is claimed at most once; inventory remains within cap | Ready latency ms; pool misses; idle slot-ms | Go; PostgreSQL — ready; modeled provisioning | [Daytona](../agentic-platforms/daytona.md) | idea |
| 35 | `sampled-placement` — How does stale load affect host selection? | Random placement versus best of two sampled hosts; host atomically enforces capacity | No accepted placement exceeds host capacity despite stale reports | Load skew; retries; rejected placements | Go — ready; deterministic host/load simulation | [E2B](../agentic-platforms/e2b.md) | idea |

## Images, templates, and snapshots

Prepared environments, immutable bases, image layers, copy-on-write, filesystem versus memory snapshots, lazy reads, restore pipelines, and caches.

Research: [E2B](../agentic-platforms/e2b.md), [Vercel Sandbox](../agentic-platforms/vercel-sandbox.md), [Daytona](../agentic-platforms/daytona.md), [Modal](../agentic-platforms/modal.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 22 | `lazy-snapshot-chunks` — What does fetching only the working set save? | Eager download versus demand fetch of immutable verified chunks | All reads match reference bytes; corrupt chunks rejected | First-read ms; fetched bytes; miss p95 ms | Go; SeaweedFS and Go S3 client — blocked: Go S3 client unconfigured; data-path model, not VM paging | [E2B](../agentic-platforms/e2b.md) | idea |
| 32 | `prepared-container-start` — What startup work can preparation remove? | Same Go workload initialized at container start versus baked prepared image versus one-use warm container claim; record image-cache state | Each instance runs the same probe with independent writable state; a warm instance is claimed at most once | Ready ms; first-command ms; preparation ms; idle container-ms; failures | Go; Docker Engine API and BuildKit — blocked: integrations unconfigured; no memory checkpoint/restore | [E2B](../agentic-platforms/e2b.md) | idea |
| 40 | `parallel-image-restore` — When does parallel restore help? | Serial versus bounded parallel Range reads of independently compressed chunks | Restored image hash matches reference, including interrupted retries | Restore ms; transferred bytes; peak buffer bytes | Go; SeaweedFS and Go S3 client — blocked: Go S3 client unconfigured; verify Range support when planning | [Vercel Sandbox](../agentic-platforms/vercel-sandbox.md) | idea |

## Workspaces, volumes, and artifacts

Private writable state, shared persistent volumes, object storage, metadata pointers, artifact publication, retention, and cleanup.

Research: [Daytona](../agentic-platforms/daytona.md), [E2B](../agentic-platforms/e2b.md), [Vercel Sandbox](../agentic-platforms/vercel-sandbox.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 15 | `artifact-generation-publish` — Can readers observe a partially published result? | Overwrite visible artifacts versus immutable generation plus atomic metadata-pointer swap | Readers see a complete old or new generation, never mixed members | Mixed reads; publication ms; unreferenced objects | Go; PostgreSQL and SeaweedFS and Go S3 client — blocked: Go S3 client unconfigured; concurrent generation publication | [E2B](../agentic-platforms/e2b.md) | idea |
| 29 | `artifact-gc-grace` — Can cleanup race with an upload becoming referenced? | Immediate orphan deletion versus grace period and metadata recheck | No artifact referenced by a committed generation is deleted | Live deletions; retained orphan bytes; reclaim delay ms | Go; PostgreSQL and SeaweedFS and Go S3 client — blocked: Go S3 client unconfigured | [Daytona](../agentic-platforms/daytona.md) | idea |
| 39 | `workspace-write-conflicts` — What happens when two writers edit the same file? | Blind last-write-wins versus expected-version compare-and-swap | Conflicting edits are explicit rather than silently lost | Lost edits; conflicts; write ms | Go; PostgreSQL — ready; models file metadata, not FUSE/POSIX | [Daytona](../agentic-platforms/daytona.md) | idea |

## Container control and interactive I/O

Runner-to-container protocols, sandbox daemons, command execution, process lifetime, file operations, terminals, and log streams.

Research: [Daytona](../agentic-platforms/daytona.md), [E2B](../agentic-platforms/e2b.md), [Vercel Sandbox](../agentic-platforms/vercel-sandbox.md). Additional context: [Muse](../agentic-platforms/meta-muse.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 7 | `peer-authenticated-tool-broker` — Can one connector impersonate another? | Caller-supplied identity versus Unix peer UID/GID plus method/worker ACL; separate fixed-UID workers inside Docker, with UID-changing capabilities removed | A forged request field cannot expand the authenticated peer's method or credential scope | Unauthorized accepts; ACL denials; dispatch ms | Go; Docker Unix peer-identity harness — blocked: harness unconfigured; broker and sockets stay in Docker Linux | [Muse](../agentic-platforms/meta-muse.md) | idea |
| 21 | `exec-request-dedup` — What if exec starts but its reply is lost? | Mock daemon accepts each retry versus durable execution ID/status lookup | One execution ID launches at most one modeled operation within retained history | Duplicate starts; outcome lookup ms; ambiguous results | Go; PostgreSQL — ready; no real container daemon | [E2B](../agentic-platforms/e2b.md) | idea |
| 31 | `log-stream-backpressure` — Can a slow reader exhaust daemon memory? | Unbounded enqueue baseline with hard experiment cap versus bounded queue and explicit drop/block policy | Buffer stays bounded; each emitted sequence is delivered or counted as dropped | Peak buffered bytes; drops; producer blocked ms | Go goroutines/channels — ready; bounded message-stream model, not process isolation or PTY/WebSocket | [Daytona](../agentic-platforms/daytona.md) | idea |
| 41 | `exec-cancel-reap` — Does cancelling a command also stop its descendants? | Signal parent only versus process-group cancellation with deadline and parent-owned waits; Go fixture tree inside Docker | All fixture descendants that remain in the group exit by cleanup deadline and their parents collect exit status | Surviving processes; cancellation ms; missing exit statuses | Go; sandbox daemon/process-control harness — blocked: integration unconfigured; escaped sessions are outside this fixture | [E2B](../agentic-platforms/e2b.md) | idea |

## Networking and access boundaries

Lifecycle APIs versus workload traffic, sandbox routing, preview proxies, ingress/egress, authentication, and per-environment access policy. Detailed identity and secrets mechanisms need further research before specific provider claims.

Research: [Daytona](../agentic-platforms/daytona.md), [E2B](../agentic-platforms/e2b.md), [Modal](../agentic-platforms/modal.md). Additional context: [Muse](../agentic-platforms/meta-muse.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 6 | `tenant-resource-authorization` — Is a valid sandbox ID sufficient permission? | Resource-ID lookup versus authenticated owner plus resource authorization on each operation | No cross-tenant read/write passes, including guessed IDs | Unauthorized successes; denied requests; check ms | Go; PostgreSQL — ready; trusted caller identity fixture, not authentication implementation | [Modal](../agentic-platforms/modal.md) | idea |
| 8 | `scoped-approval-consumption` — Can a grant authorize a changed or repeated action? | Boolean approved flag versus immutable action digest, expiry, revocation and atomic one-use claim; change recipient/body and race two claims | No mismatched, expired or revoked grant starts dispatch; one-use grant admits at most one operation ID | Invalid accepts; replay rejects; approval-to-dispatch ms | Go; PostgreSQL — ready; fixture approval issuer, no UI or live connector | [Muse](../agentic-platforms/meta-muse.md) | idea |
| 9 | `surrogate-credential-broker` — Can a worker use a credential without receiving its value? | Synthetic token in Go worker input versus opaque handle redeemed by a separate Go broker for a fixed mock action; restricted container worker | Worker cannot redeem for another action or read raw token; token absent from worker-visible output and errors | Token exposure probes; scope violations; broker ms | Go; Docker isolation and Unix-socket egress broker — blocked: harnesses unconfigured; token stored only outside worker | [Muse](../agentic-platforms/meta-muse.md) | idea |
| 10 | `enforced-egress-path` — Can a job bypass its outbound policy check? | Proxy environment variable alone versus Docker network_mode: none and a Go Unix-socket action broker; probe direct IP, host gateway, alternate ports and redirects | Blocked fixture destinations receive zero requests through every tested path; approved broker action works | Bypass successes; blocked requests; broker overhead ms | Go; Docker isolation harness and Unix-socket egress broker — blocked: harnesses unconfigured; narrow actions, not a transparent general proxy | [Muse](../agentic-platforms/meta-muse.md) | idea |
| 25 | `preview-route-generation` — Can a stale preview route reach a replacement session? | Sandbox-only cached route versus generation-aware validation and invalidation | A route cannot deliver to a different tenant or obsolete generation | Misroutes; stale rejections; route lookup ms | Go; PostgreSQL and Valkey — ready; route resolution model, no proxy | [E2B](../agentic-platforms/e2b.md) | idea |
| 36 | `worker-egress-grants` — How narrowly can a broker authorize destinations? | Broad versus explicit per-worker destination grants in a Go broker against controlled local HTTP fixtures; every redirect rechecked | Through the broker API, allowed destinations work and denied destinations receive no request | Allowed/denied requests; broker ms | Go standard library — ready; application-policy experiment; direct-network bypass enforcement is the separate enforced-egress-path idea | [Modal](../agentic-platforms/modal.md) | idea |

## Events, queues, and flow control

Event ingestion, trigger matching, enqueue/delivery acknowledgements, redelivery, per-key concurrency, fairness, backpressure, and queue-to-executor handoff.

Research: [Inngest](../agentic-platforms/inngest.md), [Vercel Workflow](../agentic-platforms/vercel-workflow.md), [Temporal and Restate](../agentic-platforms/durable-execution.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 11 | `outbox-redelivery` — Can committed work survive a publish/ack interruption? | Direct write/publish versus transactional outbox and duplicate-safe consumer | Every committed event eventually applies once despite repeated deliveries | Missing effects; duplicate attempts; delivery lag ms | Go; PostgreSQL and NATS JetStream — ready; extends counter lesson to broker boundary | [Inngest](../agentic-platforms/inngest.md) | idea |
| 18 | `tenant-fair-queue` — Can one tenant monopolize workers? | Global FIFO versus round-robin tenant queues with a fixed active-step cap | No job lost or applied twice; each nonempty tenant progresses under bounded input | Per-tenant wait p95 ms; starvation intervals; throughput jobs/s | Go; PostgreSQL — ready; controlled arrivals | [Inngest](../agentic-platforms/inngest.md) | idea |
| 26 | `bounded-admission` — What happens when arrivals exceed execution capacity? | Unbounded backlog baseline with safety cap versus bounded queue and explicit rejection | Accepted jobs complete after drain; queue never exceeds configured bound in variant | Queue depth; rejected jobs; wait ms | Go; Valkey Streams — ready; bounded overload experiment | [Inngest](../agentic-platforms/inngest.md) | idea |

## Durable execution and recovery

Run and step identity, event histories, journals, checkpointed results, replay, retries, idempotent effects, keyed state, and code-version compatibility.

Research: [Vercel Workflow](../agentic-platforms/vercel-workflow.md), [Inngest](../agentic-platforms/inngest.md), [Temporal and Restate](../agentic-platforms/durable-execution.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 13 | `step-checkpoints` — Which work repeats after a worker restart? | Whole-job retry versus persisted results keyed by run/step | Recorded outputs are reused; retries do not duplicate the idempotent business effect | Step attempts; duplicate effects; recovery ms | Go; PostgreSQL — ready; three-step job | [Inngest](../agentic-platforms/inngest.md) | idea |
| 14 | `activity-ack-ambiguity` — Can a durable workflow duplicate an external effect? | Temporal Activity with unconditional effect versus business idempotency key; interrupt after effect commit | Idempotent variant applies one effect per operation despite repeated Activity attempts | Attempts/effects; retry delay ms; completion ms | Go; Temporal and PostgreSQL — ready | [Temporal / Restate](../agentic-platforms/durable-execution.md) | idea |
| 33 | `workflow-replay-compatibility` — What does a code change do to an in-flight history? | Change command order incompatibly versus version-aware workflow branch | Recorded history replays compatibly or fails visibly; never silently changes prior decisions | Replay failures; replay ms; history events | Go; Temporal — ready; bounded recorded workflow | [Temporal / Restate](../agentic-platforms/durable-execution.md) | idea |

## Timers, signals, and orchestration

Persisted waits, deadlines, hooks, external signals, fan-out/join, and resumption without keeping a worker occupied.

Research: [Vercel Workflow](../agentic-platforms/vercel-workflow.md), [Inngest](../agentic-platforms/inngest.md), [Temporal and Restate](../agentic-platforms/durable-execution.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 16 | `durable-deadlines` — Do timers survive all workers being offline? | In-process timer versus persisted due time and overdue reconciliation | Every due action applies once after restart, including duplicate wakeups | Missed actions; wakeup lateness ms; idle workers | Go; PostgreSQL — ready | [Vercel Workflow](../agentic-platforms/vercel-workflow.md) | idea |
| 24 | `signal-before-wait` — Is an approval lost if it arrives before the waiter? | Ephemeral notification versus persisted signal ID and consume state | Each valid signal resolves its intended run at most once, regardless of arrival order | Lost/duplicate signals; resume ms | Go; PostgreSQL LISTEN/NOTIFY — ready; durable row is authoritative | [Temporal / Restate](../agentic-platforms/durable-execution.md) | idea |
| 38 | `fanout-selective-retry` — Must one failed item repeat all completed work? | Retry whole batch versus independent persisted item results and join | Join completes only with one successful result per item; successful effects not repeated | Repeated items; completion ms; maximum active items | Go; Temporal and PostgreSQL — ready | [Vercel Workflow](../agentic-platforms/vercel-workflow.md) | idea |

## Observability and resource accounting

Lifecycle events, execution attempts, queue time, startup/readiness boundaries, resource usage, and operational counters. Billing ledgers and pricing semantics require further evidence; the current research mainly covers runtime observations.

Research: [E2B](../agentic-platforms/e2b.md), [Modal](../agentic-platforms/modal.md), [Inngest](../agentic-platforms/inngest.md). Additional context: [Muse](../agentic-platforms/meta-muse.md).

| Order | Idea / question | Mechanism / comparison | Invariant | Measurement | Software / readiness | Research | Status / plan |
|---|---|---|---|---|---|---|---|
| 19 | `attempt-aware-latency` — Does a fast successful attempt hide slow user completion? | Successful-attempt-only timing versus operation timeline including queueing/retries | Every accepted operation accounted for as complete, failed, or censored | End-to-end and attempt p95 ms; queue ms; censored count | Go — ready; bounded queue/retry model with CSV events | [Inngest](../agentic-platforms/inngest.md) | idea |
| 27 | `lifecycle-event-audit` — Can events explain an orphaned allocation? | Current-state rows alone versus correlated lifecycle events plus current state | Every allocation has a traceable request/generation and terminal or live disposition | Unexplained allocations; recovery timeline ms; event count | Go; PostgreSQL — ready; application event log, not OpenTelemetry | [Daytona](../agentic-platforms/daytona.md) | idea |
| 37 | `data-taint-policy-model` — What should happen when output provenance is unknown? | Assume clean by default versus explicit clean/sensitive/unknown states carried through a fixed task graph | Sensitive and unknown outputs never enter the clean automatic-dispatch path; manual policy may separately authorize | Incorrect automatic allows; approval count; policy latency ms | Go — ready; explicit-label simulation, not eBPF or byte-level taint tracking | [Muse](../agentic-platforms/meta-muse.md) | idea |
| 42 | `usage-interval-reconciliation` — Can duplicate or missing stop events distort usage? | Sum reported durations versus deduplicated intervals reconciled with lifecycle state | No interval counted twice; unknown end times remain explicit rather than billed as complete | Duplicate intervals; unknown intervals; resource-ms discrepancy | Go; PostgreSQL usage ledger — blocked: catalog marks ledger unconfigured | [Synthesis](../agentic-platforms/README.md) | idea |

## Potential projects

First draft: five possible platforms, with lesson sequences and explicit integration
work. Choose one small end-to-end slice before combining platforms.

These are small backend platforms assembled from the ideas above, not additional
ranked lessons or implemented plans. Each starts with one workload and a small
fixture population. Their scope can span several lessons; the one-or-two-service
limit still applies to each individual lesson, not to the eventual composition.

CLI, web UI, approval screens, and other client presentation are outside the
learning scope and assumed to be **100% AI-generated**. Their API contracts,
authenticated caller identity, server-side authorization, and recovery semantics
remain part of the platform work. Begin with deterministic task fixtures;
an LLM can become another caller later without being needed to test correctness.

Diagrams are proposed architecture. A component can apply one or more lessons
and still need integration work. The component tables distinguish **Idea**
(existing lesson slugs), **Gap** (an `Oxx` side quest below), and **Fixture**
(test-only infrastructure). G labels identify integration components, not
completed implementations. Arrows show requests or dependencies, not all replies. Sources live in
[diagrams/](diagrams/) and are regenerated with the repo's
[draw-visual skill](../../.claude/skills/draw-visual/SKILL.md).

| Project | Value proposition | Core ideas | Gaps to finish it |
|---|---|---|---|
| [Focused personal agent computer](#focused-personal-agent-computer) | Useful code execution and narrowly authorized actions in a persistent workspace | Runtime cell, trusted broker, approvals, credential mediation, egress enforcement | Supervisor wiring, connector protocol, credential lifecycle, restart recovery |
| [Durable automation service](#durable-approval-based-automation-service) | Background tasks survive restarts and wait safely for approval | Checkpoints, deadlines, signals, scoped grants, idempotent effects | Workflow schema, dispatcher, approval ingestion, action adapters |
| [Multi-tenant job platform](#small-multi-tenant-job-platform) | Fair job execution within bounded shared capacity | Tenant authorization, admission, fair queues, leases, fencing, restricted workers | Worker protocol, registration, cancellation, result/log delivery, capacity reclamation |
| [Versioned artifact workspace](#versioned-artifact-workspace) | Publish complete revisions without breaking concurrent readers | Atomic publication, revision conflicts, GC grace, tenant authorization | Upload/manifest APIs, reader pins, retention, partial-failure recovery |
| [Prepared-environment runner](#prepared-environment-runner) | Reuse prepared environments to reduce repeated startup work | Warm pools, reservations, snapshot chunks, restore, placement | Template catalog, host adapter, readiness probes, cache/pool coordination |

### Focused personal agent computer

**Value:** Give one owner a persistent workspace that can run useful code and
perform a narrow set of external actions, while keeping policy and credentials
outside the workspace's control. Start with one synthetic calendar connector,
one permitted action, and one approval type rather than a general assistant.

**Built from:** `focused-runtime-cell`, `peer-authenticated-tool-broker`,
`scoped-approval-consumption`, `surrogate-credential-broker`,
`enforced-egress-path`, `exec-request-dedup`, and `lifecycle-event-audit`.
`data-taint-policy-model` is a later policy experiment, not an initial dependency.
This is the main composition motivated by the [Muse study](../agentic-platforms/meta-muse.md).

**Suggested ideas to work through:**

1. Build `scoped-approval-consumption` with Go/PostgreSQL fixtures, then
   configure the Docker harness for `worker-capabilities` and
   `surrogate-credential-broker`.
2. After configuring the Docker isolation and broker harnesses, work through `focused-runtime-cell`,
   `peer-authenticated-tool-broker`, and `enforced-egress-path`.
3. Add `exec-request-dedup` and `lifecycle-event-audit`, then integrate the
   request and recovery contracts in Other items O01–O05 and O12.

This sequence deliberately starts with eligible models before the blocked runtime
work; global impact rank is not a prerequisite order.

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

**Architecture:** The runtime is the untrusted side; G1 through G4 and the
records they control are outside it. G2 is the decision point. G3 is the only
fixture-service client and supplies the stable operation ID. Workspace access
to G4, the grant database, or a direct network path must be denied by enforcement,
not merely omitted from SDK documentation. A one-use approval permits dispatch;
it does not make the downstream effect atomic with recording completion.

| Component | Role | Coverage |
|---|---|---|
| Untrusted runtime cell | Runs task code and owns its workspace, with no authority over supervisor policy or credentials. | Idea: `focused-runtime-cell`, `worker-capabilities`; Gap: O02 |
| G1 peer broker | Accepts typed action requests and derives caller identity from the connection before checking method permissions. | Idea: `peer-authenticated-tool-broker`; Gap: O01, O02 |
| G2 action gate | Matches a request to its immutable grant and claims dispatch permission. The independent approval API feeds the grant store; that input is omitted from the diagram. | Idea: `scoped-approval-consumption`; Gap: O04 |
| Approval grants and audit | Persists grant scope, claim state and action history outside the runtime; records ambiguous outcomes for recovery. | Idea: `scoped-approval-consumption`, `lifecycle-event-audit`; Gap: O04, O12 |
| G3 connector | Maps an allowed action to one fixture request using a stable operation ID; resolves lost replies without blind redispatch. | Idea: `exec-request-dedup`; Gap: O03 |
| G4 credential store | Redeems scoped handles only for the trusted connector and supplies synthetic test credentials. Rotation, revocation and error handling still need integration. | Idea: `surrogate-credential-broker`; Gap: O05 |
| Fixture service | Pretends to be one external calendar service with observable effects and status lookup; replies are omitted for clarity. | Fixture; Gap: O03, O12 |

**Docker deployment:** Run the untrusted workspace as a non-root, capability-free
container with no network and no Docker socket. Keep supervisor policy, grants,
and credentials in separate trusted services. Share only a dedicated Unix-socket
directory through a Docker named volume; workers cannot replace the broker's
socket directory or change their fixed UIDs. Authenticate peers in Docker's Linux
environment and validate that UID mappings match the broker ACL. Only the trusted
connector has network access to the fixture service; it checks destination and
redirect policy. The host-side Go launcher owns the Docker API.

**Still missing:** Docker cell/broker packaging, a supervisor install/update path,
consistent IDs across all components, and restart tests across grant claim and
external success. The database models are eligible now; the actual Docker
isolation, peer-identity and egress harnesses remain blocked on configuration. Exit criterion: a deliberately noncompliant workspace cannot bypass the
gate, and a lost reply does not duplicate the fixture action. This does not
establish protection against all kernel or prompt-injection attacks.

### Durable approval-based automation service

**Value:** Run a small three-step business workflow that waits for approval,
survives worker restarts, and applies its final action once. Use a synthetic
request-review-publish workflow, not a full workflow language.

**Built from:** `step-checkpoints`, `durable-deadlines`, `signal-before-wait`,
`scoped-approval-consumption`, `activity-ack-ambiguity`, and
`attempt-aware-latency`. The checkpoint implementation is the first backend;
a separate Temporal version can reuse the fixture service for comparison.

**Suggested ideas to work through:**

1. Implement `step-checkpoints` and `durable-deadlines` for one three-step run.
2. Add `signal-before-wait` and `scoped-approval-consumption` so an early
   approval is retained but cannot authorize a different action.
3. Use `activity-ack-ambiguity` as the Temporal comparison and add
   `attempt-aware-latency`. Integrate O01, O03, O04, O06 and O12; do not mix
   two workflow backends into the first implementation.

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

**Architecture:** Run state and step checkpoints are tables in one PostgreSQL
database. The diagram shows a separate write view of that same store to keep
the persistence path clear; it is not a second database. The worker owns execution; the dispatcher owns wakeups. Pending
approvals release workers, and a signal means “recheck authorization,” not
“permission granted.” The effect adapter uses a stable operation ID across
all attempts. End-to-end timing starts at acceptance, not the last retry.

| Component | Role | Coverage |
|---|---|---|
| G1 run intake | Validates a versioned run request, deduplicates submission, and binds it to its owner. | Idea: `step-checkpoints`; Gap: O01, O06 |
| Run state, timers, signals | PostgreSQL records pending work, due times, approvals and completed steps; the store is authoritative when wakeups are missed. | Idea: `step-checkpoints`, `durable-deadlines`, `signal-before-wait`; Gap: O04, O06 |
| G2 wakeup dispatcher | Finds due or signaled work and claims bounded attempts. Rechecks authorization rather than treating a signal as permission. | Idea: `durable-deadlines`, `signal-before-wait`, `scoped-approval-consumption`; Gap: O04, O06 |
| Step worker | Runs the next step, reuses recorded results, and releases capacity while waiting; attempts share one business operation ID. | Idea: `step-checkpoints`, `attempt-aware-latency`; Gap: O06 |
| G3 effect adapter | Executes one external operation and reconciles timeouts; its protocol can be reused in a separate Temporal comparison. | Idea: `activity-ack-ambiguity`; Gap: O03 |
| G4 checkpoint writer | Commits attempt results with guarded state transitions and updates runnable state for the dispatcher. | Idea: `step-checkpoints`; Gap: O06, O12 |
| Run state update | Write-side view of the same PostgreSQL run store; the dispatcher discovers subsequent work from this persisted state. | Idea: `step-checkpoints`; Gap: O06 |
| Fixture service | Provides an idempotent mock action and status query so duplicate attempts can be distinguished from duplicate effects. | Fixture; Gap: O03, O12 |

**Still missing:** One cohesive state schema, worker shutdown/restart behavior,
authenticated approval input, and a failure matrix around every transition.
Go/PostgreSQL are configured; the optional comparison uses configured Temporal.
Exit criterion: restart across every step and approval boundary, then drain to
one final effect per accepted run with no lost early approval.

### Small multi-tenant job platform

**Value:** Accept short computational jobs from two tenants, bound capacity,
and keep a busy tenant from starving the other. Execute fixed Go job binaries
in restricted Docker containers; arbitrary shell jobs are outside the initial scope.

**Built from:** `tenant-resource-authorization`, `atomic-capacity-reservation`,
`tenant-fair-queue`, `lease-reclaim`, `stale-owner-fencing`,
`worker-capabilities`, and `log-stream-backpressure`.

**Suggested ideas to work through:**

1. Start with `tenant-resource-authorization`, `atomic-capacity-reservation`,
   and `tenant-fair-queue` over deterministic jobs.
2. Work through the existing `lease-reclaim` plan and `stale-owner-fencing`;
   connect failure detection to safe result publication.
3. Add `worker-capabilities` and `log-stream-backpressure`, then O01, O07,
   O08 and O12 to run bounded Go container jobs end to end.

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

**Architecture:** PostgreSQL is authoritative for admitted jobs, reservations,
and result generations. Valkey may hold liveness leases as in the lease lesson;
the result store still validates fencing tokens. Queue fairness and capacity
reservation are different decisions. A worker lease expiring does not itself
stop that worker, so duplicate execution attempts must not publish competing
results. Logs have bounded buffering and explicit loss accounting.

| Component | Role | Coverage |
|---|---|---|
| G1 job API | Accepts a validated job on behalf of a trusted tenant and exposes status/cancellation without caller-controlled ownership. | Idea: `tenant-resource-authorization`; Gap: O01, O07 |
| Tenant job queues | Holds accepted jobs grouped for fair selection; starts as PostgreSQL records, not another broker. | Idea: `tenant-fair-queue`; Gap: O07 |
| G2 scheduler | Selects a tenant fairly, reserves capacity and dispatches a uniquely identified attempt; recovers abandoned reservations. | Idea: `tenant-fair-queue`, `atomic-capacity-reservation`; Gap: O07 |
| Capacity and leases | Tracks committed slots and worker liveness. Expiry permits recovery but publication still requires a current fencing token. | Idea: `atomic-capacity-reservation`, `lease-reclaim`, `stale-owner-fencing`; Gap: O07 |
| G3 worker adapter | Translates the control-plane job into a restricted Go worker container, handles cancellation and captures exits and bounded logs. | Idea: `worker-capabilities`, `log-stream-backpressure`; Gap: O01, O07, O08 |
| Restricted Worker | Executes the fixed job function with explicit resource grants; it cannot claim that its result is authoritative. | Idea: `worker-capabilities`; Gap: O07 |
| G4 result writer | Atomically checks ownership/generation before publishing the terminal result; exposes retained results and log references. | Idea: `stale-owner-fencing`, `log-stream-backpressure`; Gap: O08 |

**Still missing:** Go control-plane/worker protocol, Docker worker adapter,
worker registration, fixtures for stale completions, and storage cleanup.
The named services are configured; Docker isolation/API integration is not.
The complete local service runs real restricted Go containers. Multiple physical
hosts and a VM fleet are outside its scope.
Exit criterion: burst both tenants, interrupt one worker, and observe bounded
capacity, progress for both tenants, and one accepted result per job.

### Versioned artifact workspace

**Value:** Let a few jobs publish related files as one consistent workspace
revision while concurrent readers keep seeing a complete revision. Scope it to
an object-backed document bundle; a POSIX filesystem or collaborative editor is
not required.

**Built from:** `artifact-generation-publish`, `workspace-write-conflicts`,
`artifact-gc-grace`, `tenant-resource-authorization`, and `lifecycle-event-audit`.

**Suggested ideas to work through:**

1. Establish `tenant-resource-authorization`, then implement
   `artifact-generation-publish` for one small bundle format.
2. Add `workspace-write-conflicts` and `artifact-gc-grace` to cover concurrent
   writers and cleanup racing with publication.
3. Add `lifecycle-event-audit`; integrate O01, O09 and O12 for manifest APIs,
   reader pinning and recovery tests.

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

**Architecture:** The three lanes show publication, reads and cleanup. Object
nodes are views of one SeaweedFS store; revision and retention nodes are views
of one PostgreSQL database. Upload finishes before manifest commit proceeds.
SeaweedFS holds immutable objects; PostgreSQL holds ownership,
revision manifests, references, and the visible-head pointer. An expected revision
protects concurrent writes. A reader pins one manifest before loading its files.
Garbage collection must consider in-flight publication and active readers, not
only whether an object is absent from the current head.

| Component | Role | Coverage |
|---|---|---|
| G1 publish session | Assigns tenant-scoped upload IDs, validates limits and hashes, then prepares an immutable candidate revision. | Idea: `artifact-generation-publish`, `tenant-resource-authorization`; Gap: O01, O09 |
| Upload objects | Stores immutable uploaded bytes before a manifest makes them visible; unfinished uploads need abandonment cleanup. | Idea: `artifact-generation-publish`; Gap: O09 |
| G2 manifest commit | Checks the expected revision and atomically advances the visible head with object-reference metadata. | Idea: `artifact-generation-publish`, `workspace-write-conflicts`; Gap: O09 |
| Revision write | Stores version manifests, tenant ownership, reader pins and the current head; records are shared by readers and cleanup. | Idea: `workspace-write-conflicts`, `lifecycle-event-audit`; Gap: O09 |
| G3 read resolver | Authenticates the tenant and pins one manifest before resolving its object keys, preventing mixed-version reads. | Idea: `tenant-resource-authorization`, `artifact-generation-publish`; Gap: O09 |
| Revision read | Read-side view of PostgreSQL manifests and reader pins; every object lookup is tied to one retained revision. | Idea: `artifact-generation-publish`; Gap: O09 |
| Read objects | Serves only bytes from the pinned revision. This is the same object store as uploads, shown separately to clarify the read path. | Idea: `artifact-generation-publish`; Gap: O09 |
| G4 collector | Finds eligible unreferenced objects after grace/pin checks, then retries bounded deletions safely. | Idea: `artifact-gc-grace`; Gap: O09, O12 |
| Retention scan | Reads PostgreSQL publication and reader-pin state to decide which object generations may be deleted. | Idea: `artifact-gc-grace`; Gap: O09 |
| Expired objects | Represents deletion candidates in the same object store, not a separate service or bucket requirement. | Idea: `artifact-gc-grace`; Gap: O09 |

**Still missing:** Manifest schema, reader-pin expiry, upload abandonment rules,
object-store error translation, and bounded retention. Go, PostgreSQL and SeaweedFS are configured, but the Go S3 client and
those integration contracts are not implemented.
Exit criterion: concurrent publish/read/cleanup never produces a mixed revision
or deletes a pinned artifact; abandoned uploads eventually become reclaimable.

### Prepared-environment runner

**Value:** Start repeated instances of one fixed toolchain quickly and show
where startup time moves as prepared state and caching increase. Keep one
image family and two modeled hosts before attempting general placement.

**Built from:** `warm-pool-claims`, `atomic-capacity-reservation`,
`lazy-snapshot-chunks`, `parallel-image-restore`, `prepared-container-start`,
`session-generation`, and `sampled-placement`.

**Suggested ideas to work through:**

1. Model `atomic-capacity-reservation`, `warm-pool-claims`, and
   `session-generation` before introducing a real runtime.
2. Build `lazy-snapshot-chunks` and `parallel-image-restore`; add
   `sampled-placement` only after correctness with a fixed host is clear.
3. Configure the Docker Engine API and image-build path, then implement
   `prepared-container-start`. Integrate O01, O10, O11 and O12; retain separate
   model and actual container results. The complete project runs real containers
   locally, without a VM runtime adapter.

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

**Architecture:** The two modeled hosts in the diagram are scheduling fixtures.
The complete project uses one local Docker Engine with two logical capacity
buckets; these are not two physical failure domains. The host adapter launches
real Go containers. Verified chunks materialize a versioned workspace/input
bundle; the prepared OCI image supplies the runtime. This integration does not
require a custom image snapshotter or VM disk driver.

Separate immutable template identity from each writable session. The pool owns capacity; the host adapter owns actual creation and
readiness. Cache hits cannot substitute for checking the template's content
identity. Record both time to ready and time to the first successful command;
lazy input materialization can shift latency between them. Claim/replenishment must respect
per-host limits even when reported load is stale.

| Component | Role | Coverage |
|---|---|---|
| G1 template catalog | Records immutable template hashes, runtime compatibility and preparation status so partial templates cannot be claimed. | Idea: `lazy-snapshot-chunks`, `prepared-container-start`; Gap: O10 |
| G2 pool controller | Claims exact-match inventory within capacity and replenishes a bounded pool, accounting for stale host reports. | Idea: `warm-pool-claims`, `atomic-capacity-reservation`, `sampled-placement`; Gap: O11 |
| G3 host adapter | Implements create/inspect/stop with stable request IDs and session generations; maps backend failures to lifecycle state. | Idea: `session-generation`, `prepared-container-start`; Gap: O01, O10 |
| Two modeled hosts | Provides controlled capacity and startup delay for placement experiments; the complete project maps these logical pools to real local Docker containers. | Idea: `sampled-placement`, `atomic-capacity-reservation`; Gap: O10 |
| Session and command | Represents one writable instance and its readiness/first-command probe; model results and actual container results stay separate. | Idea: `session-generation`, `prepared-container-start`; Gap: O10, O12 |
| G4 chunk cache | Fetches and verifies immutable chunks, bounds storage, and coordinates concurrent reads and eviction. | Idea: `lazy-snapshot-chunks`, `parallel-image-restore`; Gap: O11 |
| SeaweedFS chunks | Stores versioned workspace/input chunks for the data-path experiment; the Docker image supplies the executable runtime. | Idea: `lazy-snapshot-chunks`, `parallel-image-restore`; Gap: O10 |

**Still missing:** Docker runtime adapter, image build/import path, container
readiness probe, cache eviction policy, and cleanup after failed creation.
PostgreSQL and SeaweedFS support the control/data-path model now; Docker API and
BuildKit integrations remain unconfigured. Exit criterion: reconstruct verified
input bytes, launch independently writable containers from one prepared image,
claim warm instances once, execute the probe, and reclaim failed/finished runs.
Report preparation, cold creation and warm claims separately; no VM boot or
memory-restore performance claim is made.
### Other

Side quests to make the projects work end to end. These are **integration gaps,
not additional ranked lessons**: they have stable `Oxx` IDs and no impact Order.
Their scope is tentative until a project is selected. Completing a related idea
proves its small mechanism, not this integration work. If a side quest becomes
its own lesson, add a ranked row to the appropriate aspect and link it here.
Client screens and CLI commands remain fully AI-generated and are not side quests.

| ID | Side quest | Projects | What needs connecting | Done when | Status |
|---|---|---|---|---|---|
| O01 | Shared IDs and service contracts | All | Tenant/task/session/operation/generation identities; typed requests, errors, versioning and trusted caller context | The same operation can be traced across components; spoofed ownership and incompatible requests fail explicitly | candidate |
| O02 | Supervisor and runtime packaging | Focused computer | Service ordering, separate identities, mount/socket/network policy, workspace persistence and restart/update behavior around the cell lesson | A restart restores the intended boundaries and workspace; replacing workspace code cannot change supervisor policy | blocked: Docker isolation and broker harnesses unconfigured |
| O03 | Connector and effect adapters | Focused computer; automation | One typed action to one fixture service, stable operation ID, timeout/status reconciliation and bounded retries | External success followed by a lost reply is reconciled without a second effect | candidate |
| O04 | Approval API and grant lifecycle | Focused computer; automation | Independent authenticated approval input, immutable action binding, durable signals, expiry/revocation and dispatch claims | Duplicate/early approvals survive recovery; changed actions and expired grants cannot dispatch | candidate; client UI out of scope |
| O05 | Credential lifecycle | Focused computer | Handle binding, restricted broker storage access, rotation/revocation, safe error/log handling | A revoked handle stops working and no synthetic credential reaches runtime-visible output | candidate; production storage/auth integration undecided |
| O06 | Workflow engine assembly | Automation | Run/step schema, timer/signal dispatcher, checkpoint writer, cancellation and recovery/version rules | Every injected restart boundary ends in a terminal or explicitly waiting state without lost progress | candidate |
| O07 | Scheduler and worker integration | Job platform | Worker registration, Go protocol, fair selection, reservations, lease issuance/reclaim and cancellation | Lost workers do not leak capacity; stale completions cannot overwrite current results | candidate |
| O08 | Result and log delivery | Job platform | Fenced result commit, bounded log transport, terminal records and retention cleanup | A slow client cannot grow memory indefinitely; job outcome remains queryable after worker exit | candidate |
| O09 | Workspace API and retention | Artifact workspace | Upload sessions, validated manifests, atomic head changes, reader pins, abandonment and batched deletion | Concurrent reading/publication/GC preserves complete pinned revisions and eventually reclaims abandoned data | candidate |
| O10 | Template and host adapter | Prepared runner | OCI build/import manifest, content identity, runtime compatibility, Docker create/inspect/stop contract and container readiness probe | One template can create an independently writable Docker container and clean up a failed start | blocked: Docker Engine API and BuildKit integrations unconfigured |
| O11 | Cache and warm-pool coordination | Prepared runner | Concurrent fetch deduplication, cache limits, pin/evict rules, exact-match pool claims and bounded replenishment | Corrupt/partial templates never become ready; bursts and host loss respect capacity | candidate for model; real runtime depends on O10 |
| O12 | End-to-end failure harness | All | Reproducible fixtures, restart cut points, invariant checks, bounded cleanup and DuckDB reporting across component boundaries | Each project's stated exit criterion is demonstrated under success, delay, duplicate delivery and restart scenarios | candidate |
