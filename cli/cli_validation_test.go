package main

import (
	"path/filepath"
	"testing"
)

func TestCLIInvalidOrderNeverChangesRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.db")
	mustRunCLI(t, path, "pattern", "add", "--slug", "keep", "--name", "Keep", "--curriculum-order", "1", "--notes", "authored", "--invariant", "original")
	mustRunCLI(t, path, "pattern", "add", "--slug", "occupied", "--name", "Occupied", "--curriculum-order", "2")
	before := mustRunCLI(t, path, "pattern", "list", "--json")
	for _, value := range []string{"0", "-3", "1.5", "hello", "9223372036854775808", "2"} {
		for _, args := range [][]string{
			{"pattern", "add", "--slug", "bad", "--name", "Bad", "--curriculum-order", value},
			{"pattern", "edit", "keep", "--curriculum-order", value, "--notes", "must not replace"},
		} {
			if _, _, err := runCLI(t, path, args...); err == nil {
				t.Fatalf("accepted %v", args)
			}
			if after := mustRunCLI(t, path, "pattern", "list", "--json"); after != before {
				t.Fatalf("invalid request changed data: %v", args)
			}
		}
	}
	mustRunCLI(t, path, "pattern", "edit", "keep", "--clear-curriculum-order")
	p, err := getPatternFromCLI(t, path, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if p.CurriculumOrder != nil || p.Notes != "authored" || len(p.Invariants) != 1 || p.Invariants[0] != "original" {
		t.Fatalf("clear discarded omitted fields: %+v", p)
	}
}
