# DuckDB analysis scripts: idioms

Applies to a lesson's `analyze.sql`, run with `duckdb < analyze.sql` over the
`measurements.csv` its `main.go` or `main.ts` writes (see `lessons/*/*/analyze.sql`).

- **Load:** `create table measurements as from 'measurements.csv';` DuckDB
  detects the header and types. The script runs in memory, so `or replace` is
  unneeded. A table beats a view: the CSV is read and typed once.
- **`group by all` / `order by all`:** group by every non-aggregate selected
  column, sort by all columns left to right. No repeated key lists.
- **Final value of a series:** `arg_max(value, (event_id, attempt))` takes the
  value from the last row by order. `max(value)` only works while the value
  never goes down.
- **Counts by condition:** `count(*) filter (where applied = 0)` rather than
  arithmetic like `count(*) - sum(applied)`.
- **Latency:** `median(x)` for p50, `quantile_cont(x, 0.95)` for p95. Group by
  outcome (e.g. `mode, result`) so failed/timed-out samples do not mix with
  successful ones.
- **Label each table:** put `.print 'What the next table shows'` before every
  query. It is a DuckDB CLI dot command (works with `duckdb < analyze.sql`) and
  prints a plain line, unlike `select '...' as log`, which prints a one-cell
  table.
- **Reuse an alias in the same `select`:** a later column may refer to an
  earlier alias, e.g. `count(*) as total, count(*) filter (...) as hits,
  round(100.0 * hits / total, 1) as hit_rate_pct`. No CTE is needed just to
  name an aggregate once (`lessons/go/cache-aside/analyze.sql`).

Verified 2026-09-24: both lessons' `analyze.sql` ran in DuckDB with these forms.
- **Compare two named groups in one row:** `avg(x) filter (where mode = 'a')
  as a_ms, avg(x) filter (where mode = 'b') as b_ms, round(a_ms / b_ms, 1)`.
  Naming the groups keeps the ratio meaning a/b; `max(avg) / min(avg)` over a
  CTE only means that while `a` happens to be the slower one
  (`lessons/go/background-job-queue/analyze.sql`).

Verified 2026-09-25: alias reuse ran in `lessons/go/cache-aside/analyze.sql`;
`.print` labels ran in all three lessons' `analyze.sql`.
Verified 2026-09-26: the named-group ratio ran in
`lessons/go/background-job-queue/analyze.sql` via `make analyze-background-job-queue`.
Verified 2026-09-27: the named-group ratio and `.print` labels ran in the
first Deno lesson, `lessons/deno/cross-store-failure/analyze.sql`.
