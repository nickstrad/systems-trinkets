package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// decodeJSON reads and decodes a response body, failing the test on any error.
func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode %T: %v", v, err)
	}
	return v
}

func decodeCounter(t *testing.T, resp *http.Response) counterResponse {
	return decodeJSON[counterResponse](t, resp)
}
func decodeError(t *testing.T, resp *http.Response) errorResponse {
	return decodeJSON[errorResponse](t, resp)
}

func postIncr(t *testing.T, base, name string, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(base+"/counters/"+name+"/incr", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST incr: %v", err)
	}
	return resp
}

// reset posts /_reset and asserts the 204 the contract promises.
func reset(t *testing.T, base string) {
	t.Helper()
	resp, err := http.Post(base+"/_reset", "application/json", nil)
	if err != nil {
		t.Fatalf("POST reset: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: status = %d, want 204", resp.StatusCode)
	}
}

func getCounter(t *testing.T, base, name string) *http.Response {
	t.Helper()
	resp, err := http.Get(base + "/counters/" + name)
	if err != nil {
		t.Fatalf("GET counter: %v", err)
	}
	return resp
}

func TestContractSmoke(t *testing.T) {
	srv := httptest.NewServer(newServer("none"))
	defer srv.Close()

	// Default delta increments by 1.
	resp := postIncr(t, srv.URL, "a", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("incr default: status = %d, want 200", resp.StatusCode)
	}
	if v := decodeCounter(t, resp); v.Name != "a" || v.Value != 1 {
		t.Fatalf("incr default: got %+v, want {a 1}", v)
	}

	// Explicit delta.
	resp = postIncr(t, srv.URL, "a", `{"delta":5}`)
	if v := decodeCounter(t, resp); v.Value != 6 {
		t.Fatalf("incr delta=5: got %+v, want value 6", v)
	}

	// GET reflects the same value.
	resp = getCounter(t, srv.URL, "a")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get a: status = %d, want 200", resp.StatusCode)
	}
	if v := decodeCounter(t, resp); v.Value != 6 {
		t.Fatalf("get a: got %+v, want value 6", v)
	}

	// Unknown counter reads as 0.
	resp = getCounter(t, srv.URL, "never-touched")
	if v := decodeCounter(t, resp); v.Value != 0 {
		t.Fatalf("get unknown: got %+v, want value 0", v)
	}

	// Bad delta (< 1) is a 400 with an error body.
	resp = postIncr(t, srv.URL, "a", `{"delta":0}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("incr delta=0: status = %d, want 400", resp.StatusCode)
	}
	if e := decodeError(t, resp); e.Error == "" {
		t.Fatalf("incr delta=0: empty error message")
	}

	// Malformed JSON body is a 400.
	resp = postIncr(t, srv.URL, "a", `{"delta":`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("incr malformed json: status = %d, want 400", resp.StatusCode)
	}

	// Bad name is a 400.
	resp = postIncr(t, srv.URL, "bad name!", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("incr bad name: status = %d, want 400", resp.StatusCode)
	}

	// DELETE removes one counter only.
	postIncr(t, srv.URL, "b", "").Body.Close()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/counters/a", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE a: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete a: status = %d, want 204", resp.StatusCode)
	}
	if v := decodeCounter(t, getCounter(t, srv.URL, "a")); v.Value != 0 {
		t.Fatalf("get a after delete: got %+v, want value 0", v)
	}
	if v := decodeCounter(t, getCounter(t, srv.URL, "b")); v.Value != 1 {
		t.Fatalf("get b after deleting a: got %+v, want value 1", v)
	}
	postIncr(t, srv.URL, "a", `{"delta":6}`).Body.Close()

	// Reset wipes everything.
	reset(t, srv.URL)
	resp = getCounter(t, srv.URL, "a")
	if v := decodeCounter(t, resp); v.Value != 0 {
		t.Fatalf("get after reset: got %+v, want value 0", v)
	}

	// Healthz.
	resp, err = http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET healthz: %v", err)
	}
	defer resp.Body.Close()
	var health struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("decode healthz: %v", err)
	}
	if !health.OK {
		t.Fatalf("healthz: got %+v, want ok=true", health)
	}

	// Wrong method on a known path is 405.
	resp, err = http.Get(srv.URL + "/counters/a/incr")
	if err != nil {
		t.Fatalf("GET incr path: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET incr path: status = %d, want 405", resp.StatusCode)
	}
}

const (
	numWorkers = 32
	numIncrPer = 200
	totalIncrs = numWorkers * numIncrPer
)

// concurrentIncr fires numWorkers goroutines each doing numIncrPer increments
// of delta 1 against name, and returns every returned post-increment value.
func concurrentIncr(t *testing.T, base, name string) []int64 {
	t.Helper()
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make([]int64, 0, totalIncrs)
	)
	for range numWorkers {
		wg.Go(func() {
			for range numIncrPer {
				v := decodeCounter(t, postIncr(t, base, name, ""))
				mu.Lock()
				results = append(results, v.Value)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return results
}

func TestConcurrentIncrCorrect(t *testing.T) {
	srv := httptest.NewServer(newServer("none"))
	defer srv.Close()

	results := concurrentIncr(t, srv.URL, "race")

	final := decodeCounter(t, getCounter(t, srv.URL, "race"))
	if final.Value != totalIncrs {
		t.Fatalf("final value = %d, want %d", final.Value, totalIncrs)
	}

	slices.Sort(results)
	for i, v := range results {
		want := int64(i + 1)
		if v != want {
			t.Fatalf("returned values are not a permutation of 1..%d: sorted[%d] = %d, want %d", totalIncrs, i, v, want)
		}
	}
}

func TestConcurrentIncrLostUpdate(t *testing.T) {
	const attempts = 5
	for i := 0; i < attempts; i++ {
		srv := httptest.NewServer(newServer("lost-update"))
		concurrentIncr(t, srv.URL, "race")
		final := decodeCounter(t, getCounter(t, srv.URL, "race"))
		srv.Close()

		if final.Value < totalIncrs {
			return // observed a lost update, as required
		}
	}
	t.Fatalf("lost-update bug did not lose any increments in %d attempts", attempts)
}

func TestDropReset(t *testing.T) {
	srv := httptest.NewServer(newServer("drop-reset"))
	defer srv.Close()

	postIncr(t, srv.URL, "a", "").Body.Close()
	reset(t, srv.URL)

	final := decodeCounter(t, getCounter(t, srv.URL, "a"))
	if final.Value == 0 {
		t.Fatalf("drop-reset: reset actually cleared the counter, got %+v", final)
	}
}
