// Package results defines the row types every run produces and the Sink that
// collects them in memory and exports them to Parquet at the end of a run.
//
// Rows are flat structs with UTC timestamps and JSON already marshalled to
// strings, so the Parquet writer can be swapped without touching callers.
// See docs/architecture.md for the results pipeline design.
package results

import (
	"crypto/rand"
	"encoding/json"
	"time"
)

// RunRow is written once per run, at Close, with FinishedAt filled in.
type RunRow struct {
	RunID      string
	Pattern    string
	Language   string
	Engine     string
	Label      string
	TargetURL  string
	HarnessSHA string
	SUTRef     string
	GoVersion  string
	Host       string
	StartedAt  time.Time
	FinishedAt time.Time
}

// TestRow is one Go test (including subtests). Status is pass|fail|skip.
type TestRow struct {
	RunID      string
	Test       string
	Status     string
	Error      string
	DurationNS int64
}

// CheckRow is one evaluation of one invariant inside one test.
type CheckRow struct {
	RunID       string
	Test        string
	InvariantID string
	Message     string
	DetailsJSON string
	OK          bool
	At          time.Time
}

// SampleRow is one HTTP request observed by httpclient.
type SampleRow struct {
	RunID        string
	Test         string
	Phase        string
	Method       string
	PathTemplate string
	Err          string
	Worker       int
	Seq          int
	Status       int
	LatencyNS    int64
	StartedAt    time.Time
}

// MetricRow is a free-form number recorded by a test.
type MetricRow struct {
	RunID      string
	Test       string
	Name       string
	Unit       string
	LabelsJSON string
	Value      float64
	At         time.Time
}

// Recorder is what tests, httpclient and check write to. Sink implements it; Discard
// and Buffer are for unit tests, so packages like httpclient never import DuckDB.
type Recorder interface {
	Test(TestRow)
	Check(CheckRow)
	Sample(SampleRow)
	Metric(MetricRow)
}

// Discard drops every row.
var Discard Recorder = discard{}

type discard struct{}

func (discard) Test(TestRow)     {}
func (discard) Check(CheckRow)   {}
func (discard) Sample(SampleRow) {}
func (discard) Metric(MetricRow) {}

// JSON marshals v for a *JSON field; nil becomes "{}" and errors become an
// error object rather than failing the caller.
func JSON(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(map[string]string{"_marshal_error": err.Error()})
	}
	return string(b)
}

// PhaseSetup labels the samples suitekit.New makes while resetting and
// health-checking the SUT. Query exposes samples_measured, which excludes it.
const PhaseSetup = "setup"

// Head keeps a details slice small for reports: up to 20 items verbatim,
// otherwise a count plus the first 20.
func Head[T any](v []T) any {
	if len(v) <= 20 {
		return v
	}
	return map[string]any{"count": len(v), "first": v[:20]}
}

// NewRunID returns a sortable id: 20260912T151504Z-ab3f.
func NewRunID() string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	var b [4]byte
	_, _ = rand.Read(b[:])
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + string(b[:])
}
