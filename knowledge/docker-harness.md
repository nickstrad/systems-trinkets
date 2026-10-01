# Docker harness: Go Engine API client and isolation facts

Applies when writing or using `internal/lab/docker` (the shared Docker
prerequisites from `docs/lessons/projects.md`). The work-item plan, the full
probe table and the list of what is still unverified are in
[docs/plans/docker-harness-prereqs.md](../docs/plans/docker-harness-prereqs.md).
Catalog rows stay `no` until that plan's Mac gate passes.

## Verified 2026-09-30, scratch spike, Linux amd64, Engine 29.7.2 (API 1.55)

Nothing below has been run on Docker Desktop.

- **Module.** Use `github.com/moby/moby/client` (v0.6.0) with
  `github.com/moby/moby/api` (v1.56.0). `github.com/docker/docker` ends at
  v28.5.2+incompatible. Methods take option structs and return result
  structs (`ContainerCreate(ctx, client.ContainerCreateOptions{...})`), so
  older snippets do not compile.
- **Context.** `client.New(client.FromEnv)` reads `DOCKER_HOST` or falls back
  to `unix:///var/run/docker.sock`; it never reads the CLI context. Get the
  active endpoint with
  `docker context inspect --format '{{.Endpoints.docker.Host}}'`.
- **Architecture.** `Info` reports the kernel name (`x86_64`); map it to a
  `GOARCH` before cross-compiling a fixture.
- **Fixture images.** `ImageBuild` with an in-memory tar (`FROM scratch`,
  `COPY --chown`, numeric `USER`) works through the classic builder in about
  3 s. `docker cp` into a read-only root filesystem is refused, so bake the
  binary into the image. Build fixtures with `CGO_ENABLED=0` and
  `-buildvcs=false`.
- **Volume ownership.** A new named volume copies ownership from the image
  directory at its mount point. Without one it is root-owned and a non-root
  process cannot create files or bind a socket there. Bake an owned
  directory into the image, or run a one-shot init container as root with
  only `CHOWN` and `FOWNER`.
- **Seccomp through the API** is the profile JSON inline. A file path is
  rejected at start (`Decoding seccomp profile failed`), not at create; the
  CLI reads the file, the API does not.
- **Runtime.** An unknown `HostConfig.Runtime` fails at create.
- **Fast exits.** Register `ContainerWait` with `WaitConditionNextExit`
  before `ContainerStart`.
- **Exec.** Drain the attach stream with `stdcopy.StdCopy`, then read the
  exit code from `ExecInspect`.
- **Peer credentials across containers.** Over a Unix socket on a shared
  named volume, `SO_PEERCRED` returns the peer's UID and primary GID but PID
  `0`, because the peer is in another PID namespace. Authenticate on UID/GID
  only. Supplementary groups gate who may connect (directory `2750`, socket
  `0770` via umask `007`); they do not show up in the credentials.
- **Restricted baseline** (non-root user, `CapDrop: ALL`, no-new-privileges,
  read-only root, network `none`, memory/CPU/PID limits): `CapEff` is zero,
  the only interface is `lo`, `setuid`, `chown` and outbound dials fail, and
  `/sys/fs/cgroup/{memory.max,pids.max,cpu.max}` show the limits from inside
  the container.

The stale agents in `.claude/agents/harness-*.md` describe a deleted
top-level `harness/` module and are unrelated to this harness.

## Implementation notes

H1 (2026-10-01, Linux amd64, Engine 29.7.2, API 1.55):

- `docker.Host()` resolves `DOCKER_HOST`, then the active CLI context, then
  `client.DefaultDockerHost`. A context answer `client.ParseHostURL` rejects
  counts as no answer. The CLI call sits behind a package variable so unit
  tests cover precedence without a docker CLI.
- `Connect` pings under a 5 s deadline. Verified without stopping the
  daemon: a missing unix socket panics at once with an error naming the
  socket, and a TCP listener that accepts but never answers panics with
  `context.DeadlineExceeded` at the deadline.
- `EngineArch` reads `Info().Info.Architecture` (`x86_64` here) and maps it
  exactly; the mapping is a pure function with a native fuzz target.
- `go mod tidy` made `moby/client` and `moby/api` direct only after a
  non-test file imported the api package (`api/types/system`, for the `Info`
  type); it also added 17 indirect modules (`golang.org/x/sys` was already
  required) and `pgregory.net/rapid`.
- Still unverified on Docker Desktop: `Host()` against its per-user socket and
  `EngineArch` returning `arm64` (the Mac gate, H7).


H2 (2026-10-01, Linux amd64, Engine 29.7.2, containerd image store):

- **Fixture builder.** `BuildFixture` takes `FixtureImage{Lesson, Tag, Package,
  User, Dirs}`: `Lesson` is the `trinkets.lesson` label and the tag middle
  (`trinkets-<lesson>-<name>:dev`, validated), `User` is a numeric `uid:gid`
  defaulting to `10001:10001`, and an `OwnedDir` is a path plus integer
  `UID`/`GID`. A path that `path.Clean` would change, or that is relative,
  `/`, `/fixture...`, non-UTF-8 or holds a control character, is **rejected**,
  never tidied. Cold build (go build plus `ImageBuild`) took 3-5.5 s here.
- **Build context is flat.** The tar holds `Dockerfile`, `fixture` and
  `d/<i>` (one empty dir per owned dir), so no lesson path becomes a tar
  name; the real destination goes into the Dockerfile as a JSON exec-form
  array (`COPY --chown=U:G ["d/0", "/work"]`). The binary's mode (0755) comes
  from the tar header, not `COPY --chmod`. `InvContextNamesClean` guards the
  names in `writeTar`.
- **COPY arguments are expanded even in JSON form.** The builder expands `$`
  and handles `\` inside `COPY ["src", "dest"]`: in real builds `/w$ork`
  landed at `/w`, `/back\slash` at `/backslash`, `/x${y:-z}` at `/xz`, and a
  `"` in a path broke the build. JSON quoting does not protect a path. Owned
  directory paths are therefore an allow-list (`[A-Za-z0-9._@+-]` and `/`,
  no space); anything else is rejected. The gated test builds every allowed
  special character and reads the paths back from the saved layers.
- **`COPY --chown` only owns what it creates.** With `/a/b` copied before
  `/a`, `/a` is created root-owned as `/a/b`'s parent, and the later
  `COPY --chown` of `/a` leaves it that way because the destination already
  exists. `BuildFixture` copies owned directories in path order (an ancestor
  is a strict prefix of its descendants, so it sorts first); unlisted parents
  stay root-owned. `COPY --chown` of an empty directory creates it with the
  given owner (read back from the layer).
- **Where build failures show up.** A Dockerfile that does not parse comes
  back as an HTTP error from `ImageBuild` itself (`dockerfile parse error on
  line 2: unknown instruction: BOGUS`). A failing `COPY` or `RUN` arrives as
  an `errorDetail` object inside the 200 stream. `parseBuildStream` handles
  the stream; `buildContext` then inspects the tag, because a cut stream can
  look clean. Contract: JSON lines, each non-blank line one object, failure
  = an `errorDetail` or `error` key (any value), failure beats malformed.
- **Saved image layout.** `ImageSave` on the containerd store still writes a
  top-level `manifest.json` listing layer blobs, which is enough to list an
  image's files (the "one binary, no shell" test does this).
- **No shell.** `--entrypoint sh` fails at `ContainerStart` with `exec: "sh":
  executable file not found in $PATH`, not at create or as an exit code.
- **Probe line format** (`internal/lab/docker/probeout`, stdlib only so the
  probe does not link the moby client): `name: OK [detail]` or `name:
  DENIED (error)`, names `[a-z0-9][a-z0-9._-]*`, no control characters.
  `Parse` returns one entry per line and marks bad ones (`Err`, `Raw`)
  instead of dropping them. Probe exit status: 0, or 1 when a line is
  DENIED (`battery` is a survey and exits 0), 2 for a usage error, 3 when it
  refuses. Commands that change the machine (`write`, `setuid`, `chown`,
  `chmod`, `unlink`, `mount`, `unshare-user`, `keyctl`, `battery`, `alloc`,
  `fork`, `initdir`) refuse unless `TRINKETS_PROBE=1` is set; every fixture
  image bakes `ENV TRINKETS_PROBE=1`, so host-side tests can cover the pure
  helpers and usage paths but never run those actions on the host (they run
  in gated in-container tests, with `exec` of `/fixture <cmd>`: an exec does
  not apply the image entrypoint).
- **Probe battery reproduces the plan's isolation rows** (default `0:0` vs
  hand-written restricted): `cap-eff` `00000000a80425fb` vs `0000...`,
  `no-new-privs` 0/1, `seccomp` 2/2, `pids` `1`, `interfaces` `lo eth0` /
  `lo`, root write ok / `read-only file system`, `mount`, `unshare-user` and
  `keyctl` denied in both, `memory.max`/`pids.max`/`cpu.max` `max`/`9483`/
  `max 100000` vs `67108864`/`64`/`50000 100000`, `setuid` ok / denied.
- **Probe gotchas found while building it.**
  - `unshare(CLONE_NEWUSER)` from a Go program fails with `EINVAL` before any
    permission check (the runtime is multi-threaded), so allowed and denied
    containers would look alike; the probe uses `clone(CLONE_NEWUSER)` via
    `os/exec` on `/proc/self/exe`, which goes through the same checks.
  - `signal.Ignore(SIGTERM)` in a program that then blocks makes the Go
    runtime abort with "all goroutines are asleep" (exit 2) the moment
    anything else wakes it. "Ignore SIGTERM" is `signal.Notify` plus
    discarding. As PID 1, a handler is required either way, or the kernel
    drops the signal.
  - `PidsLimit` counts threads: each Go child of `fork` costs about 4-5 pids,
    so 64 allowed 12-14 children.
  - `mount` is probed on `/dev/shm`, which Docker always provides, because a
    missing target would answer `ENOENT` instead of the permission answer;
    `keyctl` asks for the thread keyring with create=1 for the same reason
    (the session keyring may not exist, `ENOKEY`).
  - `rapid.MakeFuzz` targets find many "interesting" inputs; the fuzzer
    minimizes each (default up to 60 s), so the exec counter can sit at 0/sec
    for several seconds. `-fuzztime` still ends the run.
- **Test layout.** Pure pieces (Dockerfile generator, context tar, stream
  parser, probe parser) are tested without a daemon, each with an oracle
  that is not the implementation: a constructive `rapid` generator for the
  stream and probe lines, a regexp for probe lines, `json.Valid` plus a number-preserving decoder
  for the stream (decoding into `float64` wrongly rejects valid JSON such as
  `1e400`; that input is a committed seed), and a Dockerfile read back through JSON arrays for the
  context. Mutating the code (accept unclean paths, ignore the `error` key,
  drop blank probe lines) made the matching property fail.
- Still unverified on Docker Desktop: the classic `ImageBuild` path, the
  saved-image layout, `arm64` cross-compilation running (it builds and is
  checked as an arm64 ELF, but not executed), and every probe row (Mac gate).
