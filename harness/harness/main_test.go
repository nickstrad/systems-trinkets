package harness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"systems-trinkets/harness/results"
)

// runCounterSuite runs one test of the counter suite in a child `go test`
// with no target in the environment, so Main takes the InProcess path.
// extraEnv is appended after the HARNESS_* variables have been stripped.
func runCounterSuite(t *testing.T, extraEnv ...string) string {
	t.Helper()
	cmd := exec.Command("go", "test", "./suites/counter/", "-run", "TestContract$", "-count=1", "-v")
	cmd.Dir = ModuleRoot()
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HARNESS_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./suites/counter: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "running in-process") {
		t.Fatalf("suite did not run in-process:\n%s", out)
	}
	return string(out)
}

func TestInProcessWritesResultsOnlyWhenAsked(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go test")
	}
	runs := RunsDir()
	before, _ := os.ReadDir(runs)

	// Without HARNESS_RESULTS: the suite runs, nothing is recorded anywhere.
	runCounterSuite(t)
	after, _ := os.ReadDir(runs)
	if len(after) != len(before) {
		t.Fatalf("in-process run without HARNESS_RESULTS wrote to %s (%d → %d entries)", runs, len(before), len(after))
	}

	// With HARNESS_RESULTS: a full run directory with the synthesised target.
	root := t.TempDir()
	dir := filepath.Join(root, "run-in-process")
	runCounterSuite(t, EnvResults+"="+dir, EnvRunID+"=run-in-process")
	for _, table := range results.Tables {
		if _, err := os.Stat(filepath.Join(dir, table+".parquet")); err != nil {
			t.Errorf("missing %s.parquet: %v", table, err)
		}
	}
	db, err := results.Query(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var pattern, engine, label string
	var tests, checks int
	row := db.QueryRow(`SELECT r.pattern, r.engine, r.label,
		(SELECT count(*) FROM tests WHERE run_id = r.run_id),
		(SELECT count(*) FROM checks WHERE run_id = r.run_id AND ok)
		FROM runs r WHERE r.run_id = 'run-in-process'`)
	if err := row.Scan(&pattern, &engine, &label, &tests, &checks); err != nil {
		t.Fatal(err)
	}
	if pattern != "counter" || engine != "memory" || label != "in-process" || tests != 1 || checks == 0 {
		t.Errorf("run row: pattern=%q engine=%q label=%q tests=%d ok_checks=%d", pattern, engine, label, tests, checks)
	}
}
