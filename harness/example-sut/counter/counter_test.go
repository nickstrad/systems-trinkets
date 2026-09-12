package counter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The correct store and the decorators that must change nothing observable
// while the process is alive all pass the conformance test.

func TestMemoryStore(t *testing.T) { StoreTest(t, NewMemory) }

func TestSlowStoreConforms(t *testing.T) {
	StoreTest(t, func() Store { return Slow(NewMemory(), 100*time.Microsecond) })
}

func TestWriteBehindStoreConforms(t *testing.T) {
	StoreTest(t, func() Store { return WriteBehind(NewMemory(), 5*time.Millisecond) })
}

// Each bug breaks exactly what it claims.

func TestLostUpdateBreaksOnlyConcurrency(t *testing.T) {
	ctx := context.Background()
	s := LostUpdate(NewMemory())

	// Sequentially it is indistinguishable from a correct store.
	if v, _ := s.Incr(ctx, "a", 1); v != 1 {
		t.Fatalf("Incr = %d, want 1", v)
	}
	if v, _ := s.Incr(ctx, "a", 5); v != 6 {
		t.Fatalf("Incr = %d, want 6", v)
	}
	mustGet(t, s, "a", 6)
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	mustGet(t, s, "a", 0)

	// Concurrently it loses increments.
	const attempts = 5
	for range attempts {
		_ = s.Reset(ctx)
		concurrentIncr(t, s, "race", 32, 200)
		if v, _ := s.Get(ctx, "race"); v < 32*200 {
			return
		}
	}
	t.Fatalf("lost-update bug did not lose any increments in %d attempts", attempts)
}

func TestDropResetBreaksOnlyReset(t *testing.T) {
	ctx := context.Background()
	s := DropReset(NewMemory())

	concurrentIncr(t, s, "a", 8, 50)
	mustGet(t, s, "a", 400)
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	mustGet(t, s, "a", 400) // the bug: reset reported ok and did nothing
	if err := s.Del(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, s, "a", 0) // Del still works
}

func TestWriteBehindLosesUnflushedWrites(t *testing.T) {
	ctx := context.Background()
	under := NewMemory()
	s := WriteBehind(under, time.Hour)  // never flushes on its own during the test
	t.Cleanup(func() { _ = s.Close() }) // idempotent; stops the flusher even if a check below fails
	w := s.(*writeBehind)

	if v, _ := s.Incr(ctx, "a", 3); v != 3 {
		t.Fatalf("Incr = %d, want 3", v)
	}
	mustGet(t, s, "a", 3)     // the client sees its write…
	mustGet(t, under, "a", 0) // …the store has not: a kill here loses it (INV-06)

	w.flushNow()
	mustGet(t, under, "a", 3)

	// Increments after a flush start from the shadow, not the store.
	if v, _ := s.Incr(ctx, "a", 1); v != 4 {
		t.Fatalf("Incr after flush = %d, want 4", v)
	}
	mustGet(t, under, "a", 3)

	// Del and Reset write through immediately; a later Incr reloads from the store.
	if err := s.Del(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, under, "a", 0)
	mustGet(t, s, "a", 0)
	_ = under.Set(ctx, "b", 10)
	if v, _ := s.Incr(ctx, "b", 1); v != 11 {
		t.Fatalf("Incr on a name only the store knows = %d, want 11", v)
	}
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	mustGet(t, under, "b", 0)
	mustGet(t, s, "b", 0)

	// Close flushes what is pending: only a kill loses data.
	_, _ = s.Incr(ctx, "c", 7)
	mustGet(t, under, "c", 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	mustGet(t, under, "c", 7)
}

func TestSlowAddsLatency(t *testing.T) {
	const d = 20 * time.Millisecond
	s := Slow(NewMemory(), d)
	start := time.Now()
	if _, err := s.Incr(context.Background(), "a", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := time.Since(start); got < 2*d {
		t.Fatalf("two ops took %v, want ≥ %v", got, 2*d)
	}
}

// The handler: the contract over the memory store.

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode %T: %v", v, err)
	}
	return v
}

func postIncr(t *testing.T, base, name string, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(base+"/counters/"+name+"/incr", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST incr: %v", err)
	}
	return resp
}

func getCounter(t *testing.T, base, name string) counterResponse {
	t.Helper()
	resp, err := http.Get(base + "/counters/" + name)
	if err != nil {
		t.Fatalf("GET counter: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", name, resp.StatusCode)
	}
	return decodeJSON[counterResponse](t, resp)
}

func do(t *testing.T, method, url string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	resp.Body.Close()
	return resp
}

func TestHandlerContract(t *testing.T) {
	srv := httptest.NewServer(NewHandler(NewMemory()))
	defer srv.Close()

	// Default delta increments by 1; explicit delta adds; GET agrees.
	resp := postIncr(t, srv.URL, "a", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("incr default: status = %d, want 200", resp.StatusCode)
	}
	if v := decodeJSON[counterResponse](t, resp); v.Name != "a" || v.Value != 1 {
		t.Fatalf("incr default: got %+v, want {a 1}", v)
	}
	if v := decodeJSON[counterResponse](t, postIncr(t, srv.URL, "a", `{"delta":5}`)); v.Value != 6 {
		t.Fatalf("incr delta=5: got %+v, want value 6", v)
	}
	if v := getCounter(t, srv.URL, "a"); v.Value != 6 {
		t.Fatalf("get a: got %+v, want value 6", v)
	}
	if v := getCounter(t, srv.URL, "never-touched"); v.Value != 0 {
		t.Fatalf("get unknown: got %+v, want value 0", v)
	}

	// 400s: bad delta, malformed body, bad name — with an error body.
	for _, c := range []struct{ name, body string }{{"a", `{"delta":0}`}, {"a", `{"delta":`}, {"bad name!", ""}} {
		resp := postIncr(t, srv.URL, c.name, c.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("incr %q %q: status = %d, want 400", c.name, c.body, resp.StatusCode)
		}
		if e := decodeJSON[errorResponse](t, resp); e.Error == "" {
			t.Fatalf("incr %q %q: empty error message", c.name, c.body)
		}
	}
	if v := getCounter(t, srv.URL, "a"); v.Value != 6 {
		t.Fatalf("rejected requests changed the value: %+v", v)
	}

	// DELETE removes one counter only.
	postIncr(t, srv.URL, "b", "").Body.Close()
	if resp := do(t, http.MethodDelete, srv.URL+"/counters/a"); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete a: status = %d, want 204", resp.StatusCode)
	}
	if v := getCounter(t, srv.URL, "a"); v.Value != 0 {
		t.Fatalf("get a after delete: got %+v, want 0", v)
	}
	if v := getCounter(t, srv.URL, "b"); v.Value != 1 {
		t.Fatalf("get b after deleting a: got %+v, want 1", v)
	}

	// Reset wipes everything.
	if resp := do(t, http.MethodPost, srv.URL+"/_reset"); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: status = %d, want 204", resp.StatusCode)
	}
	if v := getCounter(t, srv.URL, "b"); v.Value != 0 {
		t.Fatalf("get after reset: got %+v, want 0", v)
	}

	// Healthz and a wrong method.
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if h := decodeJSON[map[string]bool](t, resp); !h["ok"] {
		t.Fatalf("healthz: got %v, want ok=true", h)
	}
	if resp := do(t, http.MethodGet, srv.URL+"/counters/a/incr"); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET incr path: status = %d, want 405", resp.StatusCode)
	}
}

// failing is a Store whose every call fails, to exercise the 5xx paths.
type failing struct{ Store }

func (failing) Incr(context.Context, string, int64) (int64, error) { return 0, errFail }
func (failing) Ping(context.Context) error                         { return errFail }

var errFail = context.DeadlineExceeded

func TestHandlerStoreErrors(t *testing.T) {
	srv := httptest.NewServer(NewHandler(failing{NewMemory()}))
	defer srv.Close()
	if resp := postIncr(t, srv.URL, "a", ""); resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("incr on failing store: status = %d, want 500", resp.StatusCode)
	}
	if resp := do(t, http.MethodGet, srv.URL+"/healthz"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("healthz on failing store: status = %d, want 503", resp.StatusCode)
	}
}
