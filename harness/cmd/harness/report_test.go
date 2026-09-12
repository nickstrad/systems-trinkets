package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"systems-trinkets/harness/results"
)

// runIDA and runIDB are fixed so "last" is predictable: run ids sort
// chronologically, so B is the last run.
const (
	runIDA = "20260912T150000Z-aaaa"
	runIDB = "20260912T160000Z-bbbb"
)

// fixtureRuns writes two tiny runs of the same pattern under <tmp>/runs and
// returns that directory. Run A passes; run B has a failing invariant and a
// failing test, so every query has something to show.
func fixtureRuns(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "runs")
	writeRun(t, root, runIDA, true)
	writeRun(t, root, runIDB, false)
	return root
}

// writeRun exports one run's five parquet files. ok=false makes
// TestIncrementConcurrent fail on INV-COUNTER-01.
func writeRun(t *testing.T, root, runID string, ok bool) {
	t.Helper()
	ctx := context.Background()
	started := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	if !ok {
		started = started.Add(time.Hour)
	}
	sink, err := results.Open(ctx, filepath.Join(root, runID), results.RunRow{
		RunID: runID, Pattern: "counter", Language: "go", Engine: "memory",
		Label: "fixture", TargetURL: "http://127.0.0.1:8080", StartedAt: started,
	})
	if err != nil {
		t.Fatalf("open sink: %v", err)
	}

	sink.Test(results.TestRow{RunID: runID, Test: "TestContract", Status: "pass", DurationNS: int64(3 * time.Millisecond)})
	status, errText := "pass", ""
	if !ok {
		status, errText = "fail", "FAIL INV-COUNTER-01: no lost update"
	}
	sink.Test(results.TestRow{RunID: runID, Test: "TestIncrementConcurrent", Status: status,
		DurationNS: int64(20 * time.Millisecond), Error: errText})

	sink.Check(results.CheckRow{RunID: runID, Test: "TestContract", InvariantID: "INV-COUNTER-03",
		OK: true, Message: "sequential correctness", DetailsJSON: `{"got":1}`, At: started})
	sink.Check(results.CheckRow{RunID: runID, Test: "TestIncrementConcurrent", InvariantID: "INV-COUNTER-01",
		OK: ok, Message: "no lost update", DetailsJSON: `{"got":95,"want":100}`, At: started})

	// Two setup samples (excluded from latency) and four real ones.
	sink.Sample(results.SampleRow{RunID: runID, Test: "TestContract", Phase: results.PhaseSetup, Method: "POST",
		PathTemplate: "/_reset", Status: 204, LatencyNS: 9e8, StartedAt: started})
	sink.Sample(results.SampleRow{RunID: runID, Test: "TestContract", Phase: results.PhaseSetup, Method: "GET",
		PathTemplate: "/healthz", Status: 200, LatencyNS: 9e8, StartedAt: started})
	sink.Sample(results.SampleRow{RunID: runID, Test: "TestContract", Method: "GET",
		PathTemplate: "/counters/{name}", Status: 200, LatencyNS: 1e6, StartedAt: started})
	for i, lat := range []int64{2e6, 4e6, 6e6} {
		s := results.SampleRow{RunID: runID, Test: "TestIncrementConcurrent", Method: "POST",
			PathTemplate: "/counters/{name}/incr", Status: 200, LatencyNS: lat, Seq: i, StartedAt: started}
		if !ok && i == 2 {
			s.Status, s.Err = 500, "boom"
		}
		sink.Sample(s)
	}

	sink.Metric(results.MetricRow{RunID: runID, Test: "TestIncrementConcurrent", Name: "incr_throughput",
		Value: 1234, Unit: "req/s", LabelsJSON: "{}", At: started})

	if err := sink.Close(ctx); err != nil {
		t.Fatalf("close sink: %v", err)
	}
}

// reportJSON runs one query through the report path and decodes the rows.
func reportJSON(t *testing.T, runsDir, query, run, pattern string) []map[string]any {
	t.Helper()
	var b bytes.Buffer
	if err := report(&b, runsDir, query, run, pattern, "json"); err != nil {
		t.Fatalf("report %s: %v", query, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(b.Bytes(), &rows); err != nil {
		t.Fatalf("report %s: invalid JSON %q: %v", query, b.String(), err)
	}
	return rows
}

func TestQuerySummary(t *testing.T) {
	rows := reportJSON(t, fixtureRuns(t), "summary", "last", "")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want one per test: %v", len(rows), rows)
	}
	if got := rows[0]["test"]; got != "TestContract" {
		t.Errorf("rows are not ordered by test: first is %v", got)
	}
	if got := rows[0]["duration_ms"]; got != float64(3) {
		t.Errorf("duration_ms = %v, want 3", got)
	}
	if got := rows[0]["samples"]; got != float64(3) {
		t.Errorf("samples = %v, want 3 (setup samples count here)", got)
	}
	c := rows[1]
	if c["status"] != "fail" || c["checks_failed"] != float64(1) || c["checks"] != float64(1) {
		t.Errorf("failing test row = %v", c)
	}
	if s, _ := c["error"].(string); !strings.Contains(s, "INV-COUNTER-01") {
		t.Errorf("error = %v, want the recorded failure text", c["error"])
	}
}

func TestQuerySummaryNamedRun(t *testing.T) {
	rows := reportJSON(t, fixtureRuns(t), "summary", runIDA, "")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r["status"] != "pass" {
			t.Errorf("run A should be all green: %v", r)
		}
	}
}

func TestQueryLatency(t *testing.T) {
	rows := reportJSON(t, fixtureRuns(t), "latency", "last", "")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want one per (test, method, path): %v", len(rows), rows)
	}
	if got := rows[0]["path_template"]; got != "/counters/{name}" {
		t.Errorf("rows are not ordered by path_template: first is %v", got)
	}
	incr := rows[1]
	if incr["n"] != float64(3) || incr["ok_2xx"] != float64(2) || incr["non_2xx"] != float64(1) {
		t.Errorf("incr counts = %v", incr)
	}
	if incr["errors"] != float64(1) {
		t.Errorf("errors = %v, want 1", incr["errors"])
	}
	if incr["max_ms"] != float64(6) {
		t.Errorf("max_ms = %v, want 6", incr["max_ms"])
	}
	if got := incr["p50_ms"]; got != float64(4) {
		t.Errorf("p50_ms = %v, want 4", got)
	}
	// The setup handshake (900ms) must not leak into the percentiles.
	for _, r := range rows {
		if p, _ := r["p99_ms"].(float64); p > 100 {
			t.Errorf("setup samples leaked into latency: %v", r)
		}
	}
}

func TestQueryChecks(t *testing.T) {
	runs := fixtureRuns(t)
	rows := reportJSON(t, runs, "checks", "last", "")
	if len(rows) != 1 {
		t.Fatalf("got %d failed checks, want 1: %v", len(rows), rows)
	}
	if rows[0]["invariant_id"] != "INV-COUNTER-01" {
		t.Errorf("row = %v", rows[0])
	}
	if s, _ := rows[0]["details"].(string); !strings.Contains(s, `"want":100`) {
		t.Errorf("details = %v, want the recorded JSON", rows[0]["details"])
	}
	if rows := reportJSON(t, runs, "checks", runIDA, ""); len(rows) != 0 {
		t.Errorf("run A has no failed checks, got %v", rows)
	}
}

func TestQueryCompare(t *testing.T) {
	rows := reportJSON(t, fixtureRuns(t), "compare", runIDA+","+runIDB, "")
	kinds := map[string]int{}
	byKey := map[string]map[string]any{}
	for _, r := range rows {
		kinds[r["kind"].(string)]++
		byKey[r["key"].(string)] = r
	}
	if kinds["check"] != 2 || kinds["latency"] != 2 {
		t.Fatalf("kinds = %v, want 2 checks and 2 endpoints: %v", kinds, rows)
	}
	inv := byKey["TestIncrementConcurrent / INV-COUNTER-01"]
	if inv == nil || inv["a"] != "ok" || inv["b"] != "FAIL" {
		t.Errorf("INV-COUNTER-01 compare row = %v", inv)
	}
	if lat := byKey["POST /counters/{name}/incr p99_ms"]; lat == nil || lat["a"] == nil || lat["b"] == nil {
		t.Errorf("latency compare row = %v", lat)
	}
}

func TestQueryCompareNeedsTwoRuns(t *testing.T) {
	err := report(&bytes.Buffer{}, fixtureRuns(t), "compare", "last", "", "box")
	if err == nil || !strings.Contains(err.Error(), "two runs") {
		t.Fatalf("err = %v, want a hint about --run a,b", err)
	}
}

func TestQueryHistory(t *testing.T) {
	runs := fixtureRuns(t)
	rows := reportJSON(t, runs, "history", "last", "counter")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want one per run: %v", len(rows), rows)
	}
	if rows[0]["run_id"] != runIDA || rows[1]["run_id"] != runIDB {
		t.Errorf("rows are not ordered by started_at: %v", rows)
	}
	if rows[0]["failed"] != float64(0) || rows[1]["failed"] != float64(1) {
		t.Errorf("failed counts = %v / %v", rows[0]["failed"], rows[1]["failed"])
	}
	if rows[1]["checks_failed"] != float64(1) {
		t.Errorf("checks_failed = %v, want 1", rows[1]["checks_failed"])
	}
	// Measured samples in run B: 1, 2, 4, 6 ms (setup excluded) → p99 ≈ 5.94.
	if p, _ := rows[1]["p99_ms"].(float64); p < 5.5 || p > 6 {
		t.Errorf("p99_ms = %v, want ~5.94 (setup traffic excluded)", rows[1]["p99_ms"])
	}
	if rows := reportJSON(t, runs, "history", "last", "fifo-queue"); len(rows) != 0 {
		t.Errorf("unknown pattern should be empty, got %v", rows)
	}
}

func TestQueryHistoryNeedsPattern(t *testing.T) {
	err := report(&bytes.Buffer{}, fixtureRuns(t), "history", "last", "", "box")
	if err == nil || !strings.Contains(err.Error(), "--pattern") {
		t.Fatalf("err = %v, want a hint about --pattern", err)
	}
}

func TestReportNoRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs")
	if err := report(&bytes.Buffer{}, dir, "summary", "last", "", "box"); err != errNoRuns {
		t.Fatalf("err = %v, want errNoRuns for a missing results dir", err)
	}
}

func TestReportUnknownRunAndQuery(t *testing.T) {
	runs := fixtureRuns(t)
	if err := report(&bytes.Buffer{}, runs, "summary", "nope", "", "box"); err == nil ||
		!strings.Contains(err.Error(), "unknown run") {
		t.Errorf("unknown run: err = %v", err)
	}
	if err := report(&bytes.Buffer{}, runs, "nosuchquery", "last", "", "box"); err == nil ||
		!strings.Contains(err.Error(), "no query") {
		t.Errorf("unknown query: err = %v", err)
	}
}

func TestResolveRun(t *testing.T) {
	ids := []string{runIDA, runIDB}
	for _, tc := range []struct{ in, want string }{
		{"last", runIDB}, {"", runIDB}, {"first", runIDA}, {runIDA, runIDA},
	} {
		got, err := resolveRun(tc.in, ids)
		if err != nil || got != tc.want {
			t.Errorf("resolveRun(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := resolveRun("missing", ids); err == nil {
		t.Error("resolveRun(missing): want an error")
	}
}
