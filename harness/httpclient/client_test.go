package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"systems-trinkets/harness/results"
)

// newTestClient serves handler for the test's lifetime and returns a client
// bound to it plus the buffer it records into.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *results.Buffer) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	buf := &results.Buffer{}
	return New(srv.URL, buf, "run-1", "test-1"), buf
}

// okClient is newTestClient with a handler that always answers 200.
func okClient(t *testing.T) (*Client, *results.Buffer) {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestPathTemplating(t *testing.T) {
	var gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
	})

	resp, err := c.Get(context.Background(), "/counters/{name}/incr", map[string]string{"name": "a b/c"}, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	want := "/counters/a%20b%2Fc/incr"
	if gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestJSONBodyAndOut(t *testing.T) {
	type reqBody struct {
		N int `json:"n"`
	}
	type respBody struct {
		Sum int `json:"sum"`
	}

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("Accept = %q, want application/json", accept)
		}
		var rb reqBody
		if err := json.NewDecoder(r.Body).Decode(&rb); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{Sum: rb.N + 1})
	})

	var out respBody
	resp, err := c.Post(context.Background(), "/add", nil, reqBody{N: 41}, &out)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if out.Sum != 42 {
		t.Fatalf("out.Sum = %d, want 42", out.Sum)
	}
}

func TestNon2xxNotError(t *testing.T) {
	c, buf := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })

	counter := &Counter{}
	ctx := WithCounter(context.Background(), counter)

	resp, err := c.Get(ctx, "/missing", nil, nil)
	if err != nil {
		t.Fatalf("Get returned error for non-2xx: %v", err)
	}
	if resp.Status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Status)
	}

	_, _, samples, _ := buf.Snapshot()
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	if samples[0].Status != http.StatusNotFound {
		t.Fatalf("sample status = %d, want 404", samples[0].Status)
	}
	if samples[0].Err != "" {
		t.Fatalf("sample err = %q, want empty", samples[0].Err)
	}
	if counter.Non2xx.Load() != 1 {
		t.Fatalf("Non2xx = %d, want 1", counter.Non2xx.Load())
	}
	if counter.Total.Load() != 1 {
		t.Fatalf("Total = %d, want 1", counter.Total.Load())
	}
}

func TestTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // closed before use: connection refused

	buf := &results.Buffer{}
	c := New(url, buf, "run-1", "test-1")

	counter := &Counter{}
	ctx := WithCounter(context.Background(), counter)

	resp, err := c.Get(ctx, "/x", nil, nil)
	if err == nil {
		t.Fatalf("expected transport error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil resp on error, got %+v", resp)
	}

	_, _, samples, _ := buf.Snapshot()
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	if samples[0].Status != 0 {
		t.Fatalf("sample status = %d, want 0", samples[0].Status)
	}
	if samples[0].Err == "" {
		t.Fatalf("sample err = empty, want set")
	}
	if counter.Errors.Load() != 1 {
		t.Fatalf("Errors = %d, want 1", counter.Errors.Load())
	}
}

func TestOneSamplePerRequestConcurrent(t *testing.T) {
	c, buf := okClient(t)

	const workers, total = 8, 100
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			ctx := WithWorker(context.Background(), w)
			for i := w; i < total; i += workers {
				if _, err := c.Get(ctx, "/ping", nil, nil); err != nil {
					t.Errorf("Get: %v", err)
				}
			}
		})
	}
	wg.Wait()

	_, _, samples, _ := buf.Snapshot()
	if len(samples) != total {
		t.Fatalf("samples = %d, want %d", len(samples), total)
	}

	seqsByWorker := map[int][]int{}
	for _, s := range samples {
		seqsByWorker[s.Worker] = append(seqsByWorker[s.Worker], s.Seq)
	}
	for worker, seqs := range seqsByWorker {
		slices.Sort(seqs)
		for i, s := range seqs {
			if s != i {
				t.Fatalf("worker %d: seqs not contiguous 0..%d: %v", worker, len(seqs)-1, seqs)
			}
		}
	}
}

func TestWithTimeoutFires(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	_, err := c.Get(context.Background(), "/slow", nil, nil, WithTimeout(20*time.Millisecond))
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func TestPhaseAndWorkerInSample(t *testing.T) {
	c, buf := okClient(t)

	ctx := WithWorker(context.Background(), 3)
	ctx = WithPhase(ctx, "warmup")

	if _, err := c.Get(ctx, "/x", nil, nil); err != nil {
		t.Fatalf("Get: %v", err)
	}

	_, _, samples, _ := buf.Snapshot()
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	if samples[0].Worker != 3 {
		t.Fatalf("Worker = %d, want 3", samples[0].Worker)
	}
	if samples[0].Phase != "warmup" {
		t.Fatalf("Phase = %q, want warmup", samples[0].Phase)
	}
}

func TestMissingPathArgErrors(t *testing.T) {
	c, buf := okClient(t)

	resp, err := c.Get(context.Background(), "/counters/{name}/incr", nil, nil)
	if err == nil {
		t.Fatalf("expected error for missing path arg, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil resp, got %+v", resp)
	}

	_, _, samples, _ := buf.Snapshot()
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	if samples[0].Err == "" {
		t.Fatalf("sample err = empty, want set")
	}
}
