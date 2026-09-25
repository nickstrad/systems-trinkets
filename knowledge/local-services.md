# Local services: gotchas

Applies when running or adding a service under `services/` (see `services/index.md`).

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
- **Docker daemon down.** `docker manifest inspect` can succeed while the daemon
  is stopped; `docker info` is the real check. Start Docker Desktop with
  `open -a Docker`.

Verified 2026-09-24: all three services reached healthy, S3 rejected a wrong
secret, and `make clean-<service>` removed containers and volumes.
