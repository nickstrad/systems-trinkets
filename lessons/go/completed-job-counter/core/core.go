// Package core applies event deliveries independently of experiment accounting.
package core

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Beginner accepts a connection for the lesson or a pool for concurrent callers.
type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Strategy decides whether one delivery may increment its named counter row.
type Strategy struct {
	Name  string
	Claim func(context.Context, pgx.Tx, int) (bool, error)
}

// Naive counts every delivery attempt.
func Naive() Strategy {
	return Strategy{Name: "naive", Claim: func(context.Context, pgx.Tx, int) (bool, error) { return true, nil }}
}

// Idempotent counts each event once, using the transaction's uniqueness check.
func Idempotent() Strategy {
	return Strategy{Name: "idempotent", Claim: func(ctx context.Context, tx pgx.Tx, id int) (bool, error) {
		tag, err := tx.Exec(ctx, `
			insert into job_seen(event_id) values ($1)
			on conflict (event_id) do nothing`, id)
		return tag.RowsAffected() == 1, err
	}}
}

// Apply runs one delivery in a transaction. total is meaningful only when
// applied is true; a duplicate returns zero, not a current counter snapshot.
func Apply(ctx context.Context, db Beginner, s Strategy, id int) (total int, applied bool, err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)
	claimed, err := s.Claim(ctx, tx, id)
	if err != nil {
		return 0, false, err
	}
	if claimed {
		err := tx.QueryRow(ctx, `
		update job_counter set total = total + 1 where mode = $1
		returning total`, s.Name).Scan(&total)
		if err == pgx.ErrNoRows {
			return 0, false, errors.New("counter row is missing")
		}
		if err != nil {
			return 0, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	return total, claimed, nil
}
