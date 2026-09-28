// HTTP adapter for the completed-job-counter lesson: the base runner's two
// strategies, chosen per request, so one k6 run reproduces the base table.
// Needs Postgres (make up-postgres,
// postgres://trinkets:trinkets@localhost:5432/trinkets); the tables live in a
// private perf_* schema that shutdown drops.
package main

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/lessons/go/completed-job-counter/core"
)

const (
	maxEventID  = 999_999
	maxRequests = 300_000 // Limit answers 429 after this, bounding job_seen growth
)

// variant is one core strategy plus the adapter's tally of what it confirmed.
// The tally is the client side of the invariant (deliveries and distinct
// events the core acknowledged); /stats reads the other side from the tables.
type variant struct {
	core.Strategy
	deliveries atomic.Int64 // Apply returned without error
	failed     atomic.Int64 // Apply returned an error; the transaction may not have committed
	mu         sync.Mutex
	events     map[int]struct{} // distinct event ids with a confirmed delivery
}

func main() {
	ctx := context.Background()
	db, cleanup := perf.Database(ctx)
	defer cleanup()

	// DELIVERIES is the base lesson's knob (deliveries = 3 in main.go). The
	// adapter owns it: /health reports it, the k6 workload reads it from there
	// in setup(), and the adapter rejects an attempt above it.
	deliveries := perf.Int("DELIVERIES", 3, 1, 10)

	variants := []*variant{
		{Strategy: core.Naive()},
		{Strategy: core.Idempotent()},
	}
	byName := map[string]*variant{}
	for _, v := range variants {
		v.events = map[int]struct{}{}
		byName[v.Name] = v
	}

	_, err := db.Exec(ctx, core.Schema)
	lab.Check(err)
	// Like main.go's untimed first pass: run each strategy once on every pool
	// connection (event id 0, which k6 never sends), then reset the rows.
	reset(ctx, db, variants)
	perf.Warm(ctx, db, func(ctx context.Context, c *pgxpool.Conn) error {
		for _, v := range variants {
			if _, _, err := core.Apply(ctx, c, v.Strategy, 0); err != nil {
				return err
			}
		}
		return nil
	})
	reset(ctx, db, variants)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /operation", perf.Limit(maxRequests, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, ok := byName[q.Get("variant")]
		if !ok {
			perf.Fail(w, 400, "variant must be naive or idempotent")
			return
		}
		id, err := strconv.Atoi(q.Get("id"))
		if err != nil || id < 1 || id > maxEventID {
			perf.Fail(w, 400, "id must be 1.."+strconv.Itoa(maxEventID))
			return
		}
		attempt, err := strconv.Atoi(q.Get("attempt"))
		if err != nil || attempt < 1 || attempt > deliveries {
			perf.Fail(w, 400, "attempt must be 1..DELIVERIES ("+strconv.Itoa(deliveries)+")")
			return
		}
		// Time only core.Apply, as main.go's deliver does. The connection is
		// acquired first, so waiting for a free pool connection stays outside
		// elapsed, as the base lesson's single connection never waited.
		c, err := db.Acquire(r.Context())
		if err != nil {
			v.failed.Add(1)
			perf.Fail(w, 503, err.Error())
			return
		}
		start := time.Now()
		total, applied, err := core.Apply(r.Context(), c, v.Strategy, id)
		elapsed := time.Since(start)
		c.Release()
		if err != nil {
			v.failed.Add(1)
			perf.Fail(w, 503, err.Error())
			return
		}
		v.deliveries.Add(1)
		v.mu.Lock()
		v.events[id] = struct{}{}
		v.mu.Unlock()
		response := map[string]any{
			"variant":    v.Name,
			"attempt":    attempt,
			"applied":    applied,
			"elapsed_ms": lab.Ms(elapsed),
		}
		// A skipped duplicate has no total: core returns zero, not a snapshot.
		if applied {
			response["total"] = total
		}
		perf.JSON(w, 200, response)
	}))
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		// One statement, one snapshot: both counter rows and the seen table.
		rows, err := db.Query(r.Context(), `
			select mode, total, (select count(*) from job_seen)
			from job_counter`)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		totals := map[string]int64{}
		var seen int64
		for rows.Next() {
			var mode string
			var total int64
			if err := rows.Scan(&mode, &total, &seen); err != nil {
				rows.Close()
				perf.Fail(w, 503, err.Error())
				return
			}
			totals[mode] = total
		}
		if err := rows.Err(); err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(variants))
		for _, v := range variants {
			v.mu.Lock()
			unique := int64(len(v.events))
			v.mu.Unlock()
			n := v.deliveries.Load()
			// main.go's wantTotal: naive counts every delivery, idempotent
			// counts each event once. Here from confirmed calls, not constants.
			expected := unique
			var seenCol any // naive never claims, so it has no seen rows
			if v.Name == "naive" {
				expected = n
			} else {
				seenCol = seen
			}
			out = append(out, map[string]any{
				"variant":           v.Name,
				"deliveries":        n,
				"failed_deliveries": v.failed.Load(),
				"unique_events":     unique,
				"seen":              seenCol,
				"expected_total":    expected,
				"actual_total":      totals[v.Name],
			})
		}
		perf.JSON(w, 200, map[string]any{"variants": out})
	})
	perf.Serve(mux)
}

// reset empties both tables and seeds one counter row per strategy.
func reset(ctx context.Context, db *pgxpool.Pool, variants []*variant) {
	names := make([]string, len(variants))
	for i, v := range variants {
		names[i] = v.Name
	}
	_, err := db.Exec(ctx, `truncate job_counter, job_seen`)
	lab.Check(err)
	_, err = db.Exec(ctx, `insert into job_counter(mode) select unnest($1::text[])`, names)
	lab.Check(err)
}
