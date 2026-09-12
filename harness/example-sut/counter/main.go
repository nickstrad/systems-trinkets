// Command counter is a reference implementation of the counter suite's HTTP
// contract (suites/counter/CONTRACT.md), used as a correct and as a
// deliberately buggy target for the invariant harness.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// nameRE matches valid counter names per the contract.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// store is the in-memory counter table, guarded by mu.
type store struct {
	mu     sync.Mutex
	values map[string]int64
}

func newStore() *store {
	return &store{values: map[string]int64{}}
}

// incr adds delta to name and returns the post-increment value, using the
// correct atomic read-modify-write.
func (s *store) incr(name string, delta int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name] += delta
	return s.values[name]
}

// incrLostUpdate reproduces a classic lost-update bug: the read and the write
// happen under separate critical sections, with a scheduling gap between
// them wide enough that concurrent increments race and clobber each other.
func (s *store) incrLostUpdate(name string, delta int64) int64 {
	s.mu.Lock()
	old := s.values[name]
	s.mu.Unlock()

	runtime.Gosched()
	time.Sleep(50 * time.Microsecond)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name] = old + delta
	return s.values[name]
}

// get returns the current value of name, or 0 if it has never been touched.
func (s *store) get(name string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[name]
}

// del removes one counter; it reads 0 afterwards.
func (s *store) del(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, name)
}

// reset deletes every counter.
func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = map[string]int64{}
}

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

// newServer builds the handler for the counter contract. bug selects a
// deliberate misbehavior: "none" (correct), "lost-update" (racy incr),
// "drop-reset" (/_reset is a no-op), or "slow" (20ms latency injected on
// every counter request).
func newServer(bug string) http.Handler {
	s := newStore()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("POST /_reset", func(w http.ResponseWriter, r *http.Request) {
		if bug != "drop-reset" {
			s.reset()
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// named wraps a counter handler: the slow bug's latency and the name
	// check happen here once, so every /counters/{name} route gets both.
	named := func(fn func(w http.ResponseWriter, r *http.Request, name string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if bug == "slow" {
				time.Sleep(20 * time.Millisecond)
			}
			name := r.PathValue("name")
			if !nameRE.MatchString(name) {
				writeError(w, http.StatusBadRequest, "invalid name")
				return
			}
			fn(w, r, name)
		}
	}

	incr := s.incr
	if bug == "lost-update" {
		incr = s.incrLostUpdate
	}
	mux.HandleFunc("POST /counters/{name}/incr", named(func(w http.ResponseWriter, r *http.Request, name string) {
		delta, ok := parseDelta(w, r)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, counterResponse{Name: name, Value: incr(name, delta)})
	}))

	mux.HandleFunc("GET /counters/{name}", named(func(w http.ResponseWriter, r *http.Request, name string) {
		writeJSON(w, http.StatusOK, counterResponse{Name: name, Value: s.get(name)})
	}))

	mux.HandleFunc("DELETE /counters/{name}", named(func(w http.ResponseWriter, r *http.Request, name string) {
		s.del(name)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	bug := flag.String("bug", "none", "bug to inject: none|lost-update|drop-reset|slow")
	flag.Parse()

	switch *bug {
	case "none", "lost-update", "drop-reset", "slow":
	default:
		log.Fatalf("counter: unknown --bug %q", *bug)
	}

	srv := &http.Server{Addr: *addr, Handler: newServer(*bug)}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s bug=%s", *addr, *bug)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("counter: %v", err)
	}
}
