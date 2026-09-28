// Package core exposes the reader snapshot and timed write used by the WAL lab.
package core

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DSN builds the modernc DSN for path. Pragmas in the DSN run on every
// connection the pool opens; with db.Exec they would reach only whichever
// pooled connection ran them. mode is the journal_mode (WAL or DELETE) and
// busyTimeoutMs how long a writer waits for a lock before failing.
func DSN(path, mode string, busyTimeoutMs int) string {
	return fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(%s)", path, busyTimeoutMs, mode)
}

// Schema creates the events table and its seed row, so OpenSnapshot has a
// row to read. Callers own the database file and its lifetime.
const Schema = `
	create table events(id integer primary key, payload text);
	insert into events(payload) values ('seed');
`

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
