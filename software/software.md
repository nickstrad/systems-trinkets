# Software

One table of every piece of software worth building a lesson on, with what it
is for and whether this repo is set up to use it yet. The north star is the
skill set of someone building a platform like Daytona, E2B, Modal, or Vercel
Sandbox, or the agent compute behind OpenAI and Anthropic products: many
isolated environments, started fast, fed files and secrets, driven by an agent,
observed, metered, and torn down.

## Supported local baseline

Every idea and full project must use Docker Desktop/Compose with Go;
Deno/TypeScript is reserved for k6 tooling; lesson and platform code is Go.
DuckDB and k6 remain the analysis/load tools. Services and Linux-specific probes
run in Docker, using ARM64-compatible images on Apple silicon. Do not require
host Linux administration, a separate Linux VM, or an alternate VM runtime.
Containerd/nerdctl is allowed when an idea benefits from OCI internals; its Linux
engine must be locally accessible from the Mac and configured explicitly. A
macOS client alone does not provide a Linux container engine. Services use Docker.

**Avoid for now:** Firecracker and every KVM-dependent tool, including nested
virtualization workarounds. Apple container, Lima, other VM runtimes and native
host sandbox frameworks are outside the current project paths too. Retained
catalog entries describe research only, not future setup commitments. A row
marked `no` remains unconfigured even when its design fits the baseline.

Columns:

- **Layer**: where it sits in such a platform.
- **Use case**: what a lesson would learn from it.
- **Configured**: `yes` means this repo already runs or imports it (a compose
  file here, a helper in `internal/lab`, or an installed
  tool the Makefile uses). `no` means nothing is set up yet.
- **Setup**: how it runs or would run here. `compose` means a new
  `<name>.compose.yaml` in this folder plus a `compose_file_<name>` line and the
  name in `SERVICES` in the root `Makefile` is all it needs. `library` means
  `go get` for lesson code, nothing to start. `Linux host` names a kernel dependency, not an automatic KVM requirement;
  it is outside the current baseline unless a Docker-contained harness is
  explicitly specified. `reference` means conceptual background, not a project
  dependency. `avoid for now` prohibits adoption under the current runtime policy.

Machine at the time of writing: macOS 27, Apple M5 Max (arm64), Docker Engine
29 via Docker Desktop, Lima 2.2, Apple `container` 1.4.1 (installed, kept
stopped).

| Software | Layer | Use case | Configured | Setup |
|---|---|---|---|---|
| Go | Runtime | Lessons, control-plane code, in-sandbox daemons; every host-side component below has a Go client | yes | brew; one root `go.mod`, helpers in `internal/lab` |
| Deno (TypeScript) | Load tooling | Run the k6 lifecycle script and type-check TypeScript workloads; not a lesson/platform runtime | yes | brew; root `deno.json` and `deno.lock`, `scripts/perf/`, Go lessons' `perf/k6.ts` |
| Docker Compose | Runtime | Starts each backing service as its own project: `make up-<service>`, `down-`, `clean-`, `logs-`, `ps` | yes | Docker Desktop; compose files in this folder |
| k6 | Load | Concurrent traffic for each lesson's HTTP follow-up | yes | brew; `scripts/perf/run.ts` |
| DuckDB | Analysis | Reads `measurements.csv` in every `analyze.sql` | yes | brew CLI |
| PostgreSQL 18 | Storage | Control-plane state: sandboxes, owners, leases, jobs, usage ledger | yes | `postgres.compose.yaml`; `postgres://trinkets:trinkets@localhost:5432/trinkets` |
| Valkey 9 | Storage | Cache, queues, pub/sub, leases, rate limits, idle timers | yes | `valkey.compose.yaml`; `redis://localhost:6379` (no auth) |
| Redis 8 | Storage | Same jobs as Valkey, plus JSON documents, secondary indexes and search, vector sets, time series, Bloom filters; the default when the choice does not matter (see Choosing Redis or Valkey) | yes | `redis.compose.yaml`; `redis://localhost:6380` (no auth); the `redis:8` image bundles the modules below |
| Redis modules (JSON, Query Engine, vector sets, time series, Bloom) | Storage | Agent memory and retrieval, indexed queries over sandbox metadata, per-sandbox usage series, cheap membership tests | yes | built into the running Redis; `MODULE LIST` shows `ReJSON`, `search`, `vectorset`, `timeseries`, `bf`; Valkey 9 has none of them |
| SeaweedFS | Storage | S3 objects: artifacts, snapshots, uploads | yes | `seaweedfs/compose.yaml`; `http://localhost:8333`, access key `trinkets`, secret key `trinkets-secret`, region `us-east-1`, path-style |
| Go S3 client | Storage | Signed S3 operations for Go artifact/chunk lessons | no | Configure a Go S3 SDK and path-style SeaweedFS client in the root module; service availability does not supply a client helper |
| PostgreSQL `LISTEN`/`NOTIFY` | Storage | Wake workers without polling | yes | built into the running PostgreSQL |
| PostgreSQL logical replication | Storage | Stream state changes to a cache or index | yes | `wal_level=logical` is set in `postgres.compose.yaml`; create a publication and replication slot from the lesson |
| Valkey Streams | Storage | Consumer groups, pending entries, claim-on-crash; compare with JetStream | yes | built into the running Valkey |
| Valkey keyspace notifications | Storage | Key expiry as a reaper trigger | yes | `--notify-keyspace-events Ex` is set in `valkey.compose.yaml`; subscribe to `__keyevent@0__:expired` |
| Redis Streams and keyspace notifications | Storage | Same as the Valkey rows above, on Redis | yes | built into the running Redis; `--notify-keyspace-events Ex` is set in `redis.compose.yaml`; subscribe to `__keyevent@0__:expired` |
| pgvector | Storage | Agent memory and retrieval | no | swap the image to `pgvector/pgvector:pg18` in `postgres.compose.yaml` |
| PgBouncer | Storage | Thousands of runners against one PostgreSQL; transaction versus session pooling | yes | `pgbouncer.compose.yaml`, start postgres first; `postgres://trinkets:trinkets@localhost:6432/trinkets`, admin console `postgres://trinkets:trinkets@localhost:6432/pgbouncer`; knobs `PGBOUNCER_POOL_MODE` (transaction) and `PGBOUNCER_POOL_SIZE` (5) |
| ClickHouse | Storage | Usage and event analytics at scale | no | compose; reference, DuckDB covers the lesson value |
| Docker Engine API | Isolation | Honest sandbox-host stand-in: create, exec, stop, kill, cgroup limits, OOM, stats, idle reaping | no | Go client through the active Docker endpoint/context; host-side launcher only, never mount the Docker socket in an untrusted worker |
| containerd + runc + nerdctl | Isolation | OCI image/snapshot/task lifecycle below Docker; optional when an idea benefits from the lower-level API | no | Go client or nerdctl with a configured local Linux engine accessible from macOS; no KVM or separate VM requirement; Docker remains the service default |
| cgroups v2 | Isolation | CPU throttling, memory limits, per-sandbox usage numbers | no | read `/sys/fs/cgroup` from Go inside a container |
| Apple `container` | Isolation | One Linux VM per container on this Mac: boot latency, per-VM disks | no | outside baseline; installed but not a lesson/project dependency |
| Lima | Isolation | A Linux guest on this Mac when a lesson needs one | no | outside baseline; no separate Linux VM or nested virtualization path |
| Firecracker | Isolation | MicroVM boot, snapshot and restore, memory lazy-loading, vsock (E2B, Vercel Sandbox) | no | avoid for now: requires KVM; no setup or project integration |
| Cloud Hypervisor | Isolation | Kata's VMM; CPU and memory hotplug | no | avoid for now: KVM-based runtime; research only |
| Kata Containers | Isolation | VM isolation with a container UX | no | avoid for now: VM runtime integration outside baseline |
| gVisor (`runsc`) | Isolation | User-space kernel (Modal); syscall cost versus VM boot cost | no | reference only; alternate runtime outside baseline |
| libkrun, krunvm | Isolation | MicroVM on Hypervisor.framework; Podman machine's macOS provider | no | outside baseline; no alternate VM runtime |
| bubblewrap, nsjail, Landlock, seccomp | Isolation | Process confinement as used by Claude Code and Codex CLI on Linux | no | reference for standalone sandboxes; project seccomp/capability policy uses the Docker isolation harness |
| `sandbox-exec` (Seatbelt) | Isolation | macOS process confinement; the same story on this Mac | no | outside baseline; use Docker process boundaries |
| Docker isolation harness | Isolation | Private PID namespaces, explicit mounts, non-root workers, dropped capabilities, no-new-privileges, seccomp and network-disabled execution | no | Go launcher using Docker Engine API; harmless fixtures on Docker Desktop; no host namespace or KVM setup |
| Docker Unix peer-identity harness | Identity | Broker UID/GID authentication and method ACLs with separate fixed-UID workers | no | Go inside Linux containers; dedicated Unix sockets on Docker named volumes, protected socket directory, no UID-changing capabilities; validate UID mapping |
| Unix-socket egress broker | Networking | Mediate narrow fixture actions when a worker has no direct network | no | Go broker plus Docker network_mode: none worker; broker alone has fixture network access; explicit destination/redirect policy |
| wazero | Isolation | In-process WebAssembly with fuel metering, memory limits, cancellation | no | library, `github.com/tetratelabs/wazero` |
| wasmtime, WASI | Isolation | Same lesson from the CLI; WASI filesystem capabilities | no | brew; reference |
| CRIU | Isolation | Checkpoint and restore a process tree | no | reference only; no process-memory checkpoint/restore dependency |
| OCI registry (`registry:2`) | Images | Push and pull, layer dedup, pull latency versus image size, garbage collection | yes | `registry.compose.yaml`; `localhost:5050` (no auth), `docker push localhost:5050/<repo>:<tag>`, HTTP API under `/v2/`; Go `github.com/google/go-containerregistry` |
| BuildKit | Images | Dockerfile builds, cache mounts, layer cache hits (Daytona snapshots) | no | already inside Docker Desktop as `docker buildx`; Go client optional |
| eStargz, SOCI, Nydus | Images | Start a container before the image finishes downloading | no | Linux host; reference |
| overlayfs | Filesystem | Copy-up cost on the top layer; `--rm` cleanup | no | observe inside any container |
| btrfs, ZFS snapshots | Filesystem | Instant clone of a warm sandbox disk | no | Linux host; reference |
| NBD, ublk | Filesystem | A root disk served to a microVM from a host process | no | Linux host; reference |
| virtiofs, 9p | Filesystem | Mount a workspace into a VM without copying it | no | reference only; use Docker mounts and named volumes for projects |
| JuiceFS | Filesystem | POSIX workspaces over S3 with metadata in Valkey or PostgreSQL; small-file cost | no | brew CLI plus macFUSE, on the existing S3 and Valkey |
| rsync, Mutagen | Filesystem | Local-to-sandbox sync, conflicts, bandwidth | no | brew |
| fsnotify | Filesystem | Watch-this-directory events; coalescing and overflow | no | Go library |
| Sandbox daemon (like E2B `envd`, Daytona toolbox) | Daemon | The binary inside every sandbox: exec, PTY, files, ports | no | write it in Go as a lesson core |
| vsock | Daemon | Host-to-guest socket without networking | no | reference only; project control uses Go Unix sockets in Docker or HTTP |
| gRPC, Connect RPC, protobuf | Daemon | Typed, streaming RPC between orchestrator, runner, and daemon | no | library; `buf` via brew |
| WebSocket, SSE | Daemon | Terminal, logs, and events to a browser; backpressure | no | library (Go) |
| `creack/pty`, `gliderlabs/ssh` | Daemon | PTY allocation and an embeddable SSH server; resize, exit codes, signals | no | library |
| NATS with JetStream | Control plane | Ack, ack-wait, max-deliver redelivery, dedup by message id, KV compare-and-swap, watches, object store | yes | `nats.compose.yaml`; `nats://localhost:4222` (no auth), monitoring `http://localhost:8222`; Go `github.com/nats-io/nats.go` |
| Temporal | Control plane | Durable execution: retries, timers, signals, deterministic replay | yes | `temporal.compose.yaml` (self-contained dev server); gRPC `localhost:7233`, namespace `default`, no auth, UI `http://localhost:8233`; Go `go.temporal.io/sdk` |
| DBOS Transact | Control plane | Durable execution as a library on PostgreSQL | no | library on the existing postgres |
| Restate | Control plane | Durable RPC and virtual objects | no | compose; reference |
| Inngest dev server, Hatchet | Control plane | Event-driven steps, fan-out, concurrency keys | no | compose; reference |
| River, pgmq, Graphile Worker | Control plane | PostgreSQL job queues to compare with the repo's own | no | library or extension on the existing postgres; reference |
| etcd | Control plane | Leases, watches, elections: placement, heartbeat lapse, fencing stale writers | yes | `etcd.compose.yaml`; `http://localhost:2379` (no auth); Go `go.etcd.io/etcd/client/v3` |
| Kubernetes (kind, k3d, k3s) | Control plane | Pods per sandbox, `runtimeClass`, node pressure | no | brew; heavy; reference |
| Nomad | Control plane | Bin packing and allocation lifecycle | no | brew; reference |
| Kafka, Redpanda | Control plane | Partitioned logs and consumer groups | no | compose; reference, NATS covers the lesson value |
| Go `httputil.ReverseProxy` | Networking | Per-sandbox preview URLs, host routing, WebSocket upgrade, timeouts, retries | no | Go standard library |
| Caddy, Traefik, Envoy | Networking | Wildcard TLS, dynamic routes, connection draining, per-route limits | no | compose or brew; Caddy first |
| Linux netns, veth, tap, nftables | Networking | A network per sandbox, NAT egress, deny-by-default policy | no | reference for low-level networking; projects use Docker network configuration and a Go Unix-socket broker |
| eBPF (Cilium, `bpftrace`) | Networking | Egress allow lists, syscall tracing | no | Linux host; reference |
| WireGuard, Tailscale, Headscale | Networking | Connect sandboxes to a private network or a laptop | no | library (`wireguard-go`, `tsnet`); reference |
| CoreDNS, dnsmasq | Networking | Internal names and wildcard preview domains | no | compose; reference |
| certmagic, step-ca | Networking | Automatic TLS and mutual TLS between control plane and runners | no | library or compose; reference |
| OpenBao (Vault-compatible) | Identity | Leased secrets that die with the sandbox; dynamic database credentials | yes | `openbao.compose.yaml` (dev mode, in memory); `http://localhost:8200`, root token `trinkets`; HTTP API with header `X-Vault-Token`, Go `github.com/openbao/openbao/api/v2` |
| JWT, OIDC (Zitadel, Keycloak, Ory, Dex) | Identity | Users, orgs, API keys, service accounts | no | JWT as a library first; Dex or Zitadel compose later |
| SPIFFE, SPIRE | Identity | Runner and daemon identity without shared secrets | no | compose; heavy; reference |
| OPA, Cedar | Identity | Who may exec where; egress policy as data | no | library; reference |
| SOPS, age | Identity | Secrets at rest in a repo or bucket | no | brew; reference |
| OpenTelemetry SDK | Observability | A span per sandbox operation, exported to a file and read by DuckDB | no | library |
| OTel Collector, Jaeger, Tempo | Observability | A request crossing orchestrator, runner, and daemon | no | compose; reference |
| Prometheus, VictoriaMetrics, Grafana | Observability | Cgroup and daemon metrics over time | no | compose; reference, DuckDB covers analysis |
| Loki, Vector | Observability | Sandbox stdout at volume | no | compose; reference |
| `pprof` | Observability | Where the runner spends time under load | no | Go standard library |
| Toxiproxy | Fault injection | Latency, partitions, and resets between a lesson and its service | yes | `toxiproxy.compose.yaml`; API `http://localhost:8474`, proxy ports `127.0.0.1:22000-22009`; proxies listen on `0.0.0.0:<port>` with upstream `host.docker.internal:<port>`; Go `github.com/Shopify/toxiproxy/v2/client` |
| `tc netem`, Pumba | Fault injection | Packet loss inside a VM; kill and pause containers | no | Linux host or Docker; reference |
| testcontainers-go | Testing | Ephemeral service containers for core tests | no | library; reference |
| Anthropic SDK, Claude Agent SDK | AI | An agent that drives a sandbox through tools; streaming, cancellation, token budgets | no | library (Go SDK); `ANTHROPIC_API_KEY` |
| Model Context Protocol (MCP) | AI | Expose a sandbox as tools: exec, read, write, ports | no | library (Go SDK) |
| Ollama, llama.cpp | AI | Local models for experiments without API spend | no | brew or compose |
| Jupyter kernel protocol | AI | Stateful code execution that survives across calls (E2B code interpreter) | no | inside a container; reference |
| Pyodide, WebContainers | AI | Browser-side isolation and its limits | no | reference |
| Language server (`gopls`) | AI | Editor intelligence served from inside the sandbox; process lifetime and memory | no | brew; reference |
| devcontainer spec and CLI | AI | Build a sandbox from `devcontainer.json` (Daytona's origin) | no | npm CLI |
| Nix, devbox | AI | Reproducible toolchains and store dedup | no | reference |
| Git, Forgejo | AI | Clone into a sandbox, push results, webhooks that start jobs | no | git is installed; Forgejo via compose when needed |
| Playwright, headless Chromium | AI | Computer-use agents inside a sandbox; memory and startup cost | no | inside a container; reference |
| `ttyd`, xterm.js | AI | The browser end of a PTY stream | no | reference |
| Usage ledger in PostgreSQL | Metering | Exactly-once meter events, reconciliation with runtime | yes | not separate software: tables a lesson creates on the configured PostgreSQL (`postgres://trinkets:trinkets@localhost:5432/trinkets`) |
| OpenMeter, Lago | Metering | Metering and billing services; dedup and aggregation windows | no | compose; reference |
| Stripe meters | Metering | Push aggregated usage with idempotency keys | no | test mode; reference |

## Running configured software

Commands run from the repo root: `make up-<service>`, `make down-<service>`,
`make clean-<service>`, `make logs-<service>`, `make ps`. The names are
`postgres`, `valkey`, `redis`, `seaweedfs`, `nats`, `etcd`, `registry`, `toxiproxy`,
`temporal`, `openbao`, and `pgbouncer`. Each is its own Compose project with its
own network; a service that reaches another one (PgBouncer to PostgreSQL, a
Toxiproxy upstream) goes through the host as `host.docker.internal:<port>`.

### Port conflicts

Host ports default to the standard ones. If something else already holds a
port, override it with an environment variable when starting the service:

| Variable | Default |
|---|---|
| `POSTGRES_PORT` | 5432 |
| `VALKEY_PORT` | 6379 |
| `REDIS_PORT` | 6380 (Valkey holds 6379) |
| `SEAWEEDFS_S3_PORT` | 8333 |
| `SEAWEEDFS_MASTER_PORT` | 9333 |
| `SEAWEEDFS_FILER_PORT` | 8888 |
| `NATS_PORT` | 4222 |
| `NATS_MONITOR_PORT` | 8222 |
| `ETCD_PORT` | 2379 |
| `REGISTRY_PORT` | 5050 (5000 is macOS AirPlay) |
| `TOXIPROXY_PORT` | 8474 (proxy listeners 22000-22009 are fixed) |
| `TEMPORAL_PORT` | 7233 |
| `TEMPORAL_UI_PORT` | 8233 |
| `OPENBAO_PORT` | 8200 |
| `PGBOUNCER_PORT` | 6432 |

    POSTGRES_PORT=15432 make up-postgres

### postgres.compose.yaml

Single `postgres:18` container started with `wal_level=logical`. User, password,
and database are all `trinkets`. Data lives in the `trinkets-postgres_postgres-data`
volume.

### valkey.compose.yaml

Single `valkey/valkey:9` container with append-only persistence, so keys survive
`down`/`up` until `clean`, and `notify-keyspace-events Ex` so expiries publish to
`__keyevent@0__:expired`. No password. Data lives in `trinkets-valkey_valkey-data`.

### redis.compose.yaml

Single `redis:8` container (Redis Open Source, multi-arch, arm64 included) with
append-only persistence and `notify-keyspace-events Ex`, like the Valkey one. No
password. Data lives in `trinkets-redis_redis-data`. The host port is 6380 so it
runs beside Valkey on 6379. The image bundles the JSON, Query Engine (`search`),
vector set, time series and Bloom modules, already loaded.

### Choosing Redis or Valkey

Both speak RESP and go-redis v9 talks to both; the Go helpers are
`internal/lab/valkey` (`CACHE_URL`, port 6379) and `internal/lab/redis`
(`REDIS_URL`, port 6380). Pick per lesson by feature set. When one fits the
lesson better, use it and say why in the lesson or idea. When it does not
matter, default to Redis. Completed and planned Valkey lessons stay on Valkey.

Rule of thumb: choose Redis when the lesson needs JSON, search or secondary
indexes, vector sets, time series, probabilistic structures (Bloom, Cuckoo,
count-min sketch, Top-K), or `DELEX`. Choose Valkey when the lesson is about
Valkey-specific behavior (`DELIFEQ`, numbered databases in cluster mode) or the
open-source-fork story. Otherwise Redis.

Verified 2026-09-29 against Redis 8.10.2 (`redis:8`) and Valkey 9.1.2
(`valkey/valkey:9`):

| Behavior | Redis 8 | Valkey 9 |
|---|---|---|
| `JSON.SET`, `VADD` (vector sets), search, time series, Bloom | work; `MODULE LIST` shows the five modules | unknown commands; only the `lua` module is listed |
| `SET key val IFEQ old`, `HSETEX ... EX` | supported | supported |
| Compare-and-delete | `DELEX key IFEQ val`; rejects `DELIFEQ` | `DELIFEQ key val`; rejects `DELEX` |
| `CONFIG GET notify-keyspace-events` with `Ex` set | `xE` | `xE` |
| `COMMAND COUNT` | 447 | 257 |

Not verified here: Valkey is BSD-3 licensed under the Linux Foundation, Redis 8
is tri-licensed RSALv2, SSPLv1 and AGPLv3, and Valkey 9 supports numbered
databases in cluster mode and atomic slot migration.

### seaweedfs/

- `compose.yaml` runs `weed server` (master, volume, filer, and S3 gateway in one
  process). Ports: 8333 S3 API, 9333 master UI, 8888 filer UI.
- `s3.json` holds the S3 identities. Access key `trinkets`, secret key
  `trinkets-secret`, full permissions. Use path-style addressing and any region
  (for example `us-east-1`).

Data lives in `trinkets-seaweedfs_seaweedfs-data`.

### nats.compose.yaml

`nats:2-alpine` with JetStream (`-js`) storing under the `trinkets-nats_nats-data`
volume and the monitoring endpoint on 8222. No auth. The alpine tag is used
because the plain `nats:2` image has no shell for a healthcheck.

### etcd.compose.yaml

Single-node `gcr.io/etcd-development/etcd:v3.5.17`, no auth, data in
`trinkets-etcd_etcd-data`. Peer URLs use the literal `127.0.0.1` because etcd
resolves `localhost` to the container IP and then rejects its own
`--initial-cluster` entry.

### registry.compose.yaml

`registry:2` with deletion enabled, data in `trinkets-registry_registry-data`.
Deleting a manifest leaves the repository name in `/v2/_catalog` until a
garbage-collect run; `make clean-registry` is the reliable reset.

### toxiproxy.compose.yaml

`ghcr.io/shopify/toxiproxy:2.9.0`, no data. Proxies are created through the
API at runtime and live in memory, so recreate them after `down`/`up`. A
proxy must `listen` on `0.0.0.0:<port>` inside the container, within the
published range 22000-22009, and reach other services as
`host.docker.internal:<port>`. The image has no shell and no `PATH` entry for
its CLI, so the healthcheck calls `/toxiproxy-cli` by full path.

### temporal.compose.yaml

`temporalio/temporal:1.9.1` running `server start-dev` with its own SQLite
file in the `trinkets-temporal_temporal-data` volume mounted at
`/home/temporal` (the image runs as uid 1000 and cannot write a fresh volume
elsewhere). It does not use the repo's PostgreSQL. Only patch tags are
published for this image. Workflows survive `down`/`up` until `clean`.

### openbao.compose.yaml

`openbao/openbao:2.1.0` in dev mode: unsealed, in memory, root token
`trinkets` via `BAO_DEV_ROOT_TOKEN_ID` (the entrypoint builds the dev
command from that variable; passing the flag directly duplicates it). No
volume, so everything is gone after `down`. `docker exec` commands need
`-e BAO_TOKEN=trinkets`.

### pgbouncer.compose.yaml

`edoburu/pgbouncer:v1.25.2-p0` forwarding any database name to the repo's
PostgreSQL through `host.docker.internal:${POSTGRES_PORT:-5432}`. Start
`make up-postgres` first. `AUTH_TYPE=scram-sha-256` because PostgreSQL 18
stores SCRAM verifiers; the entrypoint writes the plaintext password to
`userlist.txt`. Pool mode and size come from `PGBOUNCER_POOL_MODE` and
`PGBOUNCER_POOL_SIZE`; changing them recreates the container. The
healthcheck only proves PgBouncer is listening, not that PostgreSQL is up,
and after a PostgreSQL restart the first query can fail for about 15 s
(`server_login_retry`).
