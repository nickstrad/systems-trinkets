# Knowledge index

Start here. Every item in `knowledge/` is listed below; keep this list current.
See `AGENTS.md` for how to add items.

| Item | What it covers |
|---|---|
| [performance-labs.md](performance-labs.md) | HTTP/k6 Make targets, the Deno runner, TypeScript k6 workloads typed via `/k6`, shared `analyze.sql`, settings reported by `/health`, isolated fixtures, CSV interpretation, custom tags in `extra_tags` and `read_json` of settings/domain files, which lessons predate the per-variant skill shape, why Make needs no `export` for settings, and verified smoke/failure workflows |
| [lesson-cores.md](lesson-cores.md) | Reusable core entry points for all six lessons, caller-owned dependencies, error/result semantics, and HTTP adapter boundaries |
| [local-services.md](local-services.md) | Running `services/` via the Makefile: port conflicts, the `.PHONY` pattern-rule trap, checking the Docker daemon |
| [sqlite-go.md](sqlite-go.md) | SQLite via modernc in Go: per-connection pragmas (use the DSN), WAL sidecar files, timing only the write |
| [postgres-go.md](postgres-go.md) | pgx: first-call statement preparation skews timings, multi-statement Exec limits, `$1` cannot be a table name, blocked `for update` re-checks the row, `returning` |
| [valkey-go.md](valkey-go.md) | go-redis: `valkey.Connect` parses `CACHE_URL` and pings, `redis.Nil` means miss, one key builder |
| [go-modules.md](go-modules.md) | One root module, lessons in `lessons/go/`; `internal/lab` stdlib helpers plus `lab/postgres` and `lab/valkey` connect helpers; `make run-/analyze-/lab-<lesson>`, `make check`; all-`// indirect` go.mod means tidy ran too early |
| [duckdb-analysis.md](duckdb-analysis.md) | `analyze.sql` idioms: loading CSVs, `.print` labels, `group by all`, `arg_max`, `filter`, p50/p95, alias reuse, named-group ratios, mismatched group literals give NULL; optional k6 skill and metric-sample CSV analysis |
| [agent-instructions.md](agent-instructions.md) | `AGENTS.md` holds instructions, `CLAUDE.md` imports it with `@AGENTS.md`; why no symlinks or `/config` setting |
| [seaweedfs-s3.md](seaweedfs-s3.md) | SeaweedFS via S3: `LastModified` is whole seconds (grace windows need +1 s), `HeadObject` missing is `NotFound`, `DeleteObjects` batches, list vs HEAD cost, non-idempotent `CreateBucket` |
| [deno-lessons.md](deno-lessons.md) | Root `deno.json` workspace (k6 type mappings, one lockfile) with `lessons/deno/` as the member import map and shared `lab/` helpers (measurements, postgres, valkey, seaweedfs), `@ts-types` for untyped npm packages, `deno check`, VS Code `enablePaths` |
