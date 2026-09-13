package counter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"systems-trinkets/examples/counter/internal/storetest"
)

// The memory implementation obeys the same store contract as every engine.

func TestMemoryStore(t *testing.T) { storetest.Run(t, NewMemory) }

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
