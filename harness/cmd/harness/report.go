package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"systems-trinkets/harness/harness"
	"systems-trinkets/harness/results"
)

// errNoRuns means results/runs holds no run directories yet; callers report it
// as an empty state, not a failure.
var errNoRuns = errors.New("no runs yet")

// outputFlags are the flags report and sql share.
type outputFlags struct {
	format  string
	runsDir string
}

func addOutputFlags(fs *flag.FlagSet) *outputFlags {
	o := &outputFlags{}
	fs.StringVar(&o.format, "format", "box", "output format: box|md|json")
	fs.StringVar(&o.runsDir, "runs-dir", "", "directory holding run directories (default <module>/results/runs)")
	return o
}

// dir resolves --runs-dir, defaulting to <module root>/results/runs.
func (o *outputFlags) dir() string { return moduleDir(o.runsDir, harness.RunsDir()) }

// moduleDir returns override when set, else the module-relative default.
func moduleDir(override, def string) string {
	if override != "" {
		return override
	}
	return def
}

// reportErr prints err the way every query-backed command does: no runs yet
// is an empty state (exit 0), anything else is a failure (exit 1).
func reportErr(cmd, runsDir string, err error) int {
	if errors.Is(err, errNoRuns) {
		fmt.Fprintf(os.Stderr, "harness: no runs yet in %s — run `harness run <pattern> --url http://…` first\n", runsDir)
		return 0
	}
	fmt.Fprintf(os.Stderr, "harness %s: %v\n", cmd, err)
	return 1
}

func cmdReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.Usage = func() { usageReport(fs.Output()) }
	var (
		run     = fs.String("run", "last", "run id, \"last\", or \"a,b\" for --query compare")
		query   = fs.String("query", "summary", "query template: "+strings.Join(queryNames(), "|"))
		pattern = fs.String("pattern", "", "pattern slug (required by --query history)")
	)
	out := addOutputFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "harness report: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if err := report(os.Stdout, out.dir(), *query, *run, *pattern, out.format); err != nil {
		return reportErr("report", out.dir(), err)
	}
	return 0
}

// report runs one named query template against the parquet views under
// runsDir and writes the result to w.
func report(w io.Writer, runsDir, query, run, pattern, format string) error {
	ids, err := runIDs(runsDir)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errNoRuns
	}
	stmt, err := loadQuery(queriesDir(), query)
	if err != nil {
		return err
	}
	params, err := queryParams(stmt, run, pattern, ids)
	if err != nil {
		return fmt.Errorf("query %s: %w", query, err)
	}
	if err := queryAndRender(w, runsDir, stmt, format, params...); err != nil {
		return fmt.Errorf("query %s: %w", query, err)
	}
	return nil
}

// queryAndRender runs stmt over the parquet views under runsDir and writes the
// rows to w. Shared by report and sql; errNoRuns when there is nothing yet.
func queryAndRender(w io.Writer, runsDir, stmt, format string, params ...any) error {
	ids, err := runIDs(runsDir)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errNoRuns
	}
	ctx := context.Background()
	db, err := results.Query(ctx, runsDir)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, stmt, params...)
	if err != nil {
		return fmt.Errorf("duckdb: %w", err)
	}
	defer rows.Close()
	table, err := ScanTable(rows)
	if err != nil {
		return err
	}
	return table.Render(w, format)
}

// queryParams works out which named parameters this template uses and binds
// them: $run_id from --run (resolving "last"), $run_a/$run_b from --run a,b,
// and $pattern from --pattern. DuckDB ignores parameters a statement does not
// reference, but an unsupplied one is still an error, hence the needs check.
func queryParams(tmpl, run, pattern string, ids []string) ([]any, error) {
	var vals []any
	needs := func(name string) bool { return strings.Contains(tmpl, "$"+name) }

	if needs("run_a") || needs("run_b") {
		a, b, ok := strings.Cut(run, ",")
		if !ok {
			return nil, errors.New("needs two runs: --run <run_a>,<run_b>")
		}
		ra, err := resolveRun(strings.TrimSpace(a), ids)
		if err != nil {
			return nil, err
		}
		rb, err := resolveRun(strings.TrimSpace(b), ids)
		if err != nil {
			return nil, err
		}
		vals = append(vals, sql.Named("run_a", ra), sql.Named("run_b", rb))
	}
	if needs("run_id") {
		r, err := resolveRun(run, ids)
		if err != nil {
			return nil, err
		}
		vals = append(vals, sql.Named("run_id", r))
	}
	if needs("pattern") {
		if pattern == "" {
			return nil, errors.New("needs --pattern <slug>")
		}
		vals = append(vals, sql.Named("pattern", pattern))
	}
	return vals, nil
}

// resolveRun maps "last" to the lexically greatest run id (run ids sort
// chronologically) and checks that a named run exists.
func resolveRun(run string, ids []string) (string, error) {
	switch run {
	case "", "last":
		return ids[len(ids)-1], nil
	case "first":
		return ids[0], nil
	}
	if slices.Contains(ids, run) {
		return run, nil
	}
	return "", fmt.Errorf("unknown run %q (have %s)", run, strings.Join(ids, ", "))
}

// runIDs lists the run directories under runsDir (os.ReadDir sorts them, so
// they come back chronological). A missing directory is an empty list, not an
// error.
func runIDs(runsDir string) ([]string, error) {
	entries, err := os.ReadDir(runsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", runsDir, err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// queriesDir is where the SQL templates live.
func queriesDir() string { return harness.Path("queries") }

// queryNames lists the templates in queriesDir, for help and error text.
func queryNames() []string {
	paths, _ := filepath.Glob(filepath.Join(queriesDir(), "*.sql"))
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, strings.TrimSuffix(filepath.Base(p), ".sql"))
	}
	return names
}

// loadQuery reads queries/<name>.sql.
func loadQuery(dir, name string) (string, error) {
	if strings.ContainsAny(name, `/\.`) {
		return "", fmt.Errorf("bad query name %q", name)
	}
	path := filepath.Join(dir, name+".sql")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("no query %q: %s not found (have %s)", name, path, strings.Join(queryNames(), ", "))
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}

func cmdSQL(args []string) int {
	fs := flag.NewFlagSet("sql", flag.ContinueOnError)
	fs.Usage = func() { usageSQL(fs.Output()) }
	out := addOutputFlags(fs)
	pos, flags := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, `harness sql: expected one SQL argument (or "-" to read stdin)`)
		return 2
	}
	stmt := pos[0]
	if stmt == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "harness sql: read stdin:", err)
			return 1
		}
		stmt = string(b)
	}
	if strings.TrimSpace(stmt) == "" {
		fmt.Fprintln(os.Stderr, "harness sql: empty statement")
		return 2
	}
	if err := queryAndRender(os.Stdout, out.dir(), stmt, out.format); err != nil {
		return reportErr("sql", out.dir(), err)
	}
	return 0
}
