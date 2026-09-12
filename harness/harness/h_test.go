package harness

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestLoadTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "counter-go-valkey.toml")
	os.WriteFile(path, []byte(`
pattern  = "counter"
language = "go"
engine   = "valkey"
url      = "http://127.0.0.1:8080"
label    = "v1 INCR"
[expect]
p99_ms = 20
`), 0o644)
	tg, err := LoadTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if tg.Pattern != "counter" || tg.Engine != "valkey" || tg.Expect["p99_ms"] != 20 {
		t.Errorf("%+v", tg)
	}

	bad := filepath.Join(dir, "bad.toml")
	os.WriteFile(bad, []byte(`pattern = "x"`+"\n"+`urll = "typo"`), 0o644)
	if _, err := LoadTarget(bad); err == nil {
		t.Error("want error for unknown key / missing url")
	}
}

func TestTargetEnvRoundTrip(t *testing.T) {
	t.Setenv(EnvTarget, "")
	for _, kv := range TargetEnv(Target{URL: "http://127.0.0.1:9999/", Pattern: "counter", Engine: "valkey"}) {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	tg, ok, err := TargetFromEnv()
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	want := Target{URL: "http://127.0.0.1:9999", Pattern: "counter", Language: "?", Engine: "valkey"}
	if !reflect.DeepEqual(tg, want) {
		t.Errorf("got %+v, want %+v", tg, want)
	}
	t.Setenv(EnvURL, "")
	if _, ok, _ := TargetFromEnv(); ok {
		t.Error("want ok=false with nothing set")
	}
}
