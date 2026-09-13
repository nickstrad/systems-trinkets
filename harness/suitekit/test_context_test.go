package suitekit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"systems-trinkets/harness/results"
)

func fakeSUT(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /_reset", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /thing", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"v":7}`)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// withTarget points the package state at a fake SUT and a Buffer recorder
// for the duration of the test, standing in for Main.
func withTarget(t *testing.T, url string) *results.Buffer {
	buf := &results.Buffer{}
	prevT, prevR, prevID := target, recorder, runID
	target, recorder, runID = &Target{Pattern: "fake", Language: "go", Engine: "memory", URL: url}, buf, "run-test"
	t.Cleanup(func() { target, recorder, runID = prevT, prevR, prevID })
	return buf
}

func TestNewResetsAndRecordsTestRow(t *testing.T) {
	srv := fakeSUT(t)
	buf := withTarget(t, srv.URL)

	t.Run("inner", func(t *testing.T) {
		h := New(t)
		var out struct{ V int }
		h.Must(h.Get("/thing", nil, &out))
		if out.V != 7 {
			t.Fatalf("got %d", out.V)
		}
	})

	tests, _, samples, _ := buf.Snapshot()
	if len(tests) != 1 || tests[0].Status != "pass" || tests[0].Test != "TestNewResetsAndRecordsTestRow/inner" || tests[0].RunID != "run-test" {
		t.Fatalf("test rows: %+v", tests)
	}
	if len(samples) != 3 {
		t.Fatalf("want 3 samples (reset, healthz, thing), got %d: %+v", len(samples), samples)
	}
	if samples[0].PathTemplate != "/_reset" || samples[0].Phase != results.PhaseSetup || samples[2].Phase != "" {
		t.Errorf("setup phase labelling wrong: %+v", samples)
	}
}
