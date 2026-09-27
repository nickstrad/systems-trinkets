// Package core exposes the reader snapshot and timed write used by the WAL lab.
package core

import (
	"context"
	"database/sql"
	"time"
)

// OpenSnapshot opens a read transaction and performs the read that establishes
// its snapshot. The caller must Rollback the returned transaction to release it.
// The caller owns the database, schema, and per-connection pragma configuration.
func OpenSnapshot(ctx context.Context, db *sql.DB) (*sql.Tx, int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from events`).Scan(&count); err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	return tx, count, nil
}

// WriteResult separates the measured write outcome from setup failures.
type WriteResult struct {
	Elapsed  time.Duration
	WriteErr error
}

// WriteEvent prepares before timing so Elapsed includes only lock wait and write.
// A busy/locked write is a measured outcome in WriteErr; the returned error is
// reserved for obtaining a connection or preparing the statement.
func WriteEvent(ctx context.Context, db *sql.DB) (WriteResult, error) {
	writer, err := db.Conn(ctx)
	if err != nil {
		return WriteResult{}, err
	}
	defer writer.Close()
	insert, err := writer.PrepareContext(ctx, `insert into events(payload) values ('new event')`)
	if err != nil {
		return WriteResult{}, err
	}
	defer insert.Close()
	start := time.Now()
	_, err = insert.ExecContext(ctx)
	return WriteResult{Elapsed: time.Since(start), WriteErr: err}, nil
}
