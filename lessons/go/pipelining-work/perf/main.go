// HTTP adapter for the pipelining-work lesson: the base runner's two
// variants, chosen per request, so one k6 run reproduces the base table.
// Needs Valkey (make up-valkey, redis://localhost:6379).
package main

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/internal/lab/valkey"
	"github.com/nickstrad/systems-trinkets/lessons/go/pipelining-work/core"
)

// variant is one of the base runner's variants plus the counters it owns.
// Each variant has its own key set, so the invariant can be checked per
// variant: shared keys would only let /stats compare one blended total.
type variant struct {
	name    string
	op      func(context.Context, core.Store, []string) error
	keys    []string
	batches atomic.Int64 // batches the core confirmed
	failed  atomic.Int64 // batches that returned an error; may be partly applied
}

func main() {
	ctx := context.Background()
	cache := valkey.Connect(ctx)
	defer cache.Close()

	// BATCH_SIZE is the base lesson's knob (200 there); one request is one batch.
	batchSize := perf.Int("BATCH_SIZE", 200, 1, 1000)

	// Unique prefix per server so concurrent servers never share counters;
	// shutdown deletes only these keys.
	prefix := "perf:pipeline:" + perf.Token()
	variants := []*variant{
		{name: "sequential", op: core.IncrementSequential},
		{name: "pipeline", op: core.IncrementPipelined},
	}
	byName := map[string]*variant{}
	var all []string
	for _, v := range variants {
		v.keys = make([]string, batchSize)
		for i := range v.keys {
			v.keys[i] = prefix + ":" + v.name + ":" + strconv.Itoa(i)
		}
		byName[v.name] = v
		all = append(all, v.keys...)
	}
	defer cache.Del(context.Background(), all...)

	// Like the base runner: one untimed call per variant so connection setup
	// does not land in the first measured request, then start from zero.
	for _, v := range variants {
		lab.Check(v.op(ctx, cache, v.keys))
	}
	lab.Check(cache.Del(ctx, all...).Err())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /operation", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("variant")
		v, ok := byName[name]
		if !ok {
			perf.Fail(w, 400, "variant must be sequential or pipeline")
			return
		}
		// Time only the core call, as main.go does; HTTP, JSON, and the
		// invariant read stay outside elapsed_ms.
		start := time.Now()
		err := v.op(r.Context(), cache, v.keys)
		elapsed := time.Since(start)
		if err != nil {
			v.failed.Add(1)
			perf.Fail(w, 503, err.Error())
			return
		}
		v.batches.Add(1)
		perf.JSON(w, 200, map[string]any{
			"variant":    v.name,
			"elapsed_ms": lab.Ms(elapsed),
		})
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		rows := make([]map[string]any, 0, len(variants))
		for _, v := range variants {
			// MGET reads what Valkey stores, not what INCR replied.
			sum, err := core.Sum(r.Context(), cache, v.keys)
			if err != nil {
				perf.Fail(w, 503, err.Error())
				return
			}
			n := v.batches.Load()
			rows = append(rows, map[string]any{
				"variant":        v.name,
				"batches":        n,
				"failed_batches": v.failed.Load(),
				"expected_sum":   n * int64(batchSize),
				"actual_sum":     sum,
			})
		}
		perf.JSON(w, 200, map[string]any{"batch_size": batchSize, "variants": rows})
	})
	perf.Serve(mux)
}
