// Package store opens a configured counter engine. Driver implementations stay internal.
package store

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"systems-trinkets/examples/counter"
	"systems-trinkets/examples/counter/internal/store/postgres"
	"systems-trinkets/examples/counter/internal/store/sqlite"
	"systems-trinkets/examples/counter/internal/store/valkey"
)

// Default DSNs per engine: the sqlite file comes from sqliteDSN, the others
// are the ../../harness/infra/ compose services on their default ports.
const (
	defaultValkeyDSN   = "redis://127.0.0.1:6379/1"
	defaultPostgresDSN = "postgres://trinkets:trinkets@127.0.0.1:5432/trinkets?sslmode=disable"
)

// Open picks the engine, filling in that engine's default DSN when --dsn
// is empty. Each engine is its own package, so this is the only place they are
// linked in.
func Open(engine, dsn, addr string) (counter.Store, error) {
	switch engine {
	case "memory":
		return counter.NewMemory(), nil
	case "sqlite":
		if dsn == "" {
			dsn = sqliteDSN(addr)
		}
		log.Printf("sqlite dsn=%s", dsn)
		return sqlite.Open(dsn)
	case "valkey":
		if dsn == "" {
			dsn = defaultValkeyDSN
		}
		return valkey.Open(dsn)
	case "postgres":
		if dsn == "" {
			dsn = defaultPostgresDSN
		}
		return postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unknown --engine %q", engine)
	}
}

// sqliteDSN is the default SQLite file for a SUT on addr: one stable path per
// listen address under the temp dir, created on first use and never deleted.
// Stable is the point — a crash test restarts the same target and must find
// the increments the killed process committed, so the path may not vary per
// start; per-address keeps four engines' SUTs from sharing one file.
func sqliteDSN(addr string) string {
	safe := strings.NewReplacer(":", "-", "/", "-", string(filepath.Separator), "-").Replace(addr)
	return filepath.Join(os.TempDir(), "counter-sqlite-"+safe+".db")
}
