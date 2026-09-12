// Package counter is the reference implementation of the counter suite's
// HTTP contract (suites/counter/CONTRACT.md): the worked example of a
// pattern done end to end, and the harness's own test fixture.
//
// NewHandler is the contract; a Store is the primitive under test. Engines
// live one package each under store/ (only cmd/counter imports them, so an
// in-process suite run links only the memory store). Bugs are Store
// decorators in bugs.go, one per invariant they break.
package counter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
)

// Store is the counter table behind the handler. Incr is the primitive whose
// atomicity INV-COUNTER-01/02 test; everything else is bookkeeping.
type Store interface {
	// Incr adds delta to name and returns the post-increment value, atomically.
	Incr(ctx context.Context, name string, delta int64) (int64, error)
	// Get returns the current value of name, 0 when absent.
	Get(ctx context.Context, name string) (int64, error)
	// Set overwrites name with v. It exists only so LostUpdate can express
	// the bug as read + Set; a correct handler never calls it.
	Set(ctx context.Context, name string, v int64) error
	// Del removes name; it reads 0 afterwards.
	Del(ctx context.Context, name string) error
	// Reset removes every counter (POST /_reset).
	Reset(ctx context.Context) error
	// Ping reports whether the store is reachable (GET /healthz).
	Ping(ctx context.Context) error
	Close() error
}

// nameRE matches valid counter names per the contract.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// incrRequest is the optional JSON body of POST /counters/{name}/incr.
type incrRequest struct {
	Delta *int64 `json:"delta"`
}

// counterResponse is the JSON body returned by incr and get.
type counterResponse struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// errorResponse is the JSON body returned on 4xx/5xx.
type errorResponse struct {
	Error string `json:"error"`
}

// NewHandler serves the counter contract on top of s: routes, the name
// regexp, delta parsing, /healthz → Ping, /_reset → Reset. A store error is a
// 500 with an error body; the handler never responds 2xx before the store
// has returned.
func NewHandler(s Store) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("POST /_reset", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Reset(r.Context()); err != nil {
			storeError(w, "reset", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// named wraps a counter handler so every /counters/{name} route checks
	// the name once, here.
	named := func(fn func(w http.ResponseWriter, r *http.Request, name string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			name := r.PathValue("name")
			if !nameRE.MatchString(name) {
				writeError(w, http.StatusBadRequest, "invalid name")
				return
			}
			fn(w, r, name)
		}
	}

	mux.HandleFunc("POST /counters/{name}/incr", named(func(w http.ResponseWriter, r *http.Request, name string) {
		delta, ok := parseDelta(w, r)
		if !ok {
			return
		}
		v, err := s.Incr(r.Context(), name, delta)
		if err != nil {
			storeError(w, "incr", err)
			return
		}
		writeJSON(w, http.StatusOK, counterResponse{Name: name, Value: v})
	}))

	mux.HandleFunc("GET /counters/{name}", named(func(w http.ResponseWriter, r *http.Request, name string) {
		v, err := s.Get(r.Context(), name)
		if err != nil {
			storeError(w, "get", err)
			return
		}
		writeJSON(w, http.StatusOK, counterResponse{Name: name, Value: v})
	}))

	mux.HandleFunc("DELETE /counters/{name}", named(func(w http.ResponseWriter, r *http.Request, name string) {
		if err := s.Del(r.Context(), name); err != nil {
			storeError(w, "del", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	return mux
}

// parseDelta reads and validates the optional request body, writing a 400
// response and returning ok=false on any error. An empty body yields the
// default delta of 1.
func parseDelta(w http.ResponseWriter, r *http.Request) (delta int64, ok bool) {
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	var body incrRequest
	if err := dec.Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return 1, true
		}
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return 0, false
	}
	if body.Delta == nil {
		return 1, true
	}
	if *body.Delta < 1 {
		writeError(w, http.StatusBadRequest, "delta must be >= 1")
		return 0, false
	}
	return *body.Delta, true
}

// storeError logs a failed store call and answers 500. The log line is what
// a crash test's sut.log shows when an engine goes away mid-run.
func storeError(w http.ResponseWriter, op string, err error) {
	log.Printf("counter: %s: %v", op, err)
	writeError(w, http.StatusInternalServerError, op+": "+err.Error())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
