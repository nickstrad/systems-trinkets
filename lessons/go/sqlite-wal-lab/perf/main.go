// HTTP adapter for the sqlite-wal-lab lesson: the base runner's two journal
// modes side by side in one server, chosen per request, so one k6 run
// reproduces the base table. No backing service: SQLite files live in a
// temporary directory that normal shutdown removes.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/perf"
	"github.com/nickstrad/systems-trinkets/lessons/go/sqlite-wal-lab/core"
)

// variant is one journal mode: its own database file, the reader snapshot
// held open on it for the server's lifetime, and its outcome counters.
type variant struct {
	name        string // journal mode, as main.go names it
	path        string
	db          *sql.DB
	journalMode string // what `pragma journal_mode` reports, to prove the DSN took effect

	snapMu   sync.Mutex // a *sql.Tx is not safe for concurrent use
	snapshot *sql.Tx
	seedRows int // rows the snapshot saw when it opened
	ok       atomic.Int64
	locked   atomic.Int64 // SQLITE_BUSY or SQLITE_LOCKED: the measured outcome
	failed   atomic.Int64 // any other write error
}

func main() {
	ctx := context.Background()

	// BUSY_MS is the base lesson's busy timeout (2000 ms in main.go). 250 keeps
	// DELETE writes under the 1500 ms p95 budget and below lock saturation at
	// the default load rate; see perf/README.md.
	busyMs := perf.Int("BUSY_MS", 250, 0, 4000)
	// POOL_SIZE is writer connections per database; the held snapshot pins one
	// more, so writers never compete with the reader for a connection. More
	// concurrent writers than POOL_SIZE queue in database/sql, which shows in
	// HTTP time but not in elapsed_ms.
	poolSize := perf.Int("POOL_SIZE", 8, 1, 64)

	dir, err := os.MkdirTemp("", "sqlite-wal-perf-")
	lab.Check(err)
	fmt.Println("temporary directory:", dir)
	defer func() {
		lab.Check(os.RemoveAll(dir))
		fmt.Println("removed", dir)
	}()

	variants := []*variant{{name: "DELETE"}, {name: "WAL"}}
	byName := map[string]*variant{}
	for _, v := range variants {
		open(ctx, v, dir, busyMs, poolSize)
		defer v.db.Close()
		defer v.snapshot.Rollback() // runs before Close: release the reader first
		byName[v.name] = v
		fmt.Printf("%s: journal_mode=%s, reader snapshot open (%d rows)\n", v.name, v.journalMode, v.seedRows)
	}

	mux := http.NewServeMux()
	// The cap bounds the WAL file: the held snapshot stops checkpoints from
	// reusing it, so every successful WAL write grows it.
	mux.HandleFunc("POST /operation", perf.Limit(10000, func(w http.ResponseWriter, r *http.Request) {
		v, ok := byName[r.URL.Query().Get("variant")]
		if !ok {
			perf.Fail(w, 400, "variant must be DELETE or WAL")
			return
		}
		// WriteEvent times only lock wait plus the insert, as in main.go;
		// waiting for a pool connection and preparing stay outside elapsed_ms.
		write, err := core.WriteEvent(r.Context(), v.db)
		if err != nil {
			v.failed.Add(1)
			perf.Fail(w, 503, err.Error())
			return
		}
		resp := map[string]any{"variant": v.name, "outcome": "ok", "elapsed_ms": lab.Ms(write.Elapsed)}
		switch {
		case write.WriteErr == nil:
			v.ok.Add(1)
		case isLocked(write.WriteErr):
			// The lesson's finding, not a broken request: answer 200 with it.
			v.locked.Add(1)
			resp["outcome"] = "locked"
			resp["error"] = write.WriteErr.Error() // main.go's result column
		default:
			v.failed.Add(1)
			perf.Fail(w, 500, write.WriteErr.Error())
			return
		}
		perf.JSON(w, 200, resp)
	}))
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		rows := make([]map[string]any, 0, len(variants))
		for _, v := range variants {
			s, err := v.stats(ctx)
			if err != nil {
				perf.Fail(w, 503, v.name+": "+err.Error())
				return
			}
			rows = append(rows, s)
		}
		perf.JSON(w, 200, map[string]any{"variants": rows})
	})
	perf.Serve(mux)
}

// open creates the variant's database the way main.go does, then holds a
// reader snapshot on it until shutdown: every write the server serves
// arrives while that reader is open, as the base runner's one write does.
func open(ctx context.Context, v *variant, dir string, busyMs, poolSize int) {
	v.path = filepath.Join(dir, v.name+".db")
	db, err := sql.Open("sqlite", core.DSN(v.path, v.name, busyMs))
	lab.Check(err)
	db.SetMaxOpenConns(poolSize + 1)
	db.SetMaxIdleConns(poolSize + 1) // keep connections; the default of 2 would churn them
	v.db = db
	_, err = db.ExecContext(ctx, core.Schema)
	lab.Check(err)
	lab.Check(db.QueryRowContext(ctx, "pragma journal_mode").Scan(&v.journalMode))
	warm(ctx, db, poolSize+1)
	v.snapshot, v.seedRows, err = core.OpenSnapshot(ctx, db)
	lab.Check(err)
}

// warm opens n connections and has each read the table before any traffic.
// A new connection runs the DSN pragmas and loads the schema, and both need a
// SHARED lock; in DELETE mode a writer waiting for EXCLUSIVE holds PENDING,
// which blocks new SHARED locks. Unwarmed, a request that opened a connection
// under contention failed inside WriteEvent's setup (Conn or Prepare) with
// SQLITE_BUSY after BUSY_MS, before its timer started. Warm connections stay
// in the pool because max idle equals max open and nothing expires them.
func warm(ctx context.Context, db *sql.DB, n int) {
	conns := make([]*sql.Conn, n)
	for i := range conns {
		c, err := db.Conn(ctx)
		lab.Check(err)
		var rows int
		lab.Check(c.QueryRowContext(ctx, "select count(*) from events").Scan(&rows))
		conns[i] = c
	}
	for _, c := range conns {
		lab.Check(c.Close()) // returns it to the pool
	}
}

// isLocked reports whether a write failed on a lock: SQLITE_BUSY after the
// busy timeout (or at once, when waiting could deadlock) or SQLITE_LOCKED.
func isLocked(err error) bool {
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return false
	}
	code := e.Code() & 0xff // primary code; extended codes add high bits
	return code == sqlite3.SQLITE_BUSY || code == sqlite3.SQLITE_LOCKED
}

// stats reports both sides of the invariant: rows the database holds now
// (a fresh read) vs seed rows plus successful writes, and what the held
// snapshot still sees (snapshot isolation: the seed count).
func (v *variant) stats(ctx context.Context) (map[string]any, error) {
	var rows int
	if err := v.db.QueryRowContext(ctx, "select count(*) from events").Scan(&rows); err != nil {
		return nil, err
	}
	v.snapMu.Lock()
	var snapshotRows int
	err := v.snapshot.QueryRowContext(ctx, "select count(*) from events").Scan(&snapshotRows)
	v.snapMu.Unlock()
	if err != nil {
		return nil, err
	}
	walBytes := int64(0) // DELETE mode has no -wal file
	if info, err := os.Stat(v.path + "-wal"); err == nil {
		walBytes = info.Size()
	}
	ok := v.ok.Load()
	return map[string]any{
		"variant":       v.name,
		"journal_mode":  v.journalMode,
		"writes_ok":     ok,
		"writes_locked": v.locked.Load(),
		"writes_failed": v.failed.Load(),
		"seed_rows":     v.seedRows,
		"expected_rows": int64(v.seedRows) + ok,
		"rows":          rows,
		"snapshot_rows": snapshotRows,
		"wal_bytes":     walBytes,
	}, nil
}
