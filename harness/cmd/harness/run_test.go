package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTargetEnvFromURL(t *testing.T) {
	env, err := targetEnv(&runFlags{url: "http://127.0.0.1:8080", language: "go", engine: "valkey", label: "v1 INCR"}, "counter")
	if err != nil {
		t.Fatalf("targetEnv: %v", err)
	}
	want := []string{
		"HARNESS_URL=http://127.0.0.1:8080",
		"HARNESS_PATTERN=counter",
		"HARNESS_LANGUAGE=go",
		"HARNESS_ENGINE=valkey",
		"HARNESS_LABEL=v1 INCR",
	}
	if !slices.Equal(env, want) {
		t.Errorf("env = %q, want %q", env, want)
	}
}

func TestTargetEnvFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "counter-go-valkey.toml")
	body := "pattern = \"counter\"\nlanguage = \"go\"\nengine = \"valkey\"\nurl = \"http://127.0.0.1:8080\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	env, err := targetEnv(&runFlags{target: path}, "counter")
	if err != nil {
		t.Fatalf("targetEnv: %v", err)
	}
	if len(env) != 1 || env[0] != "HARNESS_TARGET="+path {
		t.Errorf("env = %q, want an absolute HARNESS_TARGET", env)
	}
	if _, err := targetEnv(&runFlags{target: path}, "fifo-queue"); err == nil ||
		!strings.Contains(err.Error(), "not \"fifo-queue\"") {
		t.Errorf("pattern mismatch: err = %v", err)
	}
}

func TestTargetEnvRequiresExactlyOne(t *testing.T) {
	if _, err := targetEnv(&runFlags{}, "counter"); err == nil {
		t.Error("no target: want an error")
	}
	if _, err := targetEnv(&runFlags{url: "http://x", target: "y.toml"}, "counter"); err == nil ||
		!strings.Contains(err.Error(), "not both") {
		t.Error("both target and url: want an error")
	}
}

func TestRunRejectsUnknownSuite(t *testing.T) {
	if code := cmdRun([]string{"definitely-not-a-pattern", "--url", "http://127.0.0.1:9"}); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunTargetsBatch(t *testing.T) {
	dir := t.TempDir()
	write := func(name, pattern string) {
		t.Helper()
		body := "pattern = \"" + pattern + "\"\nurl = \"http://127.0.0.1:8080\"\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("counter-z.toml", "counter")
	write("counter-a.toml", "counter")
	write("queue-a.toml", "queue")
	f := &runFlags{allTargets: true, sutRef: "revision"}
	targets, err := runTargets(f, "counter", dir)
	if err != nil {
		t.Fatal(err)
	}
	var visited []string
	code := runSequence(targets, func(target runFlags) int {
		visited = append(visited, filepath.Base(target.target))
		if target.sutRef != "revision" {
			t.Error("lost SUT reference")
		}
		if len(visited) == 1 {
			return 7
		}
		return 1
	})
	if code != 7 || !slices.Equal(visited, []string{"counter-a.toml", "counter-z.toml"}) {
		t.Fatalf("batch code=%d visited=%v", code, visited)
	}
	write("counter-z.toml", "queue")
	if targets, err := runTargets(f, "counter", dir); err == nil || len(targets) != 0 {
		t.Fatalf("mismatched target must prevent the entire batch: %v, %v", targets, err)
	}
	if _, err := runTargets(f, "missing", dir); err == nil {
		t.Fatal("empty batch must fail")
	}
	for _, flags := range []*runFlags{{allTargets: true, url: "http://x"}, {allTargets: true, target: "x.toml"}} {
		if _, err := runTargets(flags, "counter", dir); err == nil {
			t.Fatal("conflicting target selection must fail")
		}
	}
}

func TestRunEnvironmentOverridesInheritedTarget(t *testing.T) {
	parent := []string{"PATH=/bin", "HARNESS_TARGET=stale.toml", "HARNESS_ENGINE=stale", "HARNESS_HEALTH_TIMEOUT=5s"}
	selected := []string{"HARNESS_URL=http://chosen", "HARNESS_PATTERN=counter"}
	got := runEnvironment(parent, selected)
	want := append([]string{"PATH=/bin", "HARNESS_HEALTH_TIMEOUT=5s"}, selected...)
	if !slices.Equal(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
}

// Exercise the actual CLI loop with a child that records its environment and
// fails its first target. No SUT is needed to prove forwarding and continuation.
func TestRunAllTargetsContinuesAfterChildFailure(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"targets", "suites/counter", "bin"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"a", "b"} {
		body := "pattern = \"counter\"\nurl = \"http://127.0.0.1:9\"\n"
		if err := os.WriteFile(filepath.Join(root, "targets", "counter-"+label+".toml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script := `#!/bin/sh
printf '%s|%s|%s|%s\n' "$HARNESS_TARGET" "$HARNESS_RUN_ID" "$HARNESS_RESULTS" "$*" >> "$AUDIT_LEDGER"
case "$HARNESS_TARGET" in *counter-a.toml) exit 7;; esac
exit 0
`
	if err := os.WriteFile(filepath.Join(root, "bin", "go"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("PATH", filepath.Join(root, "bin"))
	ledger := filepath.Join(root, "ledger")
	t.Setenv("AUDIT_LEDGER", ledger)
	t.Setenv("HARNESS_TARGET", "inherited.toml")
	if code := cmdRun([]string{"--all-targets", "counter", "--", "-run", "TestContract$"}); code != 7 {
		t.Fatalf("exit=%d, want first target's 7", code)
	}
	data, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(rows) != 2 {
		t.Fatalf("targets executed: %s", data)
	}
	first, second := strings.Split(rows[0], "|"), strings.Split(rows[1], "|")
	if first[1] == second[1] || first[2] == second[2] {
		t.Fatalf("runs share IDs or directories: %s", data)
	}
	for i, row := range [][]string{first, second} {
		want := filepath.Join(root, "targets", "counter-"+[]string{"a", "b"}[i]+".toml")
		if row[0] != want || !strings.Contains(row[3], "-run TestContract$") {
			t.Errorf("target/flags not forwarded: %v", row)
		}
	}
}
