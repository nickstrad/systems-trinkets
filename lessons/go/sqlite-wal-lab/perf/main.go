package main

import (
	"context"
	"database/sql"
	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/lessons/go/sqlite-wal-lab/core"
	_ "modernc.org/sqlite"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
)

func main() {
	ctx := context.Background()
	mode := perf.Choice("MODE", "WAL", "DELETE")
	dir, err := os.MkdirTemp("", "sqlite-perf-")
	lab.Check(err)
	defer os.RemoveAll(dir)
	db, err := sql.Open("sqlite", core.DSN(filepath.Join(dir, "events.db"), mode, 200))
	lab.Check(err)
	defer db.Close()
	db.SetMaxOpenConns(perf.Int("POOL_SIZE", 8, 2, 64))
	_, err = db.ExecContext(ctx, core.Schema)
	lab.Check(err)
	snapshot, _, err := core.OpenSnapshot(ctx, db)
	lab.Check(err)
	defer snapshot.Rollback()
	var writes, failures atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /operation", perf.Limit(100000, func(w http.ResponseWriter, r *http.Request) {
		result, err := core.WriteEvent(r.Context(), db)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		if result.WriteErr != nil {
			failures.Add(1)
			perf.Fail(w, 503, result.WriteErr.Error())
			return
		}
		writes.Add(1)
		perf.JSON(w, 200, map[string]any{"written": true, "mode": mode, "write_ms": lab.Ms(result.Elapsed)})
	}))
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		perf.JSON(w, 200, map[string]any{"writes": writes.Load(), "write_failures": failures.Load(), "mode": mode})
	})
	perf.Serve(mux)
}
