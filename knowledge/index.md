# Knowledge index

Start here. Every item in `knowledge/` is listed below; keep this list current.
See `AGENTS.md` for how to add items.

| Item | What it covers |
|---|---|
| [terminal-diagrams.md](terminal-diagrams.md) | Repo-local draw-visual skill for Claude/Codex, regenerable project diagrams, preview review, prerequisites and portability limits |
| [lesson-authoring.md](lesson-authoring.md) | `$create-lesson`, globally impact-ranked ideas, draft platform compositions, Other integration tasks and local instructions, ideas → planned → completed lifecycle, configured-software gate, platform research, glow guides, embedded k6 plans, and scratch verification |
| [performance-labs.md](performance-labs.md) | HTTP/k6 Make targets, the Deno runner, the `workload.ts` helpers (`request` tags, `assertResponse` body, `settings`, `everyVariant`, `optionsFor`), shared `analyze.sql` with the `tag()` macro and settings header, `perf.Warm`, how each of the five lessons puts its contrast in one run, isolated fixtures, CSV interpretation, why Make needs no `export` for settings, the idle-gap timing inflation, and verified smoke/load/failure workflows |
| [lesson-cores.md](lesson-cores.md) | Reusable core entry points for all five lessons, caller-owned dependencies, error/result semantics, HTTP adapter boundaries, and per-variant schema/warm-up needs the adapters discovered |
| [local-services.md](local-services.md) | Running `software/` via the Makefile: port conflicts, the `.PHONY` pattern-rule trap, checking the Docker daemon; the `services/` to `software/` rename and the `software.md` catalog; Apple `container` CLI start-before-kernel-set gotcha; per-image gotchas for the seven optional services (nats, etcd, registry, toxiproxy, temporal, openbao, pgbouncer) and the wal_level/keyspace flags |
| [sqlite-go.md](sqlite-go.md) | SQLite via modernc in Go: per-connection pragmas (use the DSN), WAL sidecar files, timing only the write |
| [postgres-go.md](postgres-go.md) | pgx: first-call statement preparation skews timings, multi-statement Exec limits, `$1` cannot be a table name, blocked `for update` re-checks the row, `returning` |
| [valkey-go.md](valkey-go.md) | go-redis: URL parsing/ping, `redis.Nil`, shared key builders; lease TTL validation, atomic token-checked renewal, and limits of ownership success counts |
| [go-modules.md](go-modules.md) | One root module, lessons in `lessons/go/`; `internal/lab` stdlib helpers plus `lab/postgres` and `lab/valkey` connect helpers; `make run-/analyze-/lab-<lesson>`, `make check`; all-`// indirect` go.mod means tidy ran too early |
| [duckdb-analysis.md](duckdb-analysis.md) | `analyze.sql` idioms: loading CSVs, `.print` labels (no apostrophes), `group by all`, `arg_max`, `filter`, p50/p95, alias reuse (not beside a scalar subquery), named-group ratios, mismatched group literals give NULL, recursive `unnest` of JSON arrays, macros; optional k6 skill and metric-sample CSV analysis |
| [agent-instructions.md](agent-instructions.md) | `AGENTS.md` holds instructions, `CLAUDE.md` imports it with `@AGENTS.md`; why no symlinks or `/config` setting |
| [seaweedfs-s3.md](seaweedfs-s3.md) | SeaweedFS via S3: `LastModified` is whole seconds (grace windows need +1 s), `HeadObject` missing is `NotFound`, `DeleteObjects` batches, list vs HEAD cost, non-idempotent `CreateBucket` |
| [mac-portability.md](mac-portability.md) | Docker + Go baseline for every idea and full project; KVM excluded, replacement slugs, container/broker designs and remaining validation |
