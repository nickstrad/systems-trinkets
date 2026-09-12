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
