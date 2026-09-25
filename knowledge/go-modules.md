# One Go module, many lessons: layout and gotchas

Applies to every Go lesson. The repo is one module
(`github.com/nickstrad/systems-trinkets`, root `go.mod`); each lesson is a
`package main` in `lessons/<name>/` with `main.go`, `analyze.sql`, and the
`measurements.csv` it writes. Run from the root: `make run-<name>`,
`make analyze-<name>`, or `make lab-<name>` for both. Changed 2026-09-25 from
one module per lesson.

- **Shared helpers live in `internal/lab`.** `lab.PostgresURL()` and
  `lab.ValkeyURL()` read `DATABASE_URL` / `CACHE_URL` with the local defaults
  (`postgres://trinkets:trinkets@localhost:5432/trinkets`,
  `redis://localhost:6379`); `lab.Check` panics on error; `lab.Ms` converts a
  duration to milliseconds; `lab.NewMeasurements(cols...)` creates
  `measurements.csv` in the current directory, `Write(fields...)` appends a
  row, `Close` flushes, checks, and closes. The package is standard-library
  only, so a lesson never compiles a driver it does not import.
- **Lessons run from their own directory.** The Makefile does `cd lessons/<name>`
  before `go run .`, because the CSV and `analyze.sql` use relative paths. Root
  commands like `go vet ./...` and `go build ./...` still cover every lesson.
- **One version per dependency.** All lessons share the root `go.sum`; pgx,
  go-redis, and modernc sqlite each have one pinned version.
- **Every dependency marked `// indirect` means tidy ran too early.** If
  `go mod tidy` ran before a `main.go` imported the package, the direct import
  lands in the indirect block and gopls warns "should be direct". Rerun
  `go mod tidy` at the root. `go mod tidy -diff` checks without changing
  anything: it exits non-zero when stale, zero when clean.
- **Adding a lesson:** create `lessons/<name>/main.go` and `analyze.sql`; the
  Makefile discovers directories under `lessons/` with `$(wildcard)`, so no
  Makefile edit is needed. A `main.ts` instead of `main.go` makes the target
  run `deno run -A main.ts`.

Verified 2026-09-25 on Go 1.26.4: `go vet ./...`, `go mod tidy -diff` (exit 0),
and `make lab-<name>` for all three lessons after the migration.
