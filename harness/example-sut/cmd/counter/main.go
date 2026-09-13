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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"systems-trinkets/harness/example-sut/counter"
	"systems-trinkets/harness/example-sut/counter/store/postgres"
	"systems-trinkets/harness/example-sut/counter/store/sqlite"
	"systems-trinkets/harness/example-sut/counter/store/valkey"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	engine := flag.String("engine", "memory", "backing store: memory|sqlite|valkey|postgres")
	dsn := flag.String("dsn", "", "store DSN (engine-specific default when empty)")
	bug := flag.String("bug", "none", "bug to inject: none|lost-update|drop-reset|write-behind|slow")
	flag.Parse()

	store, err := openStore(*engine, *dsn, *addr)
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

// Default DSNs per engine: the sqlite file comes from sqliteDSN, the others
// are the infra/ compose services on their default ports.
const (
	defaultValkeyDSN   = "redis://127.0.0.1:6379/1"
	defaultPostgresDSN = "postgres://trinkets:trinkets@127.0.0.1:5432/trinkets?sslmode=disable"
)

// openStore picks the engine, filling in that engine's default DSN when --dsn
// is empty. Each engine is its own package, so this is the only place they are
// linked in.
func openStore(engine, dsn, addr string) (counter.Store, error) {
	switch engine {
	case "memory":
		return counter.NewMemory(), nil
	case "sqlite":
		if dsn == "" {
			dsn = sqliteDSN(addr)
		}
		log.Printf("sqlite dsn=%s", dsn)
		return sqlite.Open(dsn)
	case "valkey":
		if dsn == "" {
			dsn = defaultValkeyDSN
		}
		return valkey.Open(dsn)
	case "postgres":
		if dsn == "" {
			dsn = defaultPostgresDSN
		}
		return postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unknown --engine %q", engine)
	}
}

// sqliteDSN is the default SQLite file for a SUT on addr: one stable path per
// listen address under the temp dir, created on first use and never deleted.
// Stable is the point — a crash test restarts the same target and must find
// the increments the killed process committed, so the path may not vary per
// start; per-address keeps four engines' SUTs from sharing one file.
func sqliteDSN(addr string) string {
	safe := strings.NewReplacer(":", "-", "/", "-", string(filepath.Separator), "-").Replace(addr)
	return filepath.Join(os.TempDir(), "counter-sqlite-"+safe+".db")
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
