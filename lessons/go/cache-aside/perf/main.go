package main

import (
	"context"
	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/internal/lab/valkey"
	"github.com/nickstrad/systems-trinkets/lessons/go/cache-aside/core"
	"net/http"
	"sync/atomic"
	"time"
)

func main() {
	ctx := context.Background()
	db, cleanup := perf.Database(ctx)
	defer cleanup()
	cache := valkey.Connect(ctx)
	defer cache.Close()
	id := time.Now().UnixNano()
	defer cache.Del(ctx, core.ProfileKey(id))
	_, err := db.Exec(ctx, core.Schema)
	lab.Check(err)
	_, err = db.Exec(ctx, `insert into cache_aside_profiles values ($1, 'Ada')`, id)
	lab.Check(err)
	ttl := time.Duration(perf.Int("CACHE_TTL_MS", 1000, 1, 60000)) * time.Millisecond
	var hits, misses atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("GET /operation", func(w http.ResponseWriter, r *http.Request) {
		name, source, err := core.ReadProfile(r.Context(), db, cache, id, ttl)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		if source == "cache" {
			hits.Add(1)
		} else {
			misses.Add(1)
		}
		perf.JSON(w, 200, map[string]string{"name": name, "source": source})
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		perf.JSON(w, 200, map[string]int64{"hits": hits.Load(), "misses": misses.Load()})
	})
	perf.Serve(mux)
}
