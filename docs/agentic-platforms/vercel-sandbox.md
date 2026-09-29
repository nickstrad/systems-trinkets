# Vercel Sandbox: isolation and fast filesystem restore

Research snapshot: 2026-09-29. This is a source-grounded design note for future systems lessons, not a description of an implementation in this repo.

## Request path and isolation

Vercel says Sandbox reuses **Hive**, its regional compute platform for untrusted builds. In the published Hive design, a caller selects a hive and asks its API to run a cell; the control plane places work and manages autoscaling and health. A box is a bare-metal host; its daemon provisions a block device and starts one Firecracker process per cell, then talks to a daemon in the guest over a dedicated socket. The guest daemon controls workload containers. Vercel explicitly says each Sandbox workload is an isolated container **inside** a Firecracker microVM. The exact public Sandbox API gateway, placement policy, guest protocol, and tenant-to-cell mapping are not published; applying the 2024 build request path to Sandbox is an inference because the 2026 snapshot post identifies Hive as shared infrastructure. [Hive engineering post, 2024](https://vercel.com/blog/a-deep-dive-into-hive-vercels-builds-infrastructure); [Sandbox snapshot engineering post, 2026](https://vercel.com/blog/optimizing-vercel-sandbox-snapshots).

```text
Sandbox SDK / CLI -> Vercel service -> regional Hive API -> control plane -> box daemon
                                                                  | block device + Firecracker
                                                                  v
                                                       cell daemon -> workload container
```

The dedicated guest kernel supplies the VM boundary; the container supplies the familiar Linux process and filesystem environment. Vercel documents a separate filesystem and network for each sandbox. Its docs expose command execution, file operations, snapshots, and live previews. Treat claims such as “milliseconds” or “sub-second” startup as vendor performance claims for particular paths, not a universal cold-boot SLA. In the 2024 **build** account, a prewarmed cell made work immediate, whereas a newly provisioned cell took about five seconds; those build figures do not measure current Sandbox creation. [Sandbox docs](https://vercel.com/docs/sandbox); [Sandbox GA announcement, 2026](https://vercel.com/blog/vercel-sandbox-is-now-generally-available); [Hive engineering post, 2024](https://vercel.com/blog/a-deep-dive-into-hive-vercels-builds-infrastructure).

## Storage, restore, and scaling

Vercel's explicit snapshot is a compressed copy of a sandbox disk image: raw `.img` becomes its private `.vhs` format in S3. Restore downloads and decompresses that object before boot. A named persistent sandbox separates durable identity and filesystem state from a running **session**: stop snapshots the filesystem, and resume creates a new session from it. The snapshot preserves files and installed packages; it is not described as preserving a live process or RAM image. Session timeout and persistence are independent controls. [Snapshot engineering post](https://vercel.com/blog/optimizing-vercel-sandbox-snapshots); [duration and persistence guide, updated 2026-06-29](https://vercel.com/kb/guide/vercel-sandbox-duration-and-persistence).

The engineering post identifies three restore costs: serial S3 download, serial decompression, and an intermediate disk write. Vercel added parallel HTTP Range downloads, parallel decompression of independently framed `.vhs` regions, and streaming from download to decoder. A host-local NVMe cache stores the **decompressed image**, with size-based LRU eviction, so a hit skips both transfer and decoding. Vercel reports a 95% hit rate for its workload and p75 restore improving from over 40 seconds to under one second, p95 from 50 to 5 seconds. These are Vercel's observed, workload-dependent figures; the post does not give a reproducible public benchmark or disclose the frame format. It also calls cache-affinity scheduling a future possibility, with hotspot risk. [Snapshot engineering post, 2026](https://vercel.com/blog/optimizing-vercel-sandbox-snapshots).

Hive's 2024 build post says each cell has dedicated CPU and memory, while disk and network throughput are rate-limited against box capacity; each hive is a regional failure boundary. The control plane owns placement and autoscaling, and a pool of prewarmed build cells absorbs arrivals. The 2026 Sandbox GA post says Hive operates Firecracker clusters across regions, but neither post specifies Sandbox capacity planning, fairness, oversubscription, or the scheduler's algorithm. [Hive engineering post](https://vercel.com/blog/a-deep-dive-into-hive-vercels-builds-infrastructure); [Sandbox GA announcement](https://vercel.com/blog/vercel-sandbox-is-now-generally-available).

## Bounded lesson candidates using configured software

The S3 exercises require the catalog's unconfigured Go S3 client before they
can become working guides. Service availability alone does not satisfy that
client integration prerequisite.


1. **Snapshot restore pipeline.** With Go, SeaweedFS S3, local files, and DuckDB (services configured; Go S3 client still unconfigured in `software/software.md`), create reproducible disk-like fixtures, compress them into independently decodable frames, upload them, then compare serial whole-object download/decode with parallel Range download/decode and a bounded local LRU of decoded images. Invariant: SHA-256 of every restored image equals the original, including after an interrupted transfer and retry. Record fixture size, compressed bytes, chunk size, concurrency, cache hit/miss, network bytes, restore p50/p95, and CPU time; repeat hot/cold runs separately. First verify SeaweedFS's Range behavior against the fixture. This models the **data path**, not a Firecracker boot. [Vercel's three restore optimizations](https://vercel.com/blog/optimizing-vercel-sandbox-snapshots).
2. **Durable identity versus running session.** With PostgreSQL and SeaweedFS, model `sandbox_id`, `session_id`, expiration, and a snapshot pointer. Compare a baseline that discards state at stop with stop/upload/resume. Invariant: a resume observes the last committed filesystem generation exactly once; stale sessions cannot overwrite a newer generation. Inject termination between object upload and metadata commit, and between metadata commit and stop; measure resume latency, stranded objects, stale-writer attempts, and active-session time. This is a control-plane simulation, with no VM isolation. [Persistence lifecycle](https://vercel.com/kb/guide/vercel-sandbox-duration-and-persistence).

## Future gaps and cautions

Actual per-sandbox Firecracker/KVM boot and VM-memory restore are outside the local baseline and must not become lesson or project dependencies. Container preparation, a Go daemon, and Docker-managed resource/network boundaries provide the local learning path; their integrations remain unconfigured. The repo's configured Docker Compose can support service experiments, but the Docker Engine API is presently `no`. The `.vhs` format, scheduling, and production security policy are proprietary or undocumented, so lessons should test their own stand-in mechanisms and avoid claiming parity. Source performance numbers are dated vendor observations, not targets for the local machine. [Hive engineering post](https://vercel.com/blog/a-deep-dive-into-hive-vercels-builds-infrastructure); [snapshot engineering post](https://vercel.com/blog/optimizing-vercel-sandbox-snapshots).
