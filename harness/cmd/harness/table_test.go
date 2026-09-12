package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleTable() *Table {
	at := time.Date(2026, 9, 12, 15, 15, 4, 0, time.UTC)
	return &Table{
		Cols: []string{"test", "p99_ms", "n", "when", "error"},
		Rows: [][]any{
			{"TestContract", 1.2345, int64(7), at, nil},
			{"TestIncrementConcurrent", 12.0, int64(6400), at, "boom"},
		},
	}
}

func renderTable(t *testing.T, table *Table, format string) string {
	t.Helper()
	var b bytes.Buffer
	if err := table.Render(&b, format); err != nil {
		t.Fatalf("render %s: %v", format, err)
	}
	return b.String()
}

func TestRenderBoxAligns(t *testing.T) {
	out := renderTable(t, sampleTable(), "box")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + rule + 2 rows, got %d lines:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[1], strings.Repeat("-", len("TestIncrementConcurrent"))) {
		t.Errorf("rule should be as wide as the widest cell:\n%s", out)
	}
	// Numeric columns are right-aligned, so the two p99 values end in the same column.
	col := func(line, want string) int { return strings.Index(line, want) + len(want) }
	if a, b := col(lines[2], "1.23"), col(lines[3], "12.00"); a != b {
		t.Errorf("p99_ms not right-aligned: %d vs %d\n%s", a, b, out)
	}
	if !strings.Contains(lines[2], nullCell) {
		t.Errorf("NULL should render as %s:\n%s", nullCell, out)
	}
	if !strings.Contains(out, "2026-09-12T15:15:04Z") {
		t.Errorf("times should render as RFC3339:\n%s", out)
	}
	for i, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Errorf("line %d has trailing space: %q", i, line)
		}
	}
}

func TestRenderBoxEmpty(t *testing.T) {
	out := renderTable(t, &Table{Cols: []string{"test"}}, "box")
	if !strings.Contains(out, "(0 rows)") {
		t.Errorf("empty table should say so, got %q", out)
	}
}

func TestRenderMarkdown(t *testing.T) {
	out := renderTable(t, sampleTable(), "md")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + rule + 2 rows, got %d:\n%s", len(lines), out)
	}
	if want := "| test | p99_ms | n | when | error |"; lines[0] != want {
		t.Errorf("header = %q, want %q", lines[0], want)
	}
	if want := "| --- | ---: | ---: | --- | --- |"; lines[1] != want {
		t.Errorf("rule = %q, want %q", lines[1], want)
	}
	if !strings.Contains(lines[2], "| 1.23 |") {
		t.Errorf("floats want two decimals:\n%s", out)
	}
}

func TestRenderMarkdownEscapesPipes(t *testing.T) {
	out := renderTable(t, &Table{Cols: []string{"msg"}, Rows: [][]any{{"a|b"}}}, "md")
	if !strings.Contains(out, `a\|b`) {
		t.Errorf("pipe not escaped: %q", out)
	}
}

func TestRenderJSONKeepsColumnOrderAndTypes(t *testing.T) {
	out := renderTable(t, sampleTable(), "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if v := rows[0]["p99_ms"]; v != 1.2345 {
		t.Errorf("p99_ms = %v, want the unrounded 1.2345", v)
	}
	if v, ok := rows[0]["error"]; !ok || v != nil {
		t.Errorf("error = %v, want null", v)
	}
	if v := rows[1]["n"]; v != float64(6400) {
		t.Errorf("n = %v, want 6400", v)
	}
	// Column order survives: "test" must appear before "p99_ms" in the text.
	if strings.Index(out, `"test"`) > strings.Index(out, `"p99_ms"`) {
		t.Errorf("column order lost:\n%s", out)
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	if err := sampleTable().Render(&bytes.Buffer{}, "csv"); err == nil {
		t.Fatal("want an error for an unknown format")
	}
}
