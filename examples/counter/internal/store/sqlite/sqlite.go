// Package sqlite backs the counter with a SQLite file (modernc.org/sqlite,
// the pure-Go driver the notes CLI uses). The primitive under test is a
// single upsert statement with RETURNING: one statement is one implicit
// transaction, so the read-modify-write cannot interleave.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"systems-trinkets/examples/counter"
)

// pragmas every connection needs: WAL so readers do not block the writer, and
// a busy timeout so a writer waits for the file lock instead of failing when
// another *process* holds it (within this process SetMaxOpenConns(1) means
// there is never contention). They ride on the DSN because the driver applies
// them to each new pool connection.
const pragmas = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

// schema is created on Open; value is INTEGER (SQLite's 64-bit int).
const schema = `CREATE TABLE IF NOT EXISTS counters (
	name  TEXT PRIMARY KEY,
	value INTEGER NOT NULL
)`

// incrSQL is the atomic post-increment: insert or add, returning the new
// value, in one statement.
const incrSQL = `INSERT INTO counters(name, value) VALUES(?, ?)
	ON CONFLICT(name) DO UPDATE SET value = value + excluded.value
	RETURNING value`

type store struct{ db *sql.DB }

// Open opens the SQLite database at dsn (a file path or a file: URI) and
// creates the counters table.
func Open(dsn string) (counter.Store, error) {
	db, err := sql.Open("sqlite", withPragmas(dsn))
	if err != nil {
		return nil, fmt.Errorf("sqlite open %s: %w", dsn, err)
	}
	// This is what serialises writers inside the process: SQLite has one write
	// lock per database anyway, so a single pool connection lets concurrent
	// callers queue in database/sql instead of racing for the lock (and never
	// seeing SQLITE_BUSY). The upsert statement is what makes each increment
	// atomic; this is what keeps 32 of them from contending.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}
	return &store{db: db}, nil
}

// withPragmas appends the connection pragmas to dsn, turning a bare path into
// a file: URI when needed. A DSN that already sets pragmas is left alone.
func withPragmas(dsn string) string {
	if strings.Contains(dsn, "_pragma=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + pragmas
}

func (s *store) Incr(ctx context.Context, name string, delta int64) (int64, error) {
	var v int64
	if err := s.db.QueryRowContext(ctx, incrSQL, name, delta).Scan(&v); err != nil {
		return 0, fmt.Errorf("sqlite incr %s: %w", name, err)
	}
	return v, nil
}

func (s *store) Get(ctx context.Context, name string) (int64, error) {
	var v int64
	err := s.db.QueryRowContext(ctx, `SELECT value FROM counters WHERE name = ?`, name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("sqlite get %s: %w", name, err)
	}
	return v, nil
}

func (s *store) Del(ctx context.Context, name string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM counters WHERE name = ?`, name); err != nil {
		return fmt.Errorf("sqlite del %s: %w", name, err)
	}
	return nil
}

func (s *store) Reset(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM counters`); err != nil {
		return fmt.Errorf("sqlite reset: %w", err)
	}
	return nil
}

func (s *store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("sqlite ping: %w", err)
	}
	return nil
}

// Close closes the database; the file stays on disk. That is deliberate: it
// outlives a killed process, so a SUT restarted on the same DSN still reads
// the increments the old one committed.
func (s *store) Close() error { return s.db.Close() }
