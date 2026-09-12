package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"systems-trinkets/harness/harness"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// slugRE is what a pattern slug may look like; it becomes a directory name, a
// Go package name and part of an invariant id.
var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// suiteData is what the scaffold templates are rendered with.
type suiteData struct {
	Pattern   string // fifo-queue
	Package   string // fifo_queue
	InvPrefix string // FIFO-QUEUE, so ids read INV-FIFO-QUEUE-01
	Date      string // 2026-09-12
}

// cmdNewSuite scaffolds suites/<pattern>/{CONTRACT.md,INVARIANTS.md,
// <pattern>_test.go}. It never overwrites an existing file: the docs are
// agreed with the user and hand-edited.
func cmdNewSuite(args []string) int {
	fs := flag.NewFlagSet("new-suite", flag.ContinueOnError)
	fs.Usage = func() { usageNewSuite(fs.Output()) }
	dir := fs.String("dir", "", "directory holding suites (default <module>/suites)")
	pos, flags := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "harness new-suite: expected one pattern slug, e.g. `harness new-suite fifo-queue`")
		return 2
	}
	pattern := pos[0]
	if !slugRE.MatchString(pattern) {
		fmt.Fprintf(os.Stderr, "harness new-suite: bad slug %q (lowercase words joined by -, e.g. fifo-queue)\n", pattern)
		return 2
	}

	root := moduleDir(*dir, harness.Path("suites"))
	written, err := scaffoldSuite(filepath.Join(root, pattern), pattern)
	for _, p := range written {
		fmt.Println("wrote", p)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness new-suite:", err)
		return 1
	}
	fmt.Printf("\nnext: agree CONTRACT.md and INVARIANTS.md with the user (test-plan.md §8), then write tests\n")
	return 0
}

// scaffoldSuite renders the three files into dir, returning the paths written
// even when it stops on an error.
func scaffoldSuite(dir, pattern string) ([]string, error) {
	data := suiteData{
		Pattern:   pattern,
		Package:   strings.ReplaceAll(pattern, "-", "_"),
		InvPrefix: strings.ToUpper(pattern),
		Date:      time.Now().Format("2006-01-02"),
	}
	files := []struct{ name, tmpl string }{
		{"CONTRACT.md", "contract.md.tmpl"},
		{"INVARIANTS.md", "invariants.md.tmpl"},
		{pattern + "_test.go", "suite_test.go.tmpl"},
	}
	tmpls, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	var written []string
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			return written, fmt.Errorf("%s already exists; refusing to overwrite", path)
		}
		if err != nil {
			return written, fmt.Errorf("create %s: %w", path, err)
		}
		err = tmpls.ExecuteTemplate(out, f.tmpl, data)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return written, fmt.Errorf("render %s: %w", path, err)
		}
		written = append(written, path)
	}
	return written, nil
}
