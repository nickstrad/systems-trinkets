# services/

Local backing services that lessons in this repo depend on. Each service is defined
by its own Docker Compose file so a lesson can start only what it needs.

## Rules

- One compose file per service. Use a bare `<service>.compose.yaml` file when the
  service needs nothing else; use a `<service>/` folder when it needs extra files
  (config, init scripts).
- Each compose file sets `name: trinkets-<service>` so services are separate
  Compose projects and can be started, stopped, and cleaned independently.
- Bind ports to `127.0.0.1` only, and make each host port overridable with a
  `<SERVICE>_PORT`-style variable (`${POSTGRES_PORT:-5432}`); list it in `index.md`. Credentials are fixed dev values; never reuse
  them anywhere real.
- The `Connect` column in `index.md` gives the full connection string, username
  and password included (or says there is no auth). Always quote it that way;
  never redact it.
- Every service has a healthcheck so `make up-<service>` returns only when it is ready.
- Adding a service: add its compose file, add a `compose_file_<service>` line and
  its name to `SERVICES` in the root `Makefile`, and add a row to `index.md`.

## Commands (run from repo root)

    make up-<service>      # start and wait for healthy
    make down-<service>    # stop, keep data
    make clean-<service>   # stop and delete data volumes
    make logs-<service>
    make ps

See `index.md` for what each service provides and how to connect.
