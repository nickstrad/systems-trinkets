# software/

Software that lessons build on. `software.md` is the catalog: one table of every
piece of software worth a lesson, its use case, and whether this repo is
configured to use it. The compose files here define the backing services that
are configured; each has its own file so a lesson can start only what it needs.

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
