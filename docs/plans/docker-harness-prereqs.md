# Plan: shared Docker harness prerequisites

Status: **H1–H6 in implementation** (plan written 2026-09-30; the user asked
for AI implementation the same day). Catalog rows stay `no` until the Mac gate
(H7) passes; see D4.

This plans the three Docker chunks under "Shared prerequisites" in
[projects.md](../lessons/projects.md): the Go Docker client and isolation
harness, the Unix peer-identity harness, and the Unix-socket egress broker.
The Go S3 helper is the fourth shared prerequisite; it is not Docker work and
is not planned here. BuildKit is also outside this plan (see Non-goals).

Finishing a work item is what flips a catalog row in
[software.md](../../software/software.md). This document flips nothing.

## What this unblocks

| Work item | Catalog rows it flips | Ideas it unblocks |
|---|---|---|
| H1–H4 | `Docker Engine API`, `Docker isolation harness`, `cgroups v2` | `worker-capabilities`, `focused-runtime-cell`, `noisy-neighbor-limits`, the container half of `surrogate-credential-broker`, dynamic variants of `container-namespace-boundary` and `exec-cancel-reap` |
| H5 | `Docker Unix peer-identity harness` | `peer-authenticated-tool-broker` |
| H6 | `Unix-socket egress broker` | `enforced-egress-path` |
| not here | `BuildKit` | `prepared-container-start` stays blocked on BuildKit after H4 |

## Decisions

- **D1. Who writes the harness.** Decided 2026-09-30: AI-implemented, by
  Sonnet or Opus subagents under an Opus coordinator, with a Fable agent for
  a stuck item. projects.md calls these chunks "configuration work in the
  root module, not a lesson". The lessons the harness unblocks stay with the
  learner, so the harness holds no policy: no ACL, no destination rules, no
  probe verdicts a lesson is meant to discover (see Boundary below).
- **D2. Launcher instead of Compose for the peer-identity and egress
  topologies.** projects.md words those two as "Compose services". This plan
  builds them with the H3 launcher: lesson fixtures are images built at run
  time from the lesson's own Go packages, UIDs vary per run, and a second
  mechanism would need its own build and cleanup path. Compose remains the
  tool for backing services in `software/`. Say so if you prefer Compose;
  the verified facts below apply either way.
- **D3. Test gating.** Tests that need a daemon skip unless
  `TRINKETS_DOCKER=1` and run through `make check-docker`. `make test` and
  `make fuzz` are daemon-free.
- **D4. When rows flip.** Only after the Mac gate (H7) passes for that item.
  The repo baseline is Docker Desktop on macOS; the implementation evidence
  is from Linux amd64 only. Until then the rows stay `no`, the ideas stay
  blocked, and the catalog and idea doc sweeps in H4–H6 wait.
- **D5. Testing approach.** Decided 2026-09-30: spec tests, named invariants
  enforced by the harness, and fuzz targets that assert those invariants as
  properties. See Testing approach below; it is the norm for project work
  (root `AGENTS.md`).

## Boundary: what the harness is and is not

The harness is a thin, explicit launcher. Every isolation knob is a named
field a lesson can flip one at a time, because the lessons compare a broad
container with a restricted one. A preset gives the restricted baseline; the
zero value gives Docker's defaults.

It does not contain: the peer-credential ACL (`peer-authenticated-tool-broker`
is the learner's), destination or redirect policy (`worker-egress-grants` is
the learner's), lesson probes, or any measurement CSV. Its own tests use one
small fixture binary that reports what it can see and do.

Hard rules, enforced in code and tested:

- The launcher runs on the host side. It never mounts the Docker socket into
  a container and rejects any mount whose source is the resolved socket path,
  `/var/run/docker.sock` or `/run/docker.sock`.
- There is no `Privileged` field.
- Everything the harness creates carries the label `trinkets.harness=1` and
  `trinkets.lesson=<name>`, so cleanup never touches backing services.
- The OCI runtime handler is a field from day one (`HostConfig.Runtime`); `""`
  means the engine default. A later `runsc` variant is then one field, per
  [mac-portability.md](../../knowledge/mac-portability.md).

## Verified facts (scratch spike, 2026-09-30)

Run in a scratch module outside the repo, on Linux amd64, Docker Engine
29.7.2 (API 1.55, containerd image store, cgroup v2, runc 1.4.3), Go 1.26.8.
All spike containers, volumes and images were removed. Nothing here has been
run on Docker Desktop.

Client:

- The current client is `github.com/moby/moby/client` v0.6.0 with types in
  `github.com/moby/moby/api` v1.56.0. `github.com/docker/docker` stops at
  v28.5.2+incompatible; do not add it. The pair adds 2 direct and 18 indirect
  modules in the scratch module (17 in this repo, which already required
  `golang.org/x/sys`) (OpenTelemetry, go-connections, errdefs and similar).
- The API is option-struct based, not the old positional one:
  `cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config, HostConfig,
  Name})` returns `ContainerCreateResult{ID, Warnings}`; `ContainerStart`,
  `ContainerStop`, `ContainerRemove`, `ExecCreate`, `ExecAttach`,
  `ExecInspect`, `ContainerInspect`, `ContainerLogs`, `Ping` and `Info` all
  take an options struct and return a result struct. Old blog snippets do not
  compile.
- `client.New(client.FromEnv)` reads `DOCKER_HOST` and otherwise uses
  `unix:///var/run/docker.sock`. It never reads the Docker CLI context.
  `docker context inspect --format '{{.Endpoints.docker.Host}}'` prints the
  active endpoint (`unix:///var/run/docker.sock` here).
- `Info().Info.Architecture` is the kernel name (`x86_64`), not a `GOARCH`.
  `docker version --format '{{.Server.Arch}}'` prints `amd64`.
- Negotiated API version came from the first request without configuration.

Images:

- `cli.ImageBuild` with an in-memory tar context (`FROM scratch`, `COPY`,
  `COPY --chown=10001:10001`, `USER 10001:10001`, `ENTRYPOINT`) built and
  tagged an image in 2.6 s. This is the classic builder; `Ping` reports
  `BuilderVersion` 2, yet the classic path still works on this engine.
- A static fixture needs `CGO_ENABLED=0 GOOS=linux GOARCH=<engine arch>`.
  The stripped spike binary was 6 MB. `go build` outside a clean git checkout
  can fail with `error obtaining VCS status`; `-buildvcs=false` avoids it.
- `docker cp` into a container with a read-only root filesystem is refused
  (`container rootfs is marked read-only`). Fixture binaries must be baked
  into the image, not copied in.

Isolation, same image and probe, two containers:

| Probe | Default, `user 0:0` | Restricted |
|---|---|---|
| identity | uid 0 | uid 10001, gid 10001 |
| `CapEff` | `00000000a80425fb` | `0000000000000000` |
| `NoNewPrivs` | 0 | 1 |
| `Seccomp` mode | 2 (builtin profile) | 2 |
| visible PIDs | 1 | 1 |
| interfaces | `lo eth0` | `lo` |
| write root filesystem | ok | `read-only file system` |
| write named volume at `/work` | ok | ok |
| write `/tmp` | fails, no `/tmp` in a scratch image | ok (tmpfs mount) |
| `setuid(0)`, `chown` | ok | `operation not permitted` |
| TCP dial out | ok | `network is unreachable` |
| `mount`, `unshare(CLONE_NEWUSER)`, `keyctl` | denied | denied |
| `memory.max` / `pids.max` / `cpu.max` | `max` / 9483 / `max 100000` | `67108864` / `64` / `50000 100000` |

Restricted means: `User: "10001:10001"`, `NetworkMode: "none"`,
`ReadonlyRootfs`, `CapDrop: ["ALL"]`, `SecurityOpt:
["no-new-privileges:true"]`, `IpcMode: "private"`, `CgroupnsMode: "private"`,
`Runtime: "runc"`, `Memory` = `MemorySwap` = 64 MiB, `NanoCPUs` 0.5 CPU,
`PidsLimit` 64, a tmpfs at `/tmp`, a named volume at `/work`.

- A named volume takes its ownership from the image directory at the mount
  point: the image's `/work` was `10001:10001`, so the non-root worker could
  write the fresh volume. With no such directory the volume is root-owned and
  a non-root `bind()` fails with `permission denied`.
- Seccomp through the API is the profile JSON inline
  (`seccomp={"defaultAction":...}`). A file path fails at **start**, not
  create: `Decoding seccomp profile failed`. The CLI reads the file for you;
  the API does not. A profile denying `socket` changed the dial error from
  `network is unreachable` to `socket: operation not permitted`.
  `seccomp=unconfined` gives `Seccomp: 0`.
- An unknown runtime (`Runtime: "runsc"`) fails at **create**:
  `unknown or invalid runtime name: runsc`.
- Registering `ContainerWait` with `WaitConditionNextExit` before
  `ContainerStart` caught a container that exits within milliseconds.
- Exec: drain the attach stream with `stdcopy.StdCopy`
  (`github.com/moby/moby/api/pkg/stdcopy`), then read the exit code from
  `ExecInspect`. A usage error in the probe returned exit 2. Exec inherits
  the container's user and restrictions.
- Timings on this machine, for a baseline only: create to exit 340–560 ms,
  exec round trip 46–82 ms, stop with a handled SIGTERM 0.15 s, stop with
  timeout 0 plus remove about 190 ms.

Peer identity across containers on one named volume:

- An init container as root with only `CHOWN` and `FOWNER` added back set the
  socket directory to `20000:30000`. (Corrected in H5: as `0:0` the mode came
  out `0750`, because chmod clears the setgid bit for a caller outside the
  file's group without `CAP_FSETID`; running the init container as
  `0:30000` gives `2750`.)
- Broker `20000:30000` listened with umask `007`; the socket was `srwxrwx---`.
- Workers `20001:20001` and `20002:20002` with supplementary group `30000`
  connected. `SO_PEERCRED` gave the broker each worker's UID and **primary**
  GID. The peer PID was `0`, because the worker is in another PID namespace;
  an exec inside the broker's own container reported a real PID. Identity
  must rest on UID/GID, never on PID.
- Worker `20003` without the group got `connect: permission denied`.
- A worker could not `chown` or `chmod` the socket.

Not spiked, to be proven inside the work items: read-only bind or volume
mounts (`ReadOnly: true`), OOM kill reporting, a worker trying to unlink or
replace the socket, user-defined networks and every egress bypass probe.
(H5 settled the socket ones: a worker on a read-only mount still connects,
and cannot unlink, replace, chown or chmod the socket on a read-only or a
read-write mount.)
(Whether cancelling an exec's context stops the process was settled in H3:
it does not.)

## Testing approach

Three layers, all in `internal/lab/docker`. The norm and its reasons are in
[project-testing.md](../../knowledge/project-testing.md).

1. **Spec tests.** Every "Done when" line is one test named
   `TestSpec_<Item>_<Behaviour>` (`TestSpec_H3_RejectsDockerSocketMount`).
   Table-driven where a statement has cases. A reader can map the plan to
   the test list with `go test -list '^TestSpec_' ./internal/lab/docker/...`.
2. **Invariants.** Named checks in `invariants.go`, each a small function
   returning violations. The harness enforces them itself: `CheckConfig`
   runs before every `ContainerCreate` and a violation is an error with no
   container created; `CheckObserved` compares `ContainerInspect` with what
   was asked for. Tests call the same functions, so production and tests
   cannot drift.
3. **Property and fuzz tests.** A property is an invariant asserted over
   generated inputs. Write each property once as `func(*rapid.T)` with
   `pgregory.net/rapid` (test-only, no transitive dependencies), run it as
   `TestProp_<Invariant>` with `rapid.Check`, and expose the same function to
   Go's coverage-guided fuzzer as `Fuzz<Thing>` with `rapid.MakeFuzz`. Byte
   and string parsers get plain native fuzz targets. A fuzz target must
   assert a property, never only "does not panic".

Config invariants (pure, no daemon):

| Name | Statement |
|---|---|
| `no-docker-socket` | No emitted mount is a bind mount, and no mount source, after `path.Clean`, is `/var/run/docker.sock`, `/run/docker.sock` or the resolved `Host()` socket path |
| `never-privileged` | `Privileged` is false and no namespace mode (`PidMode`, `NetworkMode`, `IpcMode`, `UTSMode`, `UsernsMode`, `CgroupnsMode`) is `host` |
| `labelled` | Labels `trinkets.harness=1` and a non-empty `trinkets.lesson` are present, and the name starts with `trinkets-<lesson>-` |
| `swap-pinned` | `Memory > 0` implies `MemorySwap == Memory` |
| `seccomp-inline` | A `seccomp=` option is `unconfined` or a JSON object |
| `restricted-complete` | `IsRestricted(config, hostConfig)` holds for everything `Restricted()` returns (the user lives in `Config`): non-root numeric user, `CapDrop` contains `ALL`, no `CapAdd`, no-new-privileges, read-only root, network `none`, private PID/IPC/cgroup namespaces, memory, CPU and PID limits set |

Observed-state invariants (daemon, gated):

| Name | Statement |
|---|---|
| `inspect-matches-intent` | The security fields the engine recorded equal the ones sent |
| `no-leak` | After a handle is removed or `Sweep` runs, no container, volume or network carries that lesson label |
| `peer-uid-is-assigned-uid` (H5) | The UID the broker reads from `SO_PEERCRED` equals the UID the launcher assigned |
| `socket-group-gates-connect` (H5) | A worker without the socket group cannot connect |
| `worker-has-no-route` (H6) | The worker's only interface is `lo` |
| `blocked-fixture-untouched` (H6) | The blocked fixture's request count is zero |

Properties and fuzz targets:

| Test | Property |
|---|---|
| `TestProp_Translate` / `FuzzTranslate` | For any generated `Spec`, including hostile mount sources (`..`, doubled slashes, trailing slashes): `Translate` succeeds implies `CheckConfig` is empty and `HostConfig.Runtime == Spec.Runtime`; `Translate` fails implies zero-value configs; two calls give equal results |
| `TestProp_RestrictedPreset` | `Restricted(lesson, role, image, cmd...)` satisfies `restricted-complete` for any arguments |
| `TestProp_TighteningNeverLoosens` | Applying any tightening step (drop a capability, remove a mount, lower a limit) to a restricted spec leaves it restricted |
| `FuzzBuildStream` | The build-output parser returns an error exactly when some line holds an `errorDetail` or `error` key, or is not one JSON object (blank lines, CRLF and a missing final newline are fine) |
| `FuzzProbeOutput` | Parsing then formatting probe lines round-trips; one entry per line, malformed lines are reported (`Err`, `Raw`), not dropped |
| `FuzzContextTar` | `InvContextNamesClean`: every name in the generated build context is relative and clean; a description with a hostile path, tag, user or duplicate is rejected, not tidied; the tar reads back to the same files and a Dockerfile holding exactly the requested `COPY` destinations and owners |
| `TestProp_LifecycleNoLeak` (gated) | A `rapid` state machine runs random create/start/exec/stop/remove/sweep sequences against the daemon and a model; they agree and `no-leak` holds at the end |
| `TestProp_PeerUID` (gated, H5) | For random distinct worker UIDs, `peer-uid-is-assigned-uid` holds |
| `TestProp_EgressBypass` (gated, H6) | For random sequences of bypass probes, `blocked-fixture-untouched` holds |

Rules learned in the scratch check:

- An invariant must be exact. A sloppy "source ends with `docker.sock`"
  check made the fuzzer report the harmless volume name `0docker.sock`.
- `go test -fuzz` accepts one target in one package per run, so `make fuzz`
  loops over `go test -list '^Fuzz'`.
- Failing native inputs are written to `testdata/fuzz/<Target>/`; commit
  them, because plain `go test` replays them. `rapid` writes `.fail` files
  under `testdata/rapid/`; turn such a failure into a seed or a spec test
  and keep that directory out of git.
- Gated property tests hit a real daemon: cap them (`-rapid.checks=20` or a
  lower default set in the test) so `make check-docker` stays under a few
  minutes.

Make targets (added in H1, extended later): `test` runs `go test ./...`
(spec tests, property tests and fuzz seeds, no daemon); `fuzz` runs every
fuzz target for `FUZZTIME` (default `10s`); `check-docker` runs the gated
tests.

## Layout

```text
internal/lab/docker/
  docker.go        Host(), Connect(ctx), EngineArch(ctx, cli)
  image.go         BuildFixture(ctx, cli, FixtureImage), Labels, build-stream parser
  spec.go          Spec, Mount, Restricted(), Spec -> container.Config/HostConfig
  run.go           Create, Start, Wait, Logs, Exec, Stop, Remove, Run
  invariants.go    named config and observed-state invariants, CheckConfig, CheckObserved
  cleanup.go       Sweep(ctx, cli, lesson), labels
  topology.go      SocketVolume (H5), FixtureNetwork (H6)
  probe/           package main (linux only): the harness's own fixture binary
  probeout/        probe line format: Parse, Format, Find (stdlib only)
  *_test.go        TestSpec_*, TestProp_*, Fuzz*; daemon tests gated by TRINKETS_DOCKER=1
  testdata/fuzz/   committed fuzz seeds and regression inputs
```

`internal/lab` stays standard-library only. Only a lesson that imports
`internal/lab/docker` compiles the moby client, as with `lab/postgres`.

A lesson that needs its own fixture keeps it at
`lessons/go/<slug>/probe/main.go`. The Makefile discovers lessons from
`lessons/go/*/main.go`, so a `probe/` subdirectory adds no target, the same
as `perf/main.go` today.

## Work items

Order: H1 → H2 → H3 → H4, then H5, then H6. H7 is the Mac gate and runs after
H4, H5 and H6 each. Each item is sized for one implementer and one review.
"Done when" lists what must pass on the machine doing the work; each line
becomes a `TestSpec_` test, and every item also lands the invariants,
properties and fuzz targets the Testing approach tables assign to its code.
Every item ends with `make check`, `make test`, `make fuzz` and, from H2 on,
`make check-docker` passing.

### H1. Engine client helper

Goal: one way to get a connected client, matching `lab/postgres.Connect`.

- `go get github.com/moby/moby/client@v0.6.0` (pulls `moby/api` v1.56.0).
  Run `go mod tidy` only after a package imports it, or the requirement lands
  in the `// indirect` block (see [go-modules.md](../../knowledge/go-modules.md)).
- `Host()`: `DOCKER_HOST` when set; otherwise the output of
  `docker context inspect --format '{{.Endpoints.docker.Host}}'`; otherwise
  the client default. This is what "through the active Docker context" in
  projects.md requires, since `FromEnv` alone ignores the context. The
  `docker` CLI is already a repo requirement for `make up-<service>`.
- `Connect(ctx) *client.Client`: `client.New(client.FromEnv,
  client.WithHost(Host()))`, then `Ping`; fail fast with `lab.Check`, because
  `client.New` does not contact the daemon.
- `EngineArch(ctx, cli) string`: a `GOARCH` for the engine. Map
  `x86_64 → amd64` and `aarch64 → arm64` from `Info`, and fail on anything
  else instead of guessing.
- Add `pgregory.net/rapid` v1.3.0 for tests, the `requireDocker(t)` gate
  helper, and the Make targets `test`, `fuzz` and `check-docker`, with their
  lines in the header comment that `make help` prints. Add
  `testdata/rapid/` to `.gitignore`.
- Tests: spec tests for `Host()` precedence and the arch mapping;
  `FuzzArch` asserting that the mapping returns `amd64`, `arm64` or an
  error for any string and never another value.

Done when:

- `go vet ./...` and `go mod tidy -diff` exit 0, and `go.mod` lists
  `moby/client` and `moby/api` as direct requirements.
- A unit test covers `Host()` precedence with `DOCKER_HOST` set and unset.
- A gated test pings the daemon and prints API version and `EngineArch`.
- With the daemon stopped, `Connect` panics with the connection error rather
  than hanging (context deadline).

### H2. Fixture image builder and probe fixture

Goal: turn a Go package into a tiny image the launcher can run, with no
Dockerfile on disk and no toolchain image.

- `FixtureImage{Lesson, Tag, Package, User, Dirs []OwnedDir}`. `Lesson` is the
  `trinkets.lesson` label and part of the tag; `User` is a numeric `uid:gid`
  (default `10001:10001`); an `OwnedDir` is an absolute, clean path plus
  integer `UID` and `GID`, for directories a non-root worker must own once a
  volume is mounted there. Paths use only `[A-Za-z0-9._@+-]` and `/` (the
  builder expands `$` and `\` in COPY arguments); anything else, or a path
  `path.Clean` would change, is rejected, not tidied. Parents are copied
  before children because `COPY --chown` only owns what it creates.
- `BuildFixture`: run `go build -buildvcs=false -trimpath -ldflags "-s -w"`
  with `CGO_ENABLED=0 GOOS=linux GOARCH=EngineArch` into a temp dir; write an
  in-memory tar with the binary, one empty directory per owned directory
  (named `d/<i>`, never from the lesson's path) and a generated Dockerfile
  (`FROM scratch`, `COPY`, `COPY --chown`, numeric `USER`, `ENTRYPOINT`, with
  the real paths as JSON exec-form arrays); call `cli.ImageBuild` with the
  harness labels; read the JSON lines stream to the end and surface an
  `errorDetail` (or `error`) as an error. A Dockerfile that does not parse
  fails as the HTTP error of `ImageBuild` itself rather than in the stream;
  both come back as an error.
- Tag convention `trinkets-<lesson>-<name>:dev`. Rebuild every run; the build
  is about 3 s and avoids stale fixtures.
- `probe`: subcommands that each print `name: OK [detail]` or
  `name: DENIED (<error>)` lines (format and parser in `probeout`): `id`,
  `status` (CapEff, NoNewPrivs, Seccomp), `pids`, `interfaces`, `write`,
  `setuid`, `chown`, `chmod`, `unlink`, `stat`, `mount`, `unshare-user`,
  `keyctl`, `cgroup`, `battery` (the survey the isolation table needs), `dial`,
  `peer-echo` (listen and report `SO_PEERCRED`), `http-count`, `sleep
  [--ignore-term]`, `alloc <bytes>`, `fork <n>` and `initdir`. Linux only;
  `//go:build linux` so `go vet ./...` on macOS skips the syscalls it cannot
  compile. Exit 0, 1 when a line is DENIED (not for `battery`), 2 for usage,
  3 when a machine-changing command runs without the `TRINKETS_PROBE=1`
  marker every fixture image sets.

Risk: the classic builder is a legacy path. If `ImageBuild` fails on some
engine, the fallback is to write the same context to a temp dir and run
`docker build` there. Do not switch pre-emptively.

Done when:

- A gated test builds the probe image, inspects it, and sees the labels, the
  numeric user and an architecture equal to the engine's.
- The image has one binary and no shell (`docker run --rm <tag> id` works,
  `docker run --rm --entrypoint sh <tag>` fails). `id` is the probe's
  identity subcommand; the entrypoint failure shows at container start.
- A deliberately broken Dockerfile line produces a Go error, not a silent
  success.
- `GOOS=darwin go vet ./...` still passes (the probe is excluded, the rest
  compiles).

### H3. Launcher

Goal: create, start, exec, stop and remove fixture containers from an
explicit spec.

```go
type Spec struct {
    Lesson, Role string   // labels and the container name prefix
    Image        string
    Cmd, Env     []string
    User         string   // "uid:gid"; "" keeps the image's USER
    Groups       []string // supplementary groups (GroupAdd)
    Mounts       []Mount  // volume or tmpfs, explicit target, ReadOnly flag, TmpfsSize
    Network      string   // "none", "bridge" or a network name; "" = engine default
    ReadOnlyRoot bool
    CapDrop      []string
    CapAdd       []string
    NoNewPrivileges bool
    Seccomp      string   // "" builtin, "unconfined", or profile JSON
    PIDMode      string   // "" private, "container:<id>" shared
    Memory       int64    // bytes; swap is pinned to the same value
    NanoCPUs     int64
    PidsLimit    int64    // 0 = engine default (no limit)
    Runtime      string   // OCI runtime handler; "" = engine default
}

func Restricted(lesson, role, image string, cmd ...string) Spec
```

- `Restricted` returns the verified baseline above, except the named volume
  at `/work`: a volume outlives its container, so the lesson creates one with
  `CreateVolume` (labelled, swept) and appends a `Mount`. Its `Runtime` is
  `"runc"` by name. Private IPC and cgroup namespaces are not `Spec` fields:
  `Translate` always sends them, which equals the engine default on cgroup v2
  (checked in H3). The zero `Spec` plus lesson, role and image is Docker's
  default container, which is the lessons' broad baseline.
- Bind mounts from the macOS host are left out on purpose: named volumes and
  tmpfs avoid Docker Desktop file sharing. Add a bind type only when a lesson
  needs one.
- Translation `Translate(spec, socket, suffix) → Request{Name, Config,
  HostConfig}` is a pure function with table tests (the resolved socket path
  and the random name part are inputs, so it stays deterministic); it owns
  the socket-mount rejection and turns a `Seccomp` value that is not `""`,
  `unconfined` or a JSON object into an early error, since the engine would
  only fail at start.
- `Run(ctx, cli, spec) Result{ExitCode, Stdout, Stderr, OOMKilled, Duration}`
  for one-shot containers (wait registered before start). `Start` returns a
  handle with `Exec(ctx, cmd...) Result`, `Stop(ctx, grace)`, `Remove(ctx)`.
- Names are `trinkets-<lesson>-<role>-<short random>` so parallel runs do not
  collide. `Sweep(ctx, cli, lesson)` force-removes containers, volumes and
  networks with that lesson label; `Run` and handles remove their own
  container on the way out, `Sweep` is the net for crashes.
- Every call takes a context. A cancelled `Exec` returns `ctx.Err()` at
  once and the process keeps running inside the container until the
  container stops (verified in H3 with the probe's `pids`; the Engine API
  has no exec-kill call).

Done when:

- Unit tests (no daemon) cover the translation, the socket-mount rejection
  for all three paths, the path-like seccomp rejection, and that `Runtime` is
  passed through untouched.
- Gated tests: create/start/wait/logs/remove for a one-shot; exec exit codes
  0 and 2; stop within the grace period for a fixture that handles SIGTERM
  and a kill after it for one that ignores it; `Runtime: "no-such-runtime"`
  fails at create with the engine's message; after each test nothing
  carrying that test's own lesson label remains (filtering on
  `trinkets.harness` alone would see other items sharing the daemon).
- A test that leaks a container on purpose is cleaned by `Sweep`.

### H4. Isolation verification, Make targets, catalog

Goal: prove the boundary the catalog row describes, then publish it.

- One gated table test runs the probe twice (default with `User: "0:0"`,
  and `Restricted`) and asserts the verified table above, row by row, so a
  regression names the probe that changed. Add the cases the spike did not
  cover: a read-only volume mount rejects writes while a read-write one
  accepts them; two private containers cannot see each other's processes
  and a `PIDMode: "container:<id>"` pair can; `alloc` beyond the memory
  limit ends with `OOMKilled` true and exit 137; a fork loop stops at
  `PidsLimit`; the cgroup files are readable from inside the container.
- Makefile: `clean-harness` (remove containers, volumes, networks and
  images labelled `trinkets.harness`), added to the header comment that
  `make help` prints. `harness` is not a name in `SERVICES`, so the generated
  `clean-<service>` rules do not collide. `check-docker` exists since H1
  (`TRINKETS_DOCKER=1 go test -count=1 ./internal/lab/docker/...`).
- `TestProp_LifecycleNoLeak` and the `inspect-matches-intent` check landed
  in H3.
- After the Mac gate (not part of the Linux implementation pass): flip `Docker Engine API`, `Docker isolation harness`
  and `cgroups v2` to `yes` in `software/software.md` with the import path,
  the helper package and the `make check-docker` command in `Setup`, and add
  a short "Docker harness" section under "Running configured software".
- Doc sweep, with the flip and not before: readiness labels and Options lines for the
  ideas in the table at the top of this plan (`prepared-container-start`
  changes its blocker to BuildKit only), the "Shared prerequisites" bullet
  and the "once the harness exists" phrasing in projects.md, and the
  "still need implementation" statements in `knowledge/mac-portability.md`,
  `docs/agentic-platforms/README.md` and `docs/agentic-platforms/meta-muse.md`.
  Find them with
  `grep -rn -i "harness\|unconfigured\|blocked" docs software knowledge README.md`.
- Knowledge: extend `knowledge/docker-harness.md` with what the
  implementation proved and update `knowledge/index.md`.

Done when: `make check`, `make test`, `make fuzz`, `make check-docker` and
`make clean-harness` pass; `make ps` still lists only backing services; no
catalog row or idea label has changed before the Mac gate.

### H5. Unix peer-identity harness

Goal: a topology in which a broker can trust the kernel-supplied UID of each
worker, plus a check that the UID it sees is the UID that was assigned.
Depends on H3.

- `SocketVolume(ctx, cli, lesson, brokerUID, socketGID)`: create a labelled
  named volume and run a one-shot init container (root, `CapDrop: ALL`,
  `CapAdd: CHOWN, FOWNER`, no network, read-only root) that sets the mount
  directory to `brokerUID:socketGID` mode `2750`. Return a `Mount` for the
  broker (read-write) and one for workers. (As built: the init container
  runs as `0:socketGID` so chmod keeps the setgid bit, the worker mount is
  read-only, and identities above `2147483647` are refused because the
  engine fails them at start.)
- Broker spec: `Restricted` with `User: "<brokerUID>:<socketGID>"`. Worker
  spec: `Restricted` with its own `uid:gid` and `Groups: [socketGID]`. With
  all capabilities dropped and no-new-privileges set, a worker has no
  `SETUID`/`SETGID` to change identity.
- `VerifyPeerIdentity(ctx, ...)`: start the `peer-echo` fixture as broker,
  dial from two workers with different UIDs, and compare the UID the broker
  reports with the UID assigned. It fails loudly on a mismatch, which is what
  a user-namespace remap or Docker Desktop's Enhanced Container Isolation
  would produce. Lessons call it before measuring.
- The broker's ACL and method table are the learner's lesson. The harness
  ships only `peer-echo`.

Done when (gated tests):

- A fresh volume without the init step makes the broker's `bind` fail; with
  it the broker listens and the socket is `srwxrwx---`.
- Two workers are reported with their own UIDs and primary GIDs; the reported
  PID is 0 and the test documents why.
- A worker without the socket group is refused at `connect`.
- A worker cannot unlink, replace, `chown` or `chmod` the socket, and cannot
  `setuid` to the other worker's UID.
- `VerifyPeerIdentity` passes here and its failure path is unit tested with a
  faked mismatch.
- After the Mac gate: flip `Docker Unix peer-identity harness`, update
  `peer-authenticated-tool-broker` and the Focused personal agent computer
  notes.

### H6. Unix-socket egress topology

Goal: a worker with no network, a broker that alone can reach fixture
services, and evidence that no other path exists. Depends on H5.

- `FixtureNetwork(ctx, cli, lesson)`: a labelled user-defined bridge network.
  Fixture HTTP services (the probe's `http-count` subcommand: answer, count
  requests per path, expose the count) join it, one "allowed" and one
  "blocked". The broker joins it and mounts the H5 socket volume. The worker
  has `Network: "none"` and the socket volume only.
- Try `Internal: true` on the network first and record whether the gateway
  and host are still reachable from the broker;
  [mac-portability.md](../../knowledge/mac-portability.md) notes that Docker
  internal networks still allow gateway/host communication, which is why the
  worker gets no network at all instead of an internal one.
- A minimal forwarder fixture (one fixed destination, no redirects) proves
  the path works. Destination grants and redirect rechecks come from the
  learner's `worker-egress-grants` core when `enforced-egress-path` is built;
  the harness does not pre-empt them. (projects.md words this chunk as a
  broker that "enforces destination and redirect policy"; this plan moves
  that policy to the lesson and should update the wording when H6 lands.)
- Bypass probes from the worker, each expected to reach nothing: the blocked
  fixture's container IP, the allowed fixture's IP directly, the network
  gateway, `host.docker.internal`, alternate ports, a DNS lookup, and
  `HTTP_PROXY` pointing at the broker's network address.

Done when (gated tests):

- Every bypass probe fails and both fixtures' request counts stay at zero.
- A request through the socket reaches the allowed fixture exactly once.
- The worker's interface list is `lo` only.
- Stopping the broker makes the worker's request fail without a fallback path.
- `Sweep` removes the network, volume and all four containers.
- After the Mac gate: flip `Unix-socket egress broker`, update
  `enforced-egress-path` and projects.md.

### H7. Mac gate (Docker Desktop, arm64)

Run on the Mac after H4, H5 and H6: `make check && make test && make fuzz &&
make check-docker && make clean-harness`. This is the user's step; the
agents implementing H1–H6 work on Linux and cannot run it. Record the outcome with a date in
`knowledge/docker-harness.md`; a row flips only after its gate passes (D4).
Things expected to differ or still unknown there:

- Socket path. Docker Desktop's context points at a per-user socket, and
  `/var/run/docker.sock` exists only when the default-socket setting is on.
  `Host()` must work with it off.
- `Info` architecture should be `aarch64`, so fixtures build as `arm64`.
- The classic `ImageBuild` path on Docker Desktop's engine.
- The cgroup v2 files and the limit values inside a container.
- `SO_PEERCRED` UIDs with Docker Desktop's default settings and with
  Enhanced Container Isolation, if that is enabled.
- `host.docker.internal` resolves inside Docker Desktop containers, so the
  egress bypass probe for it exercises a real route there; on this Linux
  engine the name did not resolve at all.
- Timings; do not reuse the Linux numbers in a lesson.

## Non-goals

- BuildKit integration and its catalog row (`prepared-container-start`).
- The Go S3 helper.
- gVisor, Kata, containerd, userns-remap setups, or any VM runtime.
- A generic container library: no port publishing, no restart policies, no
  log streaming beyond captured output, until a lesson needs one.
- Lesson guides. After a row flips, `$create-lesson` plans the unblocked
  ideas as usual.

## Working notes for whoever implements this

- The agents in `.claude/agents/harness-*.md` target the deleted top-level
  `harness/` module (an HTTP invariant harness). They are not for this work.
- Suggested effort per item: H1, H2 and H4 on Sonnet at high effort (fixed
  interfaces, tests as spec); H3, H5 and H6 on Opus (security-sensitive
  translation, identity and network boundaries); an Opus review of every
  item against its "Done when" list before the next one starts.
- An error found in existing lesson code is reported, not fixed (root
  `AGENTS.md`, "This repo is for learning").
- Keep a `.state/` log per the `trinkets-work-log` skill; several items need
  more than one session.
- A failing gated test on the Mac is a finding to explain, not something to
  patch around: record it in `knowledge/docker-harness.md` and leave the row
  at `no`.
