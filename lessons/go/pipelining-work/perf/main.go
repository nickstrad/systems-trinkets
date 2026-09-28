package main

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/internal/lab/valkey"
	"github.com/nickstrad/systems-trinkets/lessons/go/pipelining-work/core"
)

func main() {
	ctx := context.Background()
	cache := valkey.Connect(ctx)
	defer cache.Close()

	// MODE picks the core operation for the whole run; compare two runs to see
	// the round-trip cost. BATCH_SIZE bounds how many INCRs one request sends.
	mode := perf.Choice("MODE", "pipeline", "sequential")
	increment := map[string]func(context.Context, core.Store, []string) error{
		"pipeline":   core.IncrementPipelined,
		"sequential": core.IncrementSequential,
	}[mode]
	batchSize := perf.Int("BATCH_SIZE", 200, 1, 1000)

	// Unique keys per server so concurrent servers never share counters;
	// shutdown deletes only these.
	prefix := "perf:pipeline:" + perf.Token()
	keys := make([]string, batchSize)
	for i := range keys {
		keys[i] = prefix + ":" + strconv.Itoa(i)
	}
	defer cache.Del(context.Background(), keys...)
	lab.Check(cache.Del(ctx, keys...).Err())

	// batches counts requests the server confirmed, so expected_sum in /stats
	// is batches * batchSize and actual_sum is what Valkey holds.
	var batches atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /operation", func(w http.ResponseWriter, r *http.Request) {
		if err := increment(r.Context(), cache, keys); err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		batches.Add(1)
		perf.JSON(w, 200, map[string]any{"mode": mode, "increments": batchSize})
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		sum, err := core.Sum(r.Context(), cache, keys)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		n := batches.Load()
		perf.JSON(w, 200, map[string]any{
			"mode":         mode,
			"batches":      n,
			"expected_sum": n * int64(batchSize),
			"actual_sum":   sum,
		})
	})
	perf.Serve(mux)
}
