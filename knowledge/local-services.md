# Local software: gotchas

Applies when running or adding a service under `software/` (see `software/software.md`).

- **Port already allocated.** `make up-<service>` fails with
  `Bind for 127.0.0.1:<port> failed: port is already allocated` when another
  container holds the port. Find it with
  `docker ps --format '{{.Names}}\t{{.Ports}}'`. Containers from the old,
  deleted `harness/infra/` compose files (project `infra`, restart
  `unless-stopped`) come back whenever Docker starts. Either remove them or
  override the port, e.g. `POSTGRES_PORT=15432 make up-postgres`.
- **Makefile pattern rules vs `.PHONY`.** GNU make skips implicit (pattern)
  rules for targets listed in `.PHONY`, so `up-%:` with `.PHONY: up-postgres`
  prints "Nothing to be done". The root `Makefile` therefore generates explicit
  per-service rules with `define` + `$(foreach ... $(eval ...))`. Adding a
  service only needs a `compose_file_<name>` line and the name in `SERVICES`.
  The per-lesson `run-`/`analyze-`/`lab-` rules use the same trick over
  `$(wildcard lessons/go/*/main.go)`, running `go run .` in the lesson directory.
- **Docker daemon down.** `docker manifest inspect` can succeed while the daemon
  is stopped; `docker info` is the real check. Start Docker Desktop with
  `open -a Docker`.

Verified 2026-09-24: all three services reached healthy, S3 rejected a wrong
secret, and `make clean-<service>` removed containers and volumes.

## Folder and catalog (2026-09-28)

The compose files moved from `services/` to `software/`; Make target names
(`up-postgres` and so on) did not change. `software/software.md` is now one
table of every piece of software worth a lesson, with a `Configured` column
saying whether the repo runs or imports it yet and a `Setup` column with the
connection string or the way to add it. Add a row (or flip one to `yes`) when
adding a service; `software/AGENTS.md` has the rules.

## Apple `container` CLI

Installed with `brew install container` (formula, not cask; 1.4.1, no admin
password). It is not a brew service and is kept stopped by default because
Docker Desktop remains the main runtime.

- **Order matters.** The brew caveat lists `container system kernel set
  --recommended` before `container system start`, but `kernel set` fails with
  `XPC connection error: Connection invalid` until the API server is running.
  Run `container system start` first, then `kernel set`, then use it. Verified
  2026-09-28: the kernel install itself was not run, so the first
  `container run` still needs it.
- Stop with `container system stop` when done; `container system status`
  shows whether it is up.

## Optional services added 2026-09-28 (verified healthy, then stopped)

Each new service is its own Compose project; cross-service traffic goes
through the host as `host.docker.internal:<port>` with
`extra_hosts: ["host.docker.internal:host-gateway"]` so it also works on
Linux. Per-service notes live in `software/software.md`; the gotchas:

- **Minimal images and healthchecks.** `nats:2` and
  `ghcr.io/shopify/toxiproxy` have no shell. Use `nats:2-alpine` (has `wget`)
  and, for Toxiproxy, `CMD` form with the full path `/toxiproxy-cli`, which is
  not on `PATH`. `CMD-SHELL` cannot work in a shell-less image.
- **etcd peer URLs.** `--initial-advertise-peer-urls=http://localhost:2380`
  fails the `--initial-cluster` check because etcd resolves `localhost` to the
  container IP first. Use the literal `127.0.0.1` for both peer flags.
- **Temporal volume ownership.** `temporalio/temporal` runs as uid 1000; a
  fresh named volume at `/data` is root-owned and `--db-filename` cannot write
  it. Mount the volume over `/home/temporal` instead. Only patch tags exist
  (`1.9.1`, no `1` or `1.9`). Healthcheck:
  `temporal operator cluster health --address 127.0.0.1:7233`.
- **OpenBao dev token.** Set `BAO_DEV_ROOT_TOKEN_ID` as an env var; the image
  entrypoint builds `bao server -dev ...` itself, so passing
  `-dev-root-token-id` in `command` duplicates the flag. No `IPC_LOCK` needed
  in dev mode. `docker exec ... bao ...` needs `-e BAO_TOKEN=trinkets`.
- **PgBouncer against PostgreSQL 18.** `AUTH_TYPE=scram-sha-256` with the
  plaintext password in `userlist.txt` (the edoburu entrypoint writes it);
  the default `md5` fails. Its healthcheck (`pg_isready`) only proves
  PgBouncer listens: with PostgreSQL down it stays healthy and queries hang,
  and after PostgreSQL returns the first query fails for about 15 s
  (`server_login_retry`). Every probe logs a login line, so the interval is
  30 s with a 1 s `start_interval`.
- **Registry deletes.** Deleting a manifest leaves the repository in
  `/v2/_catalog`; only garbage collection or `make clean-registry` clears it.
  Host port is 5050 because macOS AirPlay holds 5000.
- **Toxiproxy proxies are in memory** and must listen on `0.0.0.0:<port>`
  within the published 22000-22009 range; recreate them after `down`/`up`.
- **Config flags on the core services.** `postgres.compose.yaml` now passes
  `-c wal_level=logical` and `valkey.compose.yaml` passes
  `--notify-keyspace-events Ex`. Verified 2026-09-28: `show wal_level`
  returned `logical`, `config get notify-keyspace-events` returned `xE`, and
  existing tables survived the container recreate.
