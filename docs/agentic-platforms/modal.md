# Modal: elastic compute and sandbox isolation

**Research date: 2026-09-29.** Modal's SDK docs define the user-facing contract; its engineering posts reveal selected implementation details. Those posts are company-authored snapshots, not a complete or independently verified description of the current fleet.

## Request and execution path

A Modal app declares Functions and Sandboxes. Calling a Function with an input causes Modal to make a container available for that function; concurrent inputs can cause additional containers to start. Containers can be reused for later inputs and eventually scaled to zero. Each Function has an autoscaling pool, with controls for maximum and minimum containers, a buffer, and the scaledown window ([scaling](https://modal.com/docs/guide/scale), [Functions](https://modal.com/docs/guide/functions)). These are user-visible controls; the docs do not expose the scheduler algorithm or placement policy.

Modal's Sandbox API creates an isolated environment from an app and image, with operations for stdin/stdout/stderr, filesystem, and termination ([Sandbox SDK reference](https://modal.com/docs/sdk/js/latest/Sandbox)). Sandboxes are built on gVisor according to Modal's security docs; they also state that Sandboxes do not have the workspace-resource authorization that Functions receive by default ([networking and security](https://modal.com/docs/guide/sandbox-networking)). Treat this as the vendor's security description, not an independent proof of isolation.

Modal also documents a beta VM runtime for Sandboxes. It gives the Sandbox a full Linux kernel and enables workloads such as nested Docker, custom init systems, eBPF, FUSE, and cgroup resource isolation that are unavailable or constrained under gVisor ([VM Sandboxes](https://modal.com/docs/guide/vm-sandboxes)). The distinction is valuable: the default sandbox runtime and a full VM have different compatibility and isolation tradeoffs.

## Engineering writeups

Modal's [container launch writeup](https://modal.com/blog/speeding-up-container-launches) describes avoiding a conventional Docker launch path, keeping image files on network storage, and using gVisor with a content-addressed local cache backed by FUSE. The engineering point is that cold-start time includes image transfer and filesystem access, not only runtime boot. The post is a vendor account of its then-current implementation, not a guarantee that every detail still matches the current platform.

The [2026 sandbox scaling post](https://modal.com/blog/scaling-to-1-million-concurrent-sandboxes-in-seconds) says their newer creation path uses horizontally scaled scheduling servers, a fast in-memory worker-selection algorithm, and direct scheduler-to-worker creation; sandbox records are stored in Redis outside the critical path. Modal says the creation path is two network hops and one cheap CPU operation. This suggests a useful systems lesson question: which coordination and durable-state work must sit on the latency-critical create path? These implementation claims are self-reported and do not expose code or independent benchmarks.

Modal's [web infrastructure writeup](https://modal.com/blog/serverless-http) explains translating HTTP requests into function calls, with separate ingress infrastructure and a custom gVisor-based compute runtime. It notes that the ingress proxy and user function compute are separate planes. Together, these writeups add architecture detail beyond the public SDK contract; they still leave the full scheduler, isolation implementation, and current topology unpublished.

## Scaling, limits, and unknowns

- By default, Function pools can scale to zero. Warm minimums and buffers reduce cold-start waiting at added cost; maximum containers cap scale-up ([scaling](https://modal.com/docs/guide/scale)).
- `Function.map()` fans independent inputs across parallel invocations. The docs say results are presented in input order; a single thrown exception propagates unless configured to return exceptions ([scaling](https://modal.com/docs/guide/scale)).
- Sandboxes expose CPU and memory resource controls; the docs distinguish requested resources from hard CPU limits and list resource constraints ([resource configuration](https://modal.com/docs/guide/resources)).
- The engineering posts disclose selected routing, image distribution, and placement mechanisms; the full current autoscaler policy, fleet topology, and tenant-boundary implementation remain unspecified in these sources.

## Lesson fit for systems-trinkets

The Modal SDK, gVisor, and Modal Sandboxes are **not configured** in [`software/software.md`](../../software/software.md). Go and PostgreSQL are configured, so a local lesson can use only those two pieces to teach a small part of the lifecycle without pretending to recreate Modal. Store numbered jobs and their queued/running/done state in PostgreSQL; have a Go worker process claim and execute them. Compare one worker with a small fixed worker pool, and compare a worker kept alive between jobs with one restarted for each batch. Invariant: every accepted job reaches one terminal state and one job ID produces at most one stored result. Measure queue wait, worker initialization time, throughput, and completion latency. This experiment isolates warm capacity and concurrency; it does not implement container isolation or autoscaling.

Container lifecycle and isolation should remain a separate future lesson. Docker Engine API and cgroups v2 are catalogued as **not configured**; Firecracker/KVM and alternate runtimes are outside the local baseline; no container creation or sandbox claims belong in the runnable Go/PostgreSQL experiment. Startup measurements can compare warm and restarted Go worker processes, but they say nothing about gVisor or VM isolation strength.

## Evidence versus inference

**Documented:** per-Function autoscaling pools, scale-to-zero, configurable warm/capacity controls, parallel mapping, Sandbox interfaces, the stated gVisor security model, and an optional beta VM runtime, with links above.

**Inference:** a warm pool trades idle cost for lower input latency; sandbox lifecycle experiments map to an agent-compute control plane. Modal does not publish enough implementation detail here to infer its autoscaler policy or service internals.
