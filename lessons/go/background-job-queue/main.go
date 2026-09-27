package main

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/postgres"
)

const (
	trials = 5
	// workTime is how long a worker holds its claimed row before finishing
	// the job. With plain for update, every other worker waits for it.
	workTime = 100 * time.Millisecond

	// claimQuery locks the oldest pending job; the worker's transaction holds
	// the row lock until it commits. skipLockedQuery is the same claim but
	// passes over rows another transaction already holds.
	claimQuery      = `select id from background_queue_jobs where status = 'pending' order by id limit 1 for update`
	skipLockedQuery = claimQuery + ` skip locked`
	finishQuery     = `update background_queue_jobs set status = 'done' where id = $1`
)

// workerCounts is how many workers race for the queue in each round. Each
// round has exactly as many pending jobs as workers.
var workerCounts = []int{1, 2, 4, 8}

// A mode is the claim query every worker in the round uses.
var modes = []struct {
	name  string
	claim string
}{
	{"blocking", claimQuery},
	{"skip_locked", skipLockedQuery},
}

// A claim is one worker's result: which job it got and how long the claim
// query took, which is the time spent waiting on other workers' locks.
type claim struct {
	jobID int64
	took  time.Duration
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// admin sets up and resets the table. Every worker needs its own
	// connection: transactions can only contend for a row lock across
	// separate connections.
	admin := postgres.Connect(ctx)
	defer admin.Close(ctx)
	maxWorkers := workerCounts[len(workerCounts)-1]
	workers := make([]*pgx.Conn, maxWorkers)
	for i := range workers {
		workers[i] = postgres.Connect(ctx)
		defer workers[i].Close(ctx)
	}

	_, err := admin.Exec(ctx, `
		drop table if exists background_queue_jobs;
		create table background_queue_jobs(
			id bigserial primary key,
			status text not null
		);
	`)
	lab.Check(err)

	// Run each mode once before measuring so pgx has already prepared its
	// statements on every connection; otherwise the first timed round of each
	// mode includes that work.
	for _, m := range modes {
		runRound(ctx, admin, workers, m.claim)
	}

	out := lab.NewMeasurements("mode", "workers", "trial", "worker", "claim_ms", "claimed_job")
	for _, m := range modes {
		for _, n := range workerCounts {
			for trial := 1; trial <= trials; trial++ {
				claims := runRound(ctx, admin, workers[:n], m.claim)
				var slowest time.Duration
				for w, c := range claims {
					slowest = max(slowest, c.took)
					out.Write(
						m.name,
						strconv.Itoa(n),
						strconv.Itoa(trial),
						strconv.Itoa(w+1),
						fmt.Sprintf("%.3f", lab.Ms(c.took)),
						strconv.FormatInt(c.jobID, 10),
					)
				}
				fmt.Printf("%-12s workers=%d trial=%d slowest_claim=%v\n", m.name, n, trial, slowest)
			}
		}
	}
	out.Close()
}

// runRound resets the queue to one pending job per worker, starts every
// worker at once, and returns each worker's claim in worker order.
func runRound(ctx context.Context, admin *pgx.Conn, workers []*pgx.Conn, claimSQL string) []claim {
	_, err := admin.Exec(ctx, `truncate background_queue_jobs restart identity`)
	lab.Check(err)
	_, err = admin.Exec(ctx,
		`insert into background_queue_jobs(status) select 'pending' from generate_series(1, $1)`,
		len(workers),
	)
	lab.Check(err)

	claims := make([]claim, len(workers))
	var wg sync.WaitGroup
	for i, db := range workers {
		wg.Go(func() { claims[i] = work(ctx, db, claimSQL) })
	}
	wg.Wait()
	return claims
}

// work claims one job, holds it for workTime as if processing it, marks it
// done, and commits. Only the claim is timed: with plain for update it waits
// for every earlier worker's commit, with skip locked it takes the next free
// job at once.
func work(ctx context.Context, db *pgx.Conn, claimSQL string) claim {
	tx, err := db.Begin(ctx)
	lab.Check(err)
	defer tx.Rollback(ctx)

	start := time.Now()
	var id int64
	lab.Check(tx.QueryRow(ctx, claimSQL).Scan(&id))
	took := time.Since(start)

	time.Sleep(workTime)
	_, err = tx.Exec(ctx, finishQuery, id)
	lab.Check(err)
	lab.Check(tx.Commit(ctx))
	return claim{jobID: id, took: took}
}
