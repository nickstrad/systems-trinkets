// Package postgres backs the counter with PostgreSQL (github.com/jackc/pgx/v5
// through database/sql). The primitive under test is the same single-statement
// upsert with RETURNING as the SQLite engine: one statement is one
// transaction, and the row lock it takes serialises concurrent increments.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"systems-trinkets/examples/counter"
)

const schema = `CREATE TABLE IF NOT EXISTS counters (
	name  TEXT PRIMARY KEY,
	value BIGINT NOT NULL
)`

const incrSQL = `INSERT INTO counters(name, value) VALUES($1, $2)
	ON CONFLICT(name) DO UPDATE SET value = counters.value + excluded.value
	RETURNING value`

type store struct{ db *sql.DB }

// Open connects to the database named by dsn (postgres://…) and creates the
// counters table.
func Open(dsn string) (counter.Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}
	// Bound the first connection: a frozen server should fail here with a
	// cause, not hang the process before it ever logs "listening on".
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres connect %s: %w", redact(dsn), err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres schema: %w", err)
	}
	return &store{db: db}, nil
}

// redact hides the password in a DSN that is about to appear in an error
// (errors reach the SUT log and, from there, a run's artifacts). A DSN in
// libpq keyword form is not a URL, so it is dropped entirely.
func redact(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return "(dsn)"
	}
	return u.Redacted()
}

func (s *store) Incr(ctx context.Context, name string, delta int64) (int64, error) {
	var v int64
	if err := s.db.QueryRowContext(ctx, incrSQL, name, delta).Scan(&v); err != nil {
		return 0, fmt.Errorf("postgres incr %s: %w", name, err)
	}
	return v, nil
}

func (s *store) Get(ctx context.Context, name string) (int64, error) {
	var v int64
	err := s.db.QueryRowContext(ctx, `SELECT value FROM counters WHERE name = $1`, name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("postgres get %s: %w", name, err)
	}
	return v, nil
}

func (s *store) Del(ctx context.Context, name string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM counters WHERE name = $1`, name); err != nil {
		return fmt.Errorf("postgres del %s: %w", name, err)
	}
	return nil
}

func (s *store) Reset(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `TRUNCATE counters`); err != nil {
		return fmt.Errorf("postgres reset: %w", err)
	}
	return nil
}

func (s *store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	return nil
}

func (s *store) Close() error { return s.db.Close() }
