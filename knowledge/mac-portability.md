# Mac lesson portability

## Current decision (2026-09-29)

The user requires every idea and full project to run locally with Docker
Desktop/Compose and Go. Existing DuckDB/k6 tooling remains in scope.
Firecracker, all KVM-based tools and nested virtualization are avoided for now.
Do not reopen the earlier Lima/nested-virtualization suggestion; hardware
eligibility is irrelevant to the chosen scope. No Apple container, alternate VM runtime or separately managed Linux VM is
needed by the project designs. Containerd/nerdctl is allowed for a specific
lesson benefit after local engine integration is configured; services use Docker.

This supersedes the earlier seven-idea portability review. The 42 ideas and
five compositions in `docs/lessons/ideas.md` now use this design boundary.
Lesson/platform code is Go only; Deno/TypeScript remains only for k6 tooling.
The former non-Go lesson and helpers were deleted at the user's request.

The affected designs are:

- `guest-kernel-boundary` was replaced by `container-namespace-boundary`:
  shared/private PID namespaces and mounts on a shared Docker Linux kernel.
- `microvm-snapshot-restore` was replaced by `prepared-container-start`:
  cold initialization, prepared OCI image and one-use warm claims. This does
  not restore live memory. Project references use the replacement slugs.
- The focused computer uses a non-root restricted Docker worker, external
  supervisor, fixed-UID peer authentication and a network-disabled worker
  requesting narrow actions over a protected Unix socket. Socket storage stays
  in a Docker named volume; the worker has neither Docker socket access nor
  UID-changing capabilities. Validate UID mapping and directory permissions.
- Resource-cap and process-control probes run inside Docker. The cancellation
  fixture keeps descendants in their process group and uses parent-owned waits.
- The prepared runner ends with real local containers, independently writable
  state, verified input chunks and bounded warm pools. Two modeled hosts become
  logical capacity buckets on one engine, not physical failure domains.

`software/software.md` distinguishes excluded research tools from unconfigured
but eligible Docker integrations. Docker API, isolation, Unix peer-identity,
egress broker, BuildKit, Go S3 client and daemon integrations still need implementation and
runtime verification. No readiness flag was promoted by this change.
Provider research retains factual architecture descriptions without making
those technologies dependencies. Root and scoped AGENTS.md carry the policy.

Validation: documentation ranks/slugs, project references and relative links
checked; no new harness was implemented or tested on a Mac. Linux requirements
are distinct from KVM requirements: Docker supplies the Linux kernel for the
container experiments. Container isolation is not separate-kernel isolation.

References retained from the earlier primary-source review:
- [Docker Desktop networking](https://docs.docker.com/desktop/features/networking/)
- [Docker resource limits](https://docs.docker.com/engine/containers/resource_constraints/)
- [Docker internal networks](https://docs.docker.com/reference/cli/docker/network/create/)
  allow gateway/host communication; this motivates the stricter network-disabled
  worker and narrow Unix-socket broker proposal, whose full harness is untested.

Implementation cleanup verification: `make check`, all five `k6 inspect`
commands and a scratch Go-adapter/k6/DuckDB smoke passed on Linux. See
[performance-labs.md](performance-labs.md) for evidence and limits. The current
lease guide was already Go and its executable code was left unchanged.
