// HTTP adapter for the background-job-queue lesson: the base runner's two
// claim modes, chosen per request, so one k6 run reproduces the base table.
// Needs Postgres (make up-postgres,
// postgres://trinkets:trinkets@localhost:5432/trinkets).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/lessons/go/background-job-queue/core"
)

const enqueue = `insert into background_queue_jobs(status) values ('pending')`

// queue is one variant's own jobs table. core.Schema and the claim queries
// name background_queue_jobs without a schema, so each variant gets its own
// private schema and pool (search_path) and therefore its own rows. A shared
// table would let blocking claims wait on rows skip_locked claims hold, and
// let one variant complete the other's jobs, so neither the claim times nor
// the exactly-once invariant could be attributed to one variant.
type queue struct {
	name     string
	claimSQL string
	db       *pgxpool.Pool
	enqueued atomic.Int64 // pending jobs this variant's requests inserted
	empty    atomic.Int64 // claims that found no pending job (pgx.ErrNoRows)
	failed   atomic.Int64 // requests that returned a dependency error

	mu     sync.Mutex
	claims int64              // successful core.Work calls
	jobs   map[int64]struct{} // distinct job ids the claims returned
}

func main() {
	ctx := context.Background()

	// WORK_MS is the base lesson's workTime: how long a claimed row stays
	// locked. POOL_SIZE (read by perf.Database) applies to each variant's pool.
	workTime := time.Duration(perf.Int("WORK_MS", 100, 1, 1000)) * time.Millisecond

	queues := []*queue{
		{name: "blocking", claimSQL: core.ClaimQuery},
		{name: "skip_locked", claimSQL: core.SkipLockedQuery},
	}
	byName := map[string]*queue{}
	for _, q := range queues {
		db, cleanup := perf.Database(ctx)
		defer cleanup()
		q.db = db
		q.jobs = map[int64]struct{}{}
		_, err := db.Exec(ctx, core.Schema)
		lab.Check(err)
		// Prepare the enqueue, claim, and finish statements on every pool
		// connection before any measured request, as the base runner's untimed
		// round does; pgx prepares each new SQL text on first use per
		// connection. Then empty the table so ids and counts start from zero.
		perf.Warm(ctx, db, func(ctx context.Context, c *pgxpool.Conn) error {
			if _, err := c.Exec(ctx, enqueue); err != nil {
				return err
			}
			_, err := core.Work(ctx, c, q.claimSQL, 0)
			return err
		})
		_, err = db.Exec(ctx, `truncate background_queue_jobs restart identity`)
		lab.Check(err)
		byName[q.name] = q
		fmt.Println("variant", q.name, "uses the schema above")
	}

	mux := http.NewServeMux()
	// One request is one job: enqueue it, then claim and finish the oldest
	// eligible job with this variant's claim query, as one base-lesson worker
	// does. The claimed job can be another request's; each still ends done.
	mux.HandleFunc("POST /operation", perf.Limit(100000, func(w http.ResponseWriter, r *http.Request) {
		q, ok := byName[r.URL.Query().Get("variant")]
		if !ok {
			perf.Fail(w, 400, "variant must be blocking or skip_locked")
			return
		}
		if _, err := q.db.Exec(r.Context(), enqueue); err != nil {
			q.failed.Add(1)
			perf.Fail(w, 503, err.Error())
			return
		}
		q.enqueued.Add(1)
		claim, err := core.Work(r.Context(), q.db, q.claimSQL, workTime)
		if errors.Is(err, pgx.ErrNoRows) {
			// An empty queue is not a completed job.
			q.empty.Add(1)
			perf.Fail(w, 409, "no pending job to claim")
			return
		}
		if err != nil {
			q.failed.Add(1)
			perf.Fail(w, 503, err.Error())
			return
		}
		q.mu.Lock()
		q.claims++
		q.jobs[claim.JobID] = struct{}{}
		q.mu.Unlock()
		// elapsed_ms is Claim.Took: the claim query alone, which is what the
		// base runner records as claim_ms. It excludes the insert, pool
		// waits, WORK_MS, the update, and the commit; HTTP time includes them.
		perf.JSON(w, 200, map[string]any{
			"variant":    q.name,
			"job_id":     claim.JobID,
			"elapsed_ms": lab.Ms(claim.Took),
		})
	}))
	// The base invariant per variant: claims made vs distinct jobs claimed
	// (every job claimed exactly once), plus what the table itself holds.
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		rows := make([]map[string]any, 0, len(queues))
		for _, q := range queues {
			var done, pending int64
			err := q.db.QueryRow(r.Context(),
				`select count(*) filter (where status = 'done'),
				        count(*) filter (where status = 'pending')
				 from background_queue_jobs`).Scan(&done, &pending)
			if err != nil {
				perf.Fail(w, 503, err.Error())
				return
			}
			q.mu.Lock()
			claims, distinct := q.claims, int64(len(q.jobs))
			q.mu.Unlock()
			rows = append(rows, map[string]any{
				"variant":         q.name,
				"enqueued":        q.enqueued.Load(),
				"claims":          claims,
				"distinct_jobs":   distinct,
				"done":            done,
				"pending":         pending,
				"empty_claims":    q.empty.Load(),
				"failed_requests": q.failed.Load(),
			})
		}
		perf.JSON(w, 200, map[string]any{"variants": rows})
	})
	perf.Serve(mux)
}
