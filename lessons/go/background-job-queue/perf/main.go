package main

import (
	"context"
	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/lessons/go/background-job-queue/core"
	"net/http"
	"time"
)

func main() {
	ctx := context.Background()
	db, cleanup := perf.Database(ctx)
	defer cleanup()
	_, err := db.Exec(ctx, core.Schema)
	lab.Check(err)
	query := map[string]string{"skip_locked": core.SkipLockedQuery, "blocking": core.ClaimQuery}[perf.Choice("MODE", "skip_locked", "blocking")]
	delay := time.Duration(perf.Int("WORK_MS", 100, 1, 1000)) * time.Millisecond
	mux := http.NewServeMux()
	mux.HandleFunc("POST /operation", perf.Limit(100000, func(w http.ResponseWriter, r *http.Request) {
		_, err := db.Exec(r.Context(), `insert into background_queue_jobs(status) values ('pending')`)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		result, err := core.Work(r.Context(), db, query, delay)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		perf.JSON(w, 200, map[string]any{"completed": true, "job_id": result.JobID, "claim_ms": lab.Ms(result.Took)})
	}))
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		var done, pending int
		err := db.QueryRow(r.Context(), `select count(*) filter(where status='done'), count(*) filter(where status='pending') from background_queue_jobs`).Scan(&done, &pending)
		if err != nil {
			perf.Fail(w, 503, err.Error())
			return
		}
		perf.JSON(w, 200, map[string]int{"done": done, "pending": pending})
	})
	perf.Serve(mux)
}
