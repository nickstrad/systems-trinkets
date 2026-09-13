// Command counter serves atomic counters on the selected storage engine.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"systems-trinkets/examples/counter"
	counterstore "systems-trinkets/examples/counter/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	engine := flag.String("engine", "memory", "backing store: memory|sqlite|valkey|postgres")
	dsn := flag.String("dsn", "", "store DSN (engine-specific default when empty)")
	flag.Parse()

	store, err := counterstore.Open(*engine, *dsn, *addr)
	if err != nil {
		log.Fatalf("counter: %v", err)
	}

	srv := &http.Server{Addr: *addr, Handler: counter.NewHandler(store)}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s engine=%s", *addr, *engine)
	err = srv.ListenAndServe()
	if cerr := store.Close(); cerr != nil {
		log.Printf("counter: close store: %v", cerr)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("counter: %v", err)
	}
}
