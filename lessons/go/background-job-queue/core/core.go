// Package core contains one transactional queue worker operation.
package core

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// Schema creates the jobs table the queries below expect. Callers own
	// dropping or truncating it; the lesson runner and the perf adapter share it.
	Schema = `create table background_queue_jobs(id bigserial primary key, status text not null)`
	// ClaimQuery locks the oldest pending job until the worker commits.
	ClaimQuery = `select id from background_queue_jobs where status = 'pending' order by id limit 1 for update`
	// SkipLockedQuery passes over jobs another transaction holds.
	SkipLockedQuery = ClaimQuery + ` skip locked`
	finishQuery     = `update background_queue_jobs set status = 'done' where id = $1`
)

// Beginner accepts a pgx connection or pool. Each concurrent transaction needs
// its own connection; a future server can supply a pool.
type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Claim records the job and time spent in the claim query, excluding processing.
type Claim struct {
	JobID int64
	Took  time.Duration
}

// Work claims one job, simulates processing, marks it done, and commits.
// claimSQL is chosen by the caller from the query constants, never from an HTTP
// request. An empty queue returns pgx.ErrNoRows; no fixture work happens here.
func Work(ctx context.Context, db Beginner, claimSQL string, workTime time.Duration) (Claim, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return Claim{}, err
	}
	defer tx.Rollback(ctx)
	start := time.Now()
	var id int64
	if err := tx.QueryRow(ctx, claimSQL).Scan(&id); err != nil {
		return Claim{}, err
	}
	took := time.Since(start)
	time.Sleep(workTime)
	if _, err := tx.Exec(ctx, finishQuery, id); err != nil {
		return Claim{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Claim{}, err
	}
	return Claim{JobID: id, Took: took}, nil
}
