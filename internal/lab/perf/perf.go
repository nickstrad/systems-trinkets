// Package perf holds lifecycle helpers for the local HTTP performance adapters.
package perf

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/postgres"
)

// settings records every experiment setting an adapter read, so GET /health can
// report the effective values and the runner never has to guess defaults.
var settings = map[string]any{}

func Token() string {
	var b [8]byte
	_, err := rand.Read(b[:])
	lab.Check(err)
	return hex.EncodeToString(b[:])
}

// Int reads a bounded experiment setting before the listener starts.
func Int(name string, fallback, min, max int) int {
	n, err := strconv.Atoi(lab.Env(name, strconv.Itoa(fallback)))
	if err != nil || n < min || n > max {
		panic(fmt.Sprintf("%s must be %d..%d", name, min, max))
	}
	settings[name] = n
	return n
}

// Choice reads a setting that must be one of allowed; the first value is the default.
func Choice(name string, allowed ...string) string {
	v := lab.Env(name, allowed[0])
	if !slices.Contains(allowed, v) {
		panic(fmt.Sprintf("%s must be one of %s", name, strings.Join(allowed, ", ")))
	}
	settings[name] = v
	return v
}

// Database creates a private schema and pool, and returns cleanup for this schema only.
func Database(ctx context.Context) (*pgxpool.Pool, func()) {
	admin := postgres.Connect(ctx)
	schema := "perf_" + Token()
	fmt.Println("temporary schema:", schema)
	_, err := admin.Exec(ctx, "create schema "+schema)
	lab.Check(err)
	cfg, err := pgxpool.ParseConfig(postgres.URL())
	lab.Check(err)
	cfg.MaxConns = int32(Int("POOL_SIZE", 8, 1, 64))
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "5000"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	lab.Check(err)
	return pool, func() {
		pool.Close()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.Exec(cleanup, "drop schema "+schema+" cascade")
		if err != nil {
			fmt.Fprintln(os.Stderr, "schema cleanup:", err)
		}
		admin.Close(cleanup)
	}
}

// Warm holds every connection the pool may open and runs fn on each, so a
// statement pgx prepares per connection is prepared before the first timed
// request rather than inside it. Callers reset any rows fn wrote afterwards.
func Warm(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context, *pgxpool.Conn) error) {
	conns := make([]*pgxpool.Conn, pool.Config().MaxConns)
	for i := range conns {
		c, err := pool.Acquire(ctx)
		lab.Check(err)
		conns[i] = c
		lab.Check(fn(ctx, c))
	}
	for _, c := range conns {
		c.Release()
	}
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Fail writes an error response in the shape every adapter uses.
func Fail(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

// Limit answers 429 after max requests so a long run cannot grow fixtures without bound.
func Limit(max int64, next http.HandlerFunc) http.HandlerFunc {
	var served atomic.Int64
	return func(w http.ResponseWriter, r *http.Request) {
		if served.Add(1) > max {
			Fail(w, 429, "run limit reached")
			return
		}
		next(w, r)
	}
}

// Serve binds localhost, reports the actual URL for the combined runner, and drains on signal.
func Serve(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, 200, map[string]any{"ready": true, "settings": settings})
	})
	port := Int("PORT", 8080, 0, 65535)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	lab.Check(err)
	url := "http://" + listener.Addr().String()
	server := &http.Server{Handler: http.TimeoutHandler(mux, 10*time.Second, "request timed out"), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 12 * time.Second, IdleTimeout: 30 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	done := make(chan struct{})
	go func() {
		<-stop.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
		}
		close(done)
	}()
	if path := os.Getenv("READY_FILE"); path != "" {
		lab.Check(os.WriteFile(path, []byte(url), 0600))
	}
	fmt.Println("serving", url)
	err = server.Serve(listener)
	if err != http.ErrServerClosed {
		lab.Check(err)
	}
	cancel()
	<-done
}
