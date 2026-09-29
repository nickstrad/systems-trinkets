# E2B: snapshot-restored microVMs

Research checked 2026-09-29. This is a design study, not a deployed E2B benchmark. E2B's current open-source runtime architecture is unusually detailed but may change after this date; the linked repository reflects its state when consulted.

## Request path and isolation

E2B's published runtime accepts SDK REST lifecycle calls through an API. The API authenticates, enforces quotas, chooses a sandbox node, and sends a gRPC create/pause/delete operation to that node's orchestrator. The orchestrator manages one Firecracker **microVM per sandbox**, including network, disk, and cgroups. A guest `envd` service handles process and filesystem calls. User-facing sandbox HTTP traffic goes through a separate client proxy, which looks up the sandbox-to-node route and forwards to the orchestrator; it does not traverse the lifecycle API. A template manager builds templates from container images on build nodes, but the resulting sandbox is a VM, not a container. [Runtime architecture](https://github.com/e2b-dev/runtime/blob/main/docs/ARCHITECTURE.md) · [Local development requirements](https://github.com/e2b-dev/runtime/blob/main/DEV-LOCAL.md)

The same architecture names PostgreSQL for durable teams, templates, builds, and paused-sandbox records; Redis for live sandbox/routing state, caches, rate limits, and peer registry; object storage for template/snapshot artifacts; and ClickHouse for metrics and events. It describes a placement algorithm that samples K ready nodes and chooses the least loaded by CPU or hugepage pressure, with a retry when a node is exhausted. Its architecture document names Kubernetes as the supported distribution while also describing a single-machine Embed stack and legacy Nomad code. These are descriptions of the open repository, not a verified map of every hosted E2B deployment. [Runtime architecture](https://github.com/e2b-dev/runtime/blob/main/docs/ARCHITECTURE.md)

## Why startup and fan-out work

E2B describes a template as a pre-booted VM snapshot containing memory, disk, and VM state in object storage. Creating a sandbox restores that snapshot instead of cold-booting the guest. Its published implementation fetches memory lazily through userfaultfd and writes the root filesystem through a copy-on-write layer, so launch need not copy all memory and disk data. The published artifact set includes `memfile`, `rootfs.ext4`, `snapfile`, metadata, and index headers. Startup claims on E2B's product site are vendor claims; response time depends on cache state, working set, concurrent demand, and where measurement begins and ends. [Runtime architecture](https://github.com/e2b-dev/runtime/blob/main/docs/ARCHITECTURE.md)

E2B's February 2025 engineering article explains why cloning a full root filesystem for thousands of Firecracker instances wastes host disk and how an OverlayFS read-only lower layer with a writable upper layer avoids that copy until files change. That article explains the storage mechanism, not a measured speedup for this repo. [Scaling Firecracker with OverlayFS](https://e2b.dev/resources/scaling-firecracker-using-overlayfs-to-save-disk-space)

On pause, the runtime documents a VM snapshot with dirty memory and filesystem differences from its template, local cache, then asynchronous object-store upload. Resume prefers the origin node so a cached snapshot can avoid an object-store read. It documents a filesystem-only recovery path for a bad memory restore, with crash-recovery semantics. Such details show why pause acknowledgement, route removal, upload durability, and retries are separate concerns. [Runtime architecture](https://github.com/e2b-dev/runtime/blob/main/docs/ARCHITECTURE.md)

The E2B SDK also exposes sandbox timeouts, environment variables, internet-access controls, connection to a running sandbox, metrics, and terminal/log operations. Current exact limits and feature gates are plan/version dependent; the architecture and SDK references should be rechecked when authoring a production-facing guide. [Python SDK reference](https://e2b.dev/docs/sdk-reference/python-sdk/v2.5.0/sandbox_sync) · [Sandbox CLI reference](https://e2b.dev/docs/sdk-reference/cli/v2.0.3/sandbox)

## Bounded lesson candidates using configured software

The S3 exercises require the catalog's unconfigured Go S3 client before they
can become working guides. Service availability alone does not satisfy that
client integration prerequisite.


1. **Lazy snapshot fetch versus eager copy, as a trace-driven model.** Use Go to generate a fixed set of page/chunk reads, SeaweedFS S3 (`http://localhost:8333`, access key `trinkets`, secret key `trinkets-secret`, region `us-east-1`, path-style) for immutable chunk objects, Valkey (`redis://localhost:6379`) for a bounded local-cache index, and DuckDB for analysis. Baseline: fetch every chunk before a request is ready. Variant: fetch only chunks on demand, with a bounded cache. Invariant: both variants return byte-identical reads and never serve an unverified chunk. Measure time to first successful read, total fetched bytes, cache hits, and p95 read latency over cold/warm working sets. This models demand paging economics; it is **not** a userfaultfd or Firecracker implementation. [Runtime architecture](https://github.com/e2b-dev/runtime/blob/main/docs/ARCHITECTURE.md)

2. **Pause commit across live routing and durable snapshot state.** Use PostgreSQL (`postgres://trinkets:trinkets@localhost:5432/trinkets`) for sandbox/snapshot metadata, a second PostgreSQL table for live routes, and SeaweedFS S3 for a mock snapshot artifact. Inject a bounded upload delay in the runner. A separate follow-up can move the routes into Valkey. Baseline: remove the route as soon as pause is requested and report success before upload. Variant: explicit state transitions and retry/reconcile around the artifact and route changes. Invariant: an acknowledged paused sandbox has a recoverable artifact, and a live route never points at a destroyed instance. Measure false-success count, pause acknowledgement latency, recovery time, and orphan artifacts. The exact transaction order is a lesson design choice, not a claim about E2B code. [Runtime architecture](https://github.com/e2b-dev/runtime/blob/main/docs/ARCHITECTURE.md)

3. **Placement under bursts and stale load reports.** Model E2B's documented best-of-K choice in Go using configured etcd (`http://localhost:2379`) for node liveness and leases, PostgreSQL for allocations, k6 for create bursts, and DuckDB for analysis. Baseline: random ready-node choice. Variant: sample K nodes and choose the lowest max(CPU load, hugepage-pool load) score, retrying on capacity rejection. Invariant: no node exceeds declared capacity and one sandbox gets at most one allocation. Measure rejection/retry rate, load skew, create latency, and stale-placement errors while a node drops out. This compares placement policies without claiming E2B's production K or fleet topology. [Runtime architecture](https://github.com/e2b-dev/runtime/blob/main/docs/ARCHITECTURE.md)

## Local scope boundary

Actual E2B VM boot and memory-snapshot restoration are outside this repo's
Docker + Go baseline: Firecracker and all KVM-dependent paths are avoided
for now. The local `container-namespace-boundary` and `prepared-container-start`
ideas teach container visibility and prepared startup instead. Chunk experiments
teach storage behavior; they do not reproduce userfaultfd or E2B isolation.
Docker API, image preparation and a real container daemon still require their
own configured integrations. See [software catalog](../../software/software.md).
