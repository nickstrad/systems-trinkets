// Command counter serves the reference counter (example-sut/counter) on one
// engine, optionally with a deliberate bug, as a target for the harness:
//
//	counter --addr 127.0.0.1:8080 --engine memory|sqlite|valkey|postgres [--dsn …] [--bug none|lost-update|drop-reset|write-behind|slow]
//
// The binary only composes NewHandler(bug(engine)); there is no
// engine-specific bug code.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"systems-trinkets/harness/example-sut/counter"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	engine := flag.String("engine", "memory", "backing store: memory|sqlite|valkey|postgres")
	dsn := flag.String("dsn", "", "store DSN (engine-specific default when empty)")
	bug := flag.String("bug", "none", "bug to inject: none|lost-update|drop-reset|write-behind|slow")
	flag.Parse()

	store, err := openStore(*engine, *dsn)
	if err != nil {
		log.Fatalf("counter: %v", err)
	}
	buggy, err := withBug(store, *bug)
	if err != nil {
		_ = store.Close()
		log.Fatalf("counter: %v", err)
	}
	store = buggy

	srv := &http.Server{Addr: *addr, Handler: counter.NewHandler(store)}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s engine=%s bug=%s", *addr, *engine, *bug)
	err = srv.ListenAndServe()
	if cerr := store.Close(); cerr != nil {
		log.Printf("counter: close store: %v", cerr)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("counter: %v", err)
	}
}

// openStore picks the engine. Engines other than memory arrive with R2
// (test-plan.md §10a); until then they are a clear error, not a fallback.
func openStore(engine, dsn string) (counter.Store, error) {
	switch engine {
	case "memory":
		return counter.NewMemory(), nil
	case "sqlite", "valkey", "postgres":
		return nil, fmt.Errorf("--engine %s: not implemented yet (R2)", engine)
	default:
		return nil, fmt.Errorf("unknown --engine %q", engine)
	}
}

// withBug wraps s in the decorator named by bug.
func withBug(s counter.Store, bug string) (counter.Store, error) {
	switch bug {
	case "none":
		return s, nil
	case "lost-update":
		return counter.LostUpdate(s), nil
	case "drop-reset":
		return counter.DropReset(s), nil
	case "write-behind":
		return counter.WriteBehind(s, time.Second), nil
	case "slow":
		return counter.Slow(s, 20*time.Millisecond), nil
	default:
		return nil, fmt.Errorf("unknown --bug %q", bug)
	}
}
