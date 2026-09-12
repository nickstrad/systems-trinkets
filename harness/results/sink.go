package results

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/duckdb/duckdb-go/v2"
)

// Tables lists every table a run exports, in a stable order. The report CLI
// registers a view per table over results/runs/*/<table>.parquet.
var Tables = []string{"runs", "tests", "checks", "samples", "metrics"}

// DDL for the in-memory database. Column order matters: AppendRow arguments
// are positional. Ints are BIGINT so callers always pass int64.
var ddl = map[string]string{
	"runs": `CREATE TABLE runs (
		run_id VARCHAR, pattern VARCHAR, language VARCHAR, engine VARCHAR, label VARCHAR,
		target_url VARCHAR, harness_git_sha VARCHAR, sut_ref VARCHAR, go_version VARCHAR, host VARCHAR,
		started_at TIMESTAMP, finished_at TIMESTAMP)`,
	"tests": `CREATE TABLE tests (
		run_id VARCHAR, test VARCHAR, status VARCHAR, duration_ns BIGINT, error VARCHAR)`,
	"checks": `CREATE TABLE checks (
		run_id VARCHAR, test VARCHAR, invariant_id VARCHAR, ok BOOLEAN, message VARCHAR,
		details VARCHAR, recorded_at TIMESTAMP)`,
	"samples": `CREATE TABLE samples (
		run_id VARCHAR, test VARCHAR, phase VARCHAR, worker BIGINT, seq BIGINT, method VARCHAR,
		path_template VARCHAR, status BIGINT, latency_ns BIGINT, err VARCHAR, started_at TIMESTAMP)`,
	"metrics": `CREATE TABLE metrics (
		run_id VARCHAR, test VARCHAR, name VARCHAR, value DOUBLE, unit VARCHAR, labels VARCHAR, recorded_at TIMESTAMP)`,
}

// Sink collects rows from any goroutine and appends them into an in-memory
// DuckDB on a single collector goroutine (the Appender is not goroutine-safe).
// Close drains, writes the run row and COPYs every table to <dir>/<table>.parquet.
type Sink struct {
	dir string
	run RunRow

	mu     sync.RWMutex // guards closed; held (read) while sending on ch
	closed bool
	ch     chan any
	done   chan struct{}
	errs   []error // collector-side append/flush errors, reported by Close

	db   *sql.DB
	conn driver.Conn
	app  map[string]*duckdb.Appender
}

type flushReq struct{ done chan struct{} }

// Open creates dir, an in-memory DuckDB with the five tables, and starts the
// collector. run.StartedAt is set to now (UTC) if zero.
func Open(ctx context.Context, dir string, run RunRow) (*Sink, error) {
	if run.RunID == "" {
		run.RunID = NewRunID()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("results: create %s: %w", dir, err)
	}
	connector, err := duckdb.NewConnector("", nil)
	if err != nil {
		return nil, fmt.Errorf("results: open duckdb: %w", err)
	}
	conn, err := connector.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("results: connect: %w", err)
	}
	db := sql.OpenDB(connector)
	s := &Sink{
		dir:  dir,
		run:  run,
		ch:   make(chan any, 4096),
		done: make(chan struct{}),
		db:   db,
		conn: conn,
		app:  map[string]*duckdb.Appender{},
	}
	for _, t := range Tables {
		if _, err := db.ExecContext(ctx, ddl[t]); err != nil {
			s.closeDB()
			return nil, fmt.Errorf("results: create table %s: %w", t, err)
		}
		if t == "runs" {
			continue // written once, at Close, with a plain INSERT
		}
		a, err := duckdb.NewAppenderFromConn(conn, "", t)
		if err != nil {
			s.closeDB()
			return nil, fmt.Errorf("results: appender %s: %w", t, err)
		}
		s.app[t] = a
	}
	go s.collect()
	return s, nil
}

func (s *Sink) Test(r TestRow)     { s.send(r) }
func (s *Sink) Check(r CheckRow)   { s.send(r) }
func (s *Sink) Sample(r SampleRow) { s.send(r) }
func (s *Sink) Metric(r MetricRow) { s.send(r) }

// send hands the row to the collector and reports whether it was accepted;
// rows sent after Close are dropped.
func (s *Sink) send(row any) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false
	}
	s.ch <- row
	return true
}

// Flush waits until every row sent so far is appended and committed to the
// in-memory tables. Cheap; harness calls it after each test so a crashed
// process has as little unflushed data as possible.
func (s *Sink) Flush(ctx context.Context) error {
	req := flushReq{done: make(chan struct{})}
	if !s.send(req) {
		return nil
	}
	select {
	case <-req.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Sink) collect() {
	defer close(s.done)
	for m := range s.ch {
		var err error
		switch r := m.(type) {
		case TestRow:
			err = s.app["tests"].AppendRow(r.RunID, r.Test, r.Status, r.DurationNS, r.Error)
		case CheckRow:
			err = s.app["checks"].AppendRow(r.RunID, r.Test, r.InvariantID, r.OK, r.Message, r.DetailsJSON, r.At.UTC())
		case SampleRow:
			err = s.app["samples"].AppendRow(r.RunID, r.Test, r.Phase, int64(r.Worker), int64(r.Seq), r.Method,
				r.PathTemplate, int64(r.Status), r.LatencyNS, r.Err, r.StartedAt.UTC())
		case MetricRow:
			err = s.app["metrics"].AppendRow(r.RunID, r.Test, r.Name, r.Value, r.Unit, r.LabelsJSON, r.At.UTC())
		case flushReq:
			for _, a := range s.app {
				if ferr := a.Flush(); ferr != nil {
					err = ferr
				}
			}
			close(r.done)
		}
		if err != nil && len(s.errs) < 10 {
			s.errs = append(s.errs, err)
		}
	}
}

// Close stops the collector, records the run row with FinishedAt=now and
// exports every table to Parquet. Safe to call more than once; later calls
// are no-ops. Rows recorded during Close are dropped.
func (s *Sink) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.ch)
	s.mu.Unlock()
	<-s.done

	var errs []error
	for _, err := range s.errs {
		errs = append(errs, fmt.Errorf("results: append: %w", err))
	}
	for t, a := range s.app {
		if err := a.Close(); err != nil {
			errs = append(errs, fmt.Errorf("results: close appender %s: %w", t, err))
		}
	}
	s.run.FinishedAt = time.Now().UTC()
	r := s.run
	if _, err := s.db.ExecContext(ctx, `INSERT INTO runs VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.RunID, r.Pattern, r.Language, r.Engine, r.Label, r.TargetURL, r.HarnessSHA, r.SUTRef,
		r.GoVersion, r.Host, r.StartedAt.UTC(), r.FinishedAt.UTC()); err != nil {
		errs = append(errs, fmt.Errorf("results: run row: %w", err))
	}
	for _, t := range Tables {
		path := filepath.Join(s.dir, t+".parquet")
		q := fmt.Sprintf(`COPY (SELECT * FROM %s) TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD)`, t, path)
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			errs = append(errs, fmt.Errorf("results: export %s: %w", t, err))
		}
	}
	s.closeDB()
	return errors.Join(errs...)
}

func (s *Sink) closeDB() {
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
}

// Query opens a fresh in-memory DuckDB with a view per table over
// <root>/*/<table>.parquet (root is typically results/runs), plus
// samples_measured: samples minus the PhaseSetup requests, which is what
// latency reports should read. Callers close the returned DB. Tables with no
// parquet file yet are skipped.
func Query(ctx context.Context, root string) (*sql.DB, error) {
	connector, err := duckdb.NewConnector("", nil)
	if err != nil {
		return nil, fmt.Errorf("results: open duckdb: %w", err)
	}
	db := sql.OpenDB(connector)
	for _, t := range Tables {
		glob := filepath.Join(root, "*", t+".parquet")
		matches, _ := filepath.Glob(glob)
		if len(matches) == 0 {
			continue
		}
		q := fmt.Sprintf(`CREATE VIEW %s AS SELECT * FROM read_parquet('%s', union_by_name = true)`, t, glob)
		if _, err := db.ExecContext(ctx, q); err != nil {
			db.Close()
			return nil, fmt.Errorf("results: view %s: %w", t, err)
		}
		if t == "samples" {
			q := fmt.Sprintf(`CREATE VIEW samples_measured AS SELECT * FROM samples WHERE phase <> '%s'`, PhaseSetup)
			if _, err := db.ExecContext(ctx, q); err != nil {
				db.Close()
				return nil, fmt.Errorf("results: view samples_measured: %w", err)
			}
		}
	}
	return db, nil
}
