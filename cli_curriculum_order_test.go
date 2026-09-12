package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI exercises the same top-level argument parsing as the binary while
// keeping each test database isolated from the tracked database.
func runCLI(t *testing.T, dbPath string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "stdout")
	errPath := filepath.Join(t.TempDir(), "stderr")
	out, openErr := os.Create(outPath)
	if openErr != nil {
		t.Fatal(openErr)
	}
	errOut, openErr := os.Create(errPath)
	if openErr != nil {
		out.Close()
		t.Fatal(openErr)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, errOut
	err = run(append([]string{"--db", dbPath}, args...))
	os.Stdout, os.Stderr = oldOut, oldErr
	if closeErr := out.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if closeErr := errOut.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	outBytes, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	errBytes, readErr := os.ReadFile(errPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(outBytes), string(errBytes), err
}

func mustRunCLI(t *testing.T, dbPath string, args ...string) string {
	t.Helper()
	out, stderr, err := runCLI(t, dbPath, args...)
	if err != nil {
		t.Fatalf("trinkets %v: %v\nstderr:\n%s\nstdout:\n%s", args, err, stderr, out)
	}
	return out
}

func TestCLICurriculumOrderViewsAndFlags(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cli.db")
	mustRunCLI(t, dbPath, "pattern", "add", "--slug", "first", "--name", "First", "--family", "state", "--curriculum-order", "1")
	mustRunCLI(t, dbPath, "pattern", "add", "--slug", "second", "--name", "Second", "--family", "state", "--curriculum-order", "2")
	mustRunCLI(t, dbPath, "pattern", "add", "--slug", "a-custom", "--name", "A custom", "--family", "custom")
	mustRunCLI(t, dbPath, "pattern", "add", "--slug", "z-custom", "--name", "Z custom", "--family", "custom")
	mustRunCLI(t, dbPath, "pattern", "add", "--slug", "explicit-null", "--name", "Explicit null", "--family", "custom", "--clear-curriculum-order")

	out := mustRunCLI(t, dbPath, "pattern", "list", "--json")
	var patterns []Pattern
	if err := json.Unmarshal([]byte(out), &patterns); err != nil {
		t.Fatalf("pattern list JSON: %v\n%s", err, out)
	}
	wantSlugs := []string{"first", "second", "a-custom", "explicit-null", "z-custom"}
	if len(patterns) != len(wantSlugs) {
		t.Fatalf("pattern count = %d, want %d", len(patterns), len(wantSlugs))
	}
	for i, want := range wantSlugs {
		if patterns[i].Slug != want {
			t.Errorf("pattern[%d].Slug = %q, want %q", i, patterns[i].Slug, want)
		}
	}
	if patterns[0].CurriculumOrder == nil || *patterns[0].CurriculumOrder != 1 ||
		patterns[1].CurriculumOrder == nil || *patterns[1].CurriculumOrder != 2 {
		t.Fatalf("ordered pattern values = %v, %v", patterns[0].CurriculumOrder, patterns[1].CurriculumOrder)
	}
	for _, p := range patterns[2:] {
		if p.CurriculumOrder != nil {
			t.Errorf("%s curriculum order = %v, want null", p.Slug, p.CurriculumOrder)
		}
	}

	out = mustRunCLI(t, dbPath, "pattern", "list", "--family", "custom")
	if !strings.Contains(out, "ORDER") || strings.Index(out, "a-custom") > strings.Index(out, "z-custom") {
		t.Fatalf("family list does not show ordered custom rows:\n%s", out)
	}

	// An edit that names another field keeps the existing curriculum position.
	mustRunCLI(t, dbPath, "pattern", "edit", "second", "--notes", "preserve order")
	out = mustRunCLI(t, dbPath, "pattern", "show", "second", "--json")
	var shown struct {
		Pattern Pattern `json:"pattern"`
	}
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("pattern show JSON: %v\n%s", err, out)
	}
	if shown.Pattern.CurriculumOrder == nil || *shown.Pattern.CurriculumOrder != 2 || shown.Pattern.Notes != "preserve order" {
		t.Fatalf("omitted order was not preserved: %#v", shown.Pattern)
	}

	// Setting and clearing are both explicit edits; an empty edit still fails.
	mustRunCLI(t, dbPath, "pattern", "edit", "second", "--curriculum-order", "4")
	mustRunCLI(t, dbPath, "pattern", "edit", "second", "--clear-curriculum-order")
	if _, _, err := runCLI(t, dbPath, "pattern", "edit", "second"); err == nil || !strings.Contains(err.Error(), "nothing to update") {
		t.Fatalf("empty edit error = %v, want nothing to update", err)
	}
	out = mustRunCLI(t, dbPath, "pattern", "show", "second")
	if !strings.Contains(out, "Order:") || !strings.Contains(out, "-") {
		t.Fatalf("show does not expose cleared order:\n%s", out)
	}

	// Both add and edit reject an ambiguous set/clear request and invalid values.
	mustRunCLI(t, dbPath, "pattern", "edit", "first", "--notes", "stable")
	for _, args := range [][]string{
		{"pattern", "add", "--slug", "conflict", "--name", "Conflict", "--curriculum-order", "3", "--clear-curriculum-order"},
		{"pattern", "edit", "first", "--curriculum-order", "3", "--clear-curriculum-order"},
		{"pattern", "add", "--slug", "zero", "--name", "Zero", "--curriculum-order", "0"},
		{"pattern", "add", "--slug", "negative", "--name", "Negative", "--curriculum-order", "-1"},
		{"pattern", "add", "--slug", "fractional", "--name", "Fractional", "--curriculum-order", "1.5"},
		{"pattern", "add", "--slug", "nonnumeric", "--name", "Nonnumeric", "--curriculum-order", "one"},
		{"pattern", "add", "--slug", "overflow", "--name", "Overflow", "--curriculum-order", "999999999999999999999999999"},
		{"pattern", "add", "--slug", "duplicate", "--name", "Duplicate", "--curriculum-order", "1"},
	} {
		if _, _, err := runCLI(t, dbPath, args...); err == nil {
			t.Fatalf("trinkets %v succeeded, want validation error", args)
		}
	}
	for _, value := range []string{"0", "-1", "1.5", "one", "999999999999999999999999"} {
		if _, _, err := runCLI(t, dbPath, "pattern", "edit", "first", "--curriculum-order", value); err == nil {
			t.Fatalf("invalid edit order %q succeeded", value)
		}
	}
	out = mustRunCLI(t, dbPath, "pattern", "show", "first", "--json")
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("pattern show JSON after invalid edits: %v\n%s", err, out)
	}
	if shown.Pattern.CurriculumOrder == nil || *shown.Pattern.CurriculumOrder != 1 || shown.Pattern.Notes != "stable" {
		t.Fatalf("failed edit changed existing fields: %#v", shown.Pattern)
	}
	if _, err := getPatternFromCLI(t, dbPath, "conflict"); err == nil {
		t.Fatal("mutually exclusive add created a pattern")
	}

	// Matrix JSON carries the same nullable order and follows the list order,
	// including after matrix assembly and family filtering.
	mustRunCLI(t, dbPath, "engine", "add", "--slug", "sqlite", "--name", "SQLite")
	mustRunCLI(t, dbPath, "approach", "add", "--pattern", "first", "--engine", "sqlite", "--title", "transaction")
	out = mustRunCLI(t, dbPath, "matrix", "--json")
	var matrix []matrixRow
	if err := json.Unmarshal([]byte(out), &matrix); err != nil {
		t.Fatalf("matrix JSON: %v\n%s", err, out)
	}
	if len(matrix) != len(wantSlugs) {
		t.Fatalf("matrix count = %d, want %d", len(matrix), len(wantSlugs))
	}
	for i, want := range []string{"first", "a-custom", "explicit-null", "z-custom", "second"} {
		if matrix[i].Pattern != want {
			t.Errorf("matrix[%d].Pattern = %q, want %q", i, matrix[i].Pattern, want)
		}
	}
	if matrix[0].CurriculumOrder == nil || *matrix[0].CurriculumOrder != 1 || matrix[3].CurriculumOrder != nil {
		t.Fatalf("matrix order fields = %v, %v", matrix[0].CurriculumOrder, matrix[3].CurriculumOrder)
	}
	out = mustRunCLI(t, dbPath, "matrix", "--family", "custom")
	if !strings.Contains(out, "ORDER") || strings.Index(out, "a-custom") > strings.Index(out, "z-custom") {
		t.Fatalf("family matrix does not show ordered rows:\n%s", out)
	}
}

func getPatternFromCLI(t *testing.T, dbPath, slug string) (Pattern, error) {
	t.Helper()
	db, err := openDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	return getPattern(db, slug)
}
