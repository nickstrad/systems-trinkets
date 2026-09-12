package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		in         []string
		positional []string
		flags      []string
	}{
		{"flags after", []string{"SELECT 1", "--format", "json"}, []string{"SELECT 1"}, []string{"--format", "json"}},
		{"flags before", []string{"--format", "json", "SELECT 1"}, []string{"SELECT 1"}, []string{"--format", "json"}},
		{"equals form", []string{"-format=md", "SELECT 1"}, []string{"SELECT 1"}, []string{"-format=md"}},
		{"stdin dash", []string{"-", "--format", "json"}, []string{"-"}, []string{"--format", "json"}},
		{"help takes no value", []string{"-h", "foo"}, []string{"foo"}, []string{"-h"}},
		{"none", nil, nil, nil},
	} {
		pos, flags := splitArgs(tc.in)
		if !slices.Equal(pos, tc.positional) || !slices.Equal(flags, tc.flags) {
			t.Errorf("%s: splitArgs(%q) = %q, %q; want %q, %q", tc.name, tc.in, pos, flags, tc.positional, tc.flags)
		}
	}
}

func TestUsageListsEveryCommand(t *testing.T) {
	var b bytes.Buffer
	usage(&b)
	for _, cmd := range []string{"run", "report", "sql", "new-suite", "targets"} {
		if !strings.Contains(b.String(), "harness "+cmd) {
			t.Errorf("usage does not document %q:\n%s", cmd, b.String())
		}
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	if code := dispatch([]string{"nope"}); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if code := dispatch(nil); code != 0 {
		t.Errorf("no args should print usage and exit 0, got %d", code)
	}
}
