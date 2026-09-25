# Knowledge index

Start here. Every item in `knowledge/` is listed below; keep this list current.
See `AGENTS.md` for how to add items.

| Item | What it covers |
|---|---|
| [local-services.md](local-services.md) | Running `services/` via the Makefile: port conflicts, the `.PHONY` pattern-rule trap, checking the Docker daemon |
| [sqlite-go.md](sqlite-go.md) | SQLite via modernc in Go: per-connection pragmas (use the DSN), WAL sidecar files, timing only the write |
| [postgres-go.md](postgres-go.md) | pgx: first-call statement preparation skews timings, multi-statement Exec limits, `returning` |
| [valkey-go.md](valkey-go.md) | go-redis: connect via `redis.ParseURL` from `CACHE_URL`, `redis.Nil` means miss, one key builder |
| [go-modules.md](go-modules.md) | One root module; `internal/lab` helpers; `make run-/analyze-/lab-<lesson>`; all-`// indirect` go.mod means tidy ran too early |
| [duckdb-analysis.md](duckdb-analysis.md) | `analyze.sql` idioms: loading CSVs, `.print` labels, `group by all`, `arg_max`, `filter`, p50/p95, alias reuse |
| [agent-instructions.md](agent-instructions.md) | `AGENTS.md` holds instructions, `CLAUDE.md` imports it with `@AGENTS.md`; why no symlinks or `/config` setting |
