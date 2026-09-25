# Knowledge index

Start here. Every item in `knowledge/` is listed below; keep this list current.
See `AGENTS.md` for how to add items.

| Item | What it covers |
|---|---|
| [local-services.md](local-services.md) | Running `services/` via the Makefile: port conflicts, the `.PHONY` pattern-rule trap, checking the Docker daemon |
| [sqlite-go.md](sqlite-go.md) | SQLite via modernc in Go: per-connection pragmas (use the DSN), WAL sidecar files, timing only the write |
| [postgres-go.md](postgres-go.md) | pgx: first-call statement preparation skews timings, multi-statement Exec limits, `returning` |
| [duckdb-analysis.md](duckdb-analysis.md) | `analyze.sql` idioms: loading CSVs, `group by all`, `arg_max`, `filter`, p50/p95 |
| [agent-instructions.md](agent-instructions.md) | `AGENTS.md` holds instructions, `CLAUDE.md` imports it with `@AGENTS.md`; why no symlinks or `/config` setting |
