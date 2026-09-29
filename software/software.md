# Software

One table of every piece of software worth building a lesson on, with what it
is for and whether this repo is set up to use it yet. The north star is the
skill set of someone building a platform like Daytona, E2B, Modal, or Vercel
Sandbox, or the agent compute behind OpenAI and Anthropic products: many
isolated environments, started fast, fed files and secrets, driven by an agent,
observed, metered, and torn down.

Columns:

- **Layer**: where it sits in such a platform.
- **Use case**: what a lesson would learn from it.
- **Configured**: `yes` means this repo already runs or imports it (a compose
  file here, a helper in `internal/lab` or `lessons/deno/lab`, or an installed
  tool the Makefile uses). `no` means nothing is set up yet.
- **Setup**: how it runs or would run here. `compose` means a new
  `<name>.compose.yaml` in this folder plus a `compose_file_<name>` line and the
  name in `SERVICES` in the root `Makefile` is all it needs. `library` means
  `go get` or an npm/JSR import, nothing to start. `Linux host` means it needs a
  Linux kernel with KVM and cannot run under Docker Desktop. `reference` means
  learn the idea, not worth a dependency yet.

Machine at the time of writing: macOS 27, Apple M5 Max (arm64), Docker Engine
29 via Docker Desktop, Lima 2.2, Apple `container` 1.4.1 (installed, kept
stopped).

| Software | Layer | Use case | Configured | Setup |
|---|---|---|---|---|
| Go | Runtime | Lessons, control-plane code, in-sandbox daemons; every host-side component below has a Go client | yes | brew; one root `go.mod`, helpers in `internal/lab` |
| Deno (TypeScript) | Runtime | Lessons; also a sandbox in its own right through `--allow-*` flags and `Worker` permissions | yes | brew; `lessons/deno/deno.json`, helpers in `lessons/deno/lab` |
| Docker Compose | Runtime | Starts each backing service as its own project: `make up-<service>`, `down-`, `clean-`, `logs-`, `ps` | yes | Docker Desktop; compose files in this folder |
| k6 | Load | Concurrent traffic for each lesson's HTTP follow-up | yes | brew; `scripts/perf/run.ts` |
| DuckDB | Analysis | Reads `measurements.csv` in every `analyze.sql` | yes | brew CLI |
| PostgreSQL 18 | Storage | Control-plane state: sandboxes, owners, leases, jobs, usage ledger | yes | `postgres.compose.yaml`; `postgres://trinkets:trinkets@localhost:5432/trinkets` |
| Valkey 9 | Storage | Cache, queues, pub/sub, leases, rate limits, idle timers | yes | `valkey.compose.yaml`; `redis://localhost:6379` (no auth) |
| SeaweedFS | Storage | S3 objects: artifacts, snapshots, uploads | yes | `seaweedfs/compose.yaml`; `http://localhost:8333`, access key `trinkets`, secret key `trinkets-secret`, region `us-east-1`, path-style |
| PostgreSQL `LISTEN`/`NOTIFY` | Storage | Wake workers without polling | yes | built into the running PostgreSQL |
| PostgreSQL logical replication | Storage | Stream state changes to a cache or index | yes | `wal_level=logical` is set in `postgres.compose.yaml`; create a publication and replication slot from the lesson |
| Valkey Streams | Storage | Consumer groups, pending entries, claim-on-crash; compare with JetStream | yes | built into the running Valkey |
| Valkey keyspace notifications | Storage | Key expiry as a reaper trigger | yes | `--notify-keyspace-events Ex` is set in `valkey.compose.yaml`; subscribe to `__keyevent@0__:expired` |
| pgvector | Storage | Agent memory and retrieval | no | swap the image to `pgvector/pgvector:pg18` in `postgres.compose.yaml` |
| PgBouncer | Storage | Thousands of runners against one PostgreSQL; transaction versus session pooling | yes | `pgbouncer.compose.yaml`, start postgres first; `postgres://trinkets:trinkets@localhost:6432/trinkets`, admin console `postgres://trinkets:trinkets@localhost:6432/pgbouncer`; knobs `PGBOUNCER_POOL_MODE` (transaction) and `PGBOUNCER_POOL_SIZE` (5) |
| ClickHouse | Storage | Usage and event analytics at scale | no | compose; reference, DuckDB covers the lesson value |
| Docker Engine API | Isolation | Honest sandbox-host stand-in: create, exec, stop, kill, cgroup limits, OOM, stats, idle reaping | no | library; the socket at `/var/run/docker.sock` is already there; Go `github.com/docker/docker/client`; Deno over a Unix socket is uncertain, use Go |
| containerd + runc | Isolation | The OCI layer under Docker: snapshotters, namespaces, cgroups | no | library, against Docker Desktop's containerd or a Lima guest |
| cgroups v2 | Isolation | CPU throttling, memory limits, per-sandbox usage numbers | no | read `/sys/fs/cgroup` from Go inside a container |
| Apple `container` | Isolation | One Linux VM per container on this Mac: boot latency, per-VM disks | no | installed via brew; `container system kernel set --recommended` once, then `container system start` and `container system stop` per session |
| Lima | Isolation | A Linux guest on this Mac when a lesson needs one | no | installed via brew |
| Firecracker | Isolation | MicroVM boot, snapshot and restore, memory lazy-loading, vsock (E2B, Vercel Sandbox) | no | Linux host; `firecracker-go-sdk` |
| Cloud Hypervisor | Isolation | Kata's VMM; CPU and memory hotplug | no | Linux host; reference |
| Kata Containers | Isolation | VM isolation with a container UX | no | Linux host |
| gVisor (`runsc`) | Isolation | User-space kernel (Modal); syscall cost versus VM boot cost | no | Linux host, Docker runtime flag |
| libkrun, krunvm | Isolation | MicroVM on Hypervisor.framework; Podman machine's macOS provider | no | brew; reference |
| bubblewrap, nsjail, Landlock, seccomp | Isolation | Process confinement as used by Claude Code and Codex CLI on Linux | no | Linux host |
| `sandbox-exec` (Seatbelt) | Isolation | macOS process confinement; the same story on this Mac | no | built into macOS; exec from Go or Deno |
| Deno permissions and Workers | Isolation | Run agent-generated TypeScript with explicit allow lists; cost of a denial | yes | built into Deno |
| wazero | Isolation | In-process WebAssembly with fuel metering, memory limits, cancellation | no | library, `github.com/tetratelabs/wazero` |
| wasmtime, WASI | Isolation | Same lesson from the CLI; WASI filesystem capabilities | no | brew; reference |
| CRIU | Isolation | Checkpoint and restore a process tree | no | Linux host; reference |
| OCI registry (`registry:2`) | Images | Push and pull, layer dedup, pull latency versus image size, garbage collection | yes | `registry.compose.yaml`; `localhost:5050` (no auth), `docker push localhost:5050/<repo>:<tag>`, HTTP API under `/v2/`; Go `github.com/google/go-containerregistry` |
| BuildKit | Images | Dockerfile builds, cache mounts, layer cache hits (Daytona snapshots) | no | already inside Docker Desktop as `docker buildx`; Go client optional |
| eStargz, SOCI, Nydus | Images | Start a container before the image finishes downloading | no | Linux host; reference |
| overlayfs | Filesystem | Copy-up cost on the top layer; `--rm` cleanup | no | observe inside any container |
| btrfs, ZFS snapshots | Filesystem | Instant clone of a warm sandbox disk | no | Linux host; reference |
| NBD, ublk | Filesystem | A root disk served to a microVM from a host process | no | Linux host; reference |
| virtiofs, 9p | Filesystem | Mount a workspace into a VM without copying it | no | Lima or Apple `container` mounts |
| JuiceFS | Filesystem | POSIX workspaces over S3 with metadata in Valkey or PostgreSQL; small-file cost | no | brew CLI plus macFUSE, on the existing S3 and Valkey |
| rsync, Mutagen | Filesystem | Local-to-sandbox sync, conflicts, bandwidth | no | brew |
| fsnotify, `Deno.watchFs` | Filesystem | Watch-this-directory events; coalescing and overflow | no | library (Go), built into Deno |
| Sandbox daemon (like E2B `envd`, Daytona toolbox) | Daemon | The binary inside every sandbox: exec, PTY, files, ports | no | write it in Go as a lesson core |
| vsock | Daemon | Host-to-guest socket without networking | no | Linux host; `mdlayher/vsock` |
| gRPC, Connect RPC, protobuf | Daemon | Typed, streaming RPC between orchestrator, runner, and daemon | no | library; `buf` via brew |
| WebSocket, SSE | Daemon | Terminal, logs, and events to a browser; backpressure | no | library (Go), built into Deno |
| `creack/pty`, `gliderlabs/ssh` | Daemon | PTY allocation and an embeddable SSH server; resize, exit codes, signals | no | library |
| NATS with JetStream | Control plane | Ack, ack-wait, max-deliver redelivery, dedup by message id, KV compare-and-swap, watches, object store | yes | `nats.compose.yaml`; `nats://localhost:4222` (no auth), monitoring `http://localhost:8222`; Go `github.com/nats-io/nats.go`, Deno `@nats-io/transport-deno` on JSR |
| Temporal | Control plane | Durable execution: retries, timers, signals, deterministic replay | yes | `temporal.compose.yaml` (self-contained dev server); gRPC `localhost:7233`, namespace `default`, no auth, UI `http://localhost:8233`; Go `go.temporal.io/sdk` only, the TypeScript SDK needs a Node native addon |
| DBOS Transact | Control plane | Durable execution as a library on PostgreSQL | no | library on the existing postgres |
| Restate | Control plane | Durable RPC and virtual objects | no | compose; reference |
| Inngest dev server, Hatchet | Control plane | Event-driven steps, fan-out, concurrency keys | no | compose; reference |
| River, pgmq, Graphile Worker | Control plane | PostgreSQL job queues to compare with the repo's own | no | library or extension on the existing postgres; reference |
| etcd | Control plane | Leases, watches, elections: placement, heartbeat lapse, fencing stale writers | yes | `etcd.compose.yaml`; `http://localhost:2379` (no auth); Go `go.etcd.io/etcd/client/v3`, Deno is weak |
| Kubernetes (kind, k3d, k3s) | Control plane | Pods per sandbox, `runtimeClass`, node pressure | no | brew; heavy; reference |
| Nomad | Control plane | Bin packing and allocation lifecycle | no | brew; reference |
| Kafka, Redpanda | Control plane | Partitioned logs and consumer groups | no | compose; reference, NATS covers the lesson value |
| Go `httputil.ReverseProxy` | Networking | Per-sandbox preview URLs, host routing, WebSocket upgrade, timeouts, retries | no | Go standard library |
| Caddy, Traefik, Envoy | Networking | Wildcard TLS, dynamic routes, connection draining, per-route limits | no | compose or brew; Caddy first |
| Linux netns, veth, tap, nftables | Networking | A network per sandbox, NAT egress, deny-by-default policy | no | Linux host; `vishvananda/netlink` |
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
| Anthropic SDK, Claude Agent SDK | AI | An agent that drives a sandbox through tools; streaming, cancellation, token budgets | no | library (Go SDK; TypeScript SDK runs in Deno); `ANTHROPIC_API_KEY` |
| Model Context Protocol (MCP) | AI | Expose a sandbox as tools: exec, read, write, ports | no | library (Go SDK, TypeScript SDK) |
| Ollama, llama.cpp | AI | Local models for experiments without API spend | no | brew or compose |
| Jupyter kernel protocol | AI | Stateful code execution that survives across calls (E2B code interpreter) | no | inside a container; reference |
| Pyodide, WebContainers | AI | Browser-side isolation and its limits | no | reference |
| Language servers (`gopls`, `typescript-language-server`) | AI | Editor intelligence served from inside the sandbox; process lifetime and memory | no | brew; reference |
| devcontainer spec and CLI | AI | Build a sandbox from `devcontainer.json` (Daytona's origin) | no | npm CLI |
| Nix, devbox | AI | Reproducible toolchains and store dedup | no | reference |
| Git, Forgejo | AI | Clone into a sandbox, push results, webhooks that start jobs | no | git is installed; Forgejo via compose when needed |
| Playwright, headless Chromium | AI | Computer-use agents inside a sandbox; memory and startup cost | no | inside a container; reference |
| `ttyd`, xterm.js | AI | The browser end of a PTY stream | no | reference |
| Usage ledger in PostgreSQL | Metering | Exactly-once meter events, reconciliation with runtime | no | tables in a lesson on the existing postgres |
| OpenMeter, Lago | Metering | Metering and billing services; dedup and aggregation windows | no | compose; reference |
| Stripe meters | Metering | Push aggregated usage with idempotency keys | no | test mode; reference |

## Running configured software

Commands run from the repo root: `make up-<service>`, `make down-<service>`,
`make clean-<service>`, `make logs-<service>`, `make ps`. The names are
`postgres`, `valkey`, `seaweedfs`, `nats`, `etcd`, `registry`, `toxiproxy`,
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
