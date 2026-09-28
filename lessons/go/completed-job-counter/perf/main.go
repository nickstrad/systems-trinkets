package main

import (
	"context"
	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/lessons/go/completed-job-counter/core"
	"net/http"
	"strconv"
)

func main() {
	ctx := context.Background()
	db, cleanup := perf.Database(ctx)
	defer cleanup()
	strategy := map[string]core.Strategy{"idempotent": core.Idempotent(), "naive": core.Naive()}[perf.Choice("MODE", "idempotent", "naive")]
	_, err := db.Exec(ctx, core.Schema)
	lab.Check(err)
	_, err = db.Exec(ctx, `insert into job_counter(mode) values ($1)`, strategy.Name)
	lab.Check(err)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /operation", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || id < 0 || id >= 10000 {
			perf.Fail(w, 400, "id must be 0..9999")
			return
		}
		total, applied, err := core.Apply(r.Context(), db, strategy, id)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		response := map[string]any{"applied": applied, "mode": strategy.Name}
		if applied {
			response["total"] = total
		}
		perf.JSON(w, 200, response)
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		var total, seen int
		err := db.QueryRow(r.Context(), `select total, (select count(*) from job_seen) from job_counter where mode=$1`, strategy.Name).Scan(&total, &seen)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		perf.JSON(w, 200, map[string]any{"total": total, "seen": seen, "mode": strategy.Name})
	})
	perf.Serve(mux)
}
