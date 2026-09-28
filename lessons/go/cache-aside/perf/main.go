// HTTP adapter for the cache-aside lesson: the base runner's read path under
// concurrent traffic. The contrast is the source the core reports (cache hit
// or postgres miss); the knob is the TTL, fixed per server run.
// Needs Postgres (postgres://trinkets:trinkets@localhost:5432/trinkets) and
// Valkey (redis://localhost:6379): make up-postgres up-valkey.
package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/internal/lab/valkey"
	"github.com/nickstrad/systems-trinkets/lessons/go/cache-aside/core"
)

// profile is one seeded row plus the bookkeeping for concurrent misses.
type profile struct {
	id int64 // the id stored in Postgres and used in the cache key

	mu          sync.Mutex
	lastMissEnd time.Time // when the latest miss for this profile returned
}

func main() {
	ctx := context.Background()
	db, cleanup := perf.Database(ctx)
	defer cleanup()
	cache := valkey.Connect(ctx)
	defer cache.Close()

	// TTL_MS is the base lesson's cacheTTL (30 s there, chosen to outlast the
	// run). A shorter TTL expires during the run, so misses recur.
	ttlMs := perf.Int("TTL_MS", 30000, 10, 600000)
	ttl := time.Duration(ttlMs) * time.Millisecond
	// PROFILE_COUNT profiles are requested round-robin by the workload, so each
	// one is read every PROFILE_COUNT/RATE seconds.
	count := perf.Int("PROFILE_COUNT", 10, 1, 1000)

	// core.ProfileKey(id) is always "profile:<id>", so the adapter namespaces
	// ids instead of keys: a random base far above the base lesson's id 42
	// keeps this server's keys apart from the lesson's and from other servers.
	base, err := strconv.ParseInt(perf.Token()[:12], 16, 64)
	lab.Check(err)
	base *= 10000
	fmt.Printf("cache keys: profile:%d..profile:%d\n", base+1, base+int64(count))

	_, err = db.Exec(ctx, core.Schema)
	lab.Check(err)
	// Profile i (1..count) has id base+i and name "Ada i"; k6 checks the name.
	_, err = db.Exec(ctx, `insert into cache_aside_profiles(id, name)
		select $1::bigint + g, 'Ada ' || g from generate_series(1, $2::int) g`, base, count)
	lab.Check(err)
	profiles := make([]*profile, count)
	keys := make([]string, count)
	for i := range profiles {
		profiles[i] = &profile{id: base + int64(i+1)}
		keys[i] = core.ProfileKey(profiles[i].id)
	}
	defer cache.Del(context.Background(), keys...)

	// pgx prepares a statement per connection on first use. One untimed miss
	// on every pool connection prepares the core's SELECT everywhere, so no
	// timed miss pays connect + prepare; deleting the key first forces each
	// read to miss. Then the cache starts cold, as in main.go.
	perf.Warm(ctx, db, func(ctx context.Context, c *pgxpool.Conn) error {
		if err := cache.Del(ctx, keys[0]).Err(); err != nil {
			return err
		}
		_, _, err := core.ReadProfile(ctx, c, cache, profiles[0].id, ttl)
		return err
	})
	lab.Check(cache.Del(ctx, keys...).Err())

	var served, hits, misses, redundant, failed atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("GET /operation", func(w http.ResponseWriter, r *http.Request) {
		i, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || i < 1 || i > count {
			perf.Fail(w, 400, fmt.Sprintf("id must be 1..%d", count))
			return
		}
		p := profiles[i-1]
		// Time only the core call, as main.go does; HTTP and bookkeeping stay
		// outside elapsed_ms.
		start := time.Now()
		name, source, err := core.ReadProfile(r.Context(), db, cache, p.id, ttl)
		elapsed := time.Since(start)
		if err != nil {
			failed.Add(1)
			perf.Fail(w, 503, err.Error())
			return
		}
		served.Add(1)
		if source == "cache" {
			hits.Add(1)
		} else {
			misses.Add(1)
			// A miss that started before another miss of the same profile had
			// returned raced it to Postgres: both found the key missing. That
			// is one redundant database read, the unit of a cache stampede.
			end := start.Add(elapsed)
			p.mu.Lock()
			if start.Before(p.lastMissEnd) {
				redundant.Add(1)
			}
			if end.After(p.lastMissEnd) {
				p.lastMissEnd = end
			}
			p.mu.Unlock()
		}
		perf.JSON(w, 200, map[string]any{
			"id":         i,
			"name":       name,
			"source":     source,
			"elapsed_ms": lab.Ms(elapsed),
		})
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		// Read both stores now: what Postgres stores and what the cache holds
		// must agree for every key still cached.
		stored := map[int64]string{}
		rows, err := db.Query(r.Context(), `select id, name from cache_aside_profiles`)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				perf.Fail(w, 503, err.Error())
				return
			}
			stored[id] = name
		}
		rows.Close()
		values, err := cache.MGet(r.Context(), keys...).Result()
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		cached, agree := 0, 0
		for i, v := range values {
			if s, ok := v.(string); ok {
				cached++
				if s == stored[profiles[i].id] {
					agree++
				}
			}
		}
		perf.JSON(w, 200, map[string]any{
			"ttl_ms":           ttlMs,
			"profiles":         count,
			"served":           served.Load(),
			"hits":             hits.Load(),
			"misses":           misses.Load(),
			"failed":           failed.Load(),
			"redundant_misses": redundant.Load(),
			"stored":           len(stored),
			"cached":           cached,
			"cached_agree":     agree,
		})
	})
	perf.Serve(mux)
}
