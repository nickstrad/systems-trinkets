package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"systems-trinkets/harness/suitekit"
)

// cmdTargets lists targets/*.toml with the dimensions each one describes.
// A broken file is reported in place of its row rather than aborting the list.
func cmdTargets(args []string) int {
	fs := flag.NewFlagSet("targets", flag.ContinueOnError)
	fs.Usage = func() { usageTargets(fs.Output()) }
	dir := fs.String("dir", "", "directory holding target files (default <module>/targets)")
	format := fs.String("format", "box", "output format: box|md|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root := moduleDir(*dir, suitekit.Path("targets"))

	paths, err := filepath.Glob(filepath.Join(root, "*.toml")) // sorted
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness targets:", err)
		return 1
	}
	if len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "harness: no targets in %s — write one (see docs/architecture.md (Targets and process lifecycle))\n", root)
		return 0
	}

	table := &Table{Cols: []string{"file", "pattern", "language", "engine", "url", "label"}}
	for _, p := range paths {
		name := filepath.Base(p)
		t, err := suitekit.LoadTarget(p)
		if err != nil {
			table.Rows = append(table.Rows, []any{name, "!", err.Error(), nil, nil, nil})
			continue
		}
		table.Rows = append(table.Rows, []any{name, t.Pattern, t.Language, t.Engine, t.URL, t.Label})
	}
	if err := table.Render(os.Stdout, *format); err != nil {
		fmt.Fprintln(os.Stderr, "harness targets:", err)
		return 1
	}
	return 0
}
