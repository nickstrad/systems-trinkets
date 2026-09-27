# Knowledge index

Start here. Every item in `knowledge/` is listed below; keep this list current.
See `AGENTS.md` for how to add items.

| Item | What it covers |
|---|---|
| [lesson-cores.md](lesson-cores.md) | Reusable core entry points for all five lessons, caller-owned dependencies, error/result semantics, and HTTP adapter boundaries |
| [local-services.md](local-services.md) | Running `services/` via the Makefile: port conflicts, the `.PHONY` pattern-rule trap, checking the Docker daemon |
| [sqlite-go.md](sqlite-go.md) | SQLite via modernc in Go: per-connection pragmas (use the DSN), WAL sidecar files, timing only the write |
| [postgres-go.md](postgres-go.md) | pgx: first-call statement preparation skews timings, multi-statement Exec limits, `$1` cannot be a table name, blocked `for update` re-checks the row, `returning` |
| [valkey-go.md](valkey-go.md) | go-redis: `valkey.Connect` parses `CACHE_URL` and pings, `redis.Nil` means miss, one key builder |
| [go-modules.md](go-modules.md) | One root module, lessons in `lessons/go/`; `internal/lab` stdlib helpers plus `lab/postgres` and `lab/valkey` connect helpers; `make run-/analyze-/lab-<lesson>`, `make check`; all-`// indirect` go.mod means tidy ran too early |
| [duckdb-analysis.md](duckdb-analysis.md) | `analyze.sql` idioms: loading CSVs, `.print` labels, `group by all`, `arg_max`, `filter`, p50/p95, alias reuse, named-group ratios; optional k6 skill and metric-sample CSV analysis |
| [agent-instructions.md](agent-instructions.md) | `AGENTS.md` holds instructions, `CLAUDE.md` imports it with `@AGENTS.md`; why no symlinks or `/config` setting |
| [seaweedfs-s3.md](seaweedfs-s3.md) | SeaweedFS via S3: `LastModified` is whole seconds (grace windows need +1 s), `HeadObject` missing is `NotFound`, `DeleteObjects` batches, list vs HEAD cost, non-idempotent `CreateBucket` |
| [deno-lessons.md](deno-lessons.md) | Deno lessons in `lessons/deno/`: one `deno.json` import map with the shared `lab/` helpers (measurements, postgres, valkey, seaweedfs), `@ts-types` for untyped npm packages, `deno check`, VS Code `enablePaths` |
