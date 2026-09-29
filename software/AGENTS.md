# software/

Software that lessons build on. `software.md` is the catalog: one table of every
piece of software worth a lesson, its use case, and whether this repo is
configured to use it. The compose files here define the backing services that
are configured; each has its own file so a lesson can start only what it needs.

## Local runtime boundary

Every lesson idea and complete project must run locally on macOS using Docker
Desktop/Compose and Go. Deno/TypeScript is reserved for k6 tooling; lesson cores,
runners, HTTP adapters and platform components are Go only. Existing DuckDB
analysis and k6 load tooling stay in scope. Run Linux-specific harnesses inside Docker. Exclude Firecracker and
all KVM-dependent tools, nested virtualization, separately managed Linux VMs,
Apple container and alternate VM runtimes from project paths for now.
Containerd/nerdctl is allowed when useful to a specific lesson, with its Linux
engine running locally on the Mac; Docker remains the backing-service default.
Excluded tools may appear in provider research only.
Use ARM64-compatible images on Apple silicon. Keep unimplemented Docker harnesses
blocked until configured; portability alone does not make a lesson ready.

## Rules

- One compose file per service. Use a bare `<service>.compose.yaml` file when the
  service needs nothing else; use a `<service>/` folder when it needs extra files
  (config, init scripts).
- Each compose file sets `name: trinkets-<service>` so services are separate
  Compose projects and can be started, stopped, and cleaned independently.
- Bind ports to `127.0.0.1` only, and make each host port overridable with a
  `<SERVICE>_PORT`-style variable (`${POSTGRES_PORT:-5432}`); list it in the
  port table in `software.md`. Credentials are fixed dev values; never reuse
  them anywhere real.
- The `Setup` column in `software.md` gives the full connection string for each
  configured service, username and password included (or says there is no
  auth). Always quote it that way; never redact it.
- Redis and Valkey both run (ports 6380 and 6379). Pick per lesson by feature
  set, and default to Redis when it does not matter; see "Choosing Redis or
  Valkey" in `software.md`.
- Every service has a healthcheck so `make up-<service>` returns only when it is ready.
- Adding a service: add its compose file, add a `compose_file_<service>` line and
  its name to `SERVICES` in the root `Makefile`, flip its row in `software.md`
  to `Configured: yes` with the connection details in `Setup`, and add a short
  section under "Running configured software".
- Adding software that needs no service (a library, a brew tool): flip its row
  to `yes` once a helper or lesson actually uses it.

## Commands (run from repo root)

    make up-<service>      # start and wait for healthy
    make down-<service>    # stop, keep data
    make clean-<service>   # stop and delete data volumes
    make logs-<service>
    make ps
