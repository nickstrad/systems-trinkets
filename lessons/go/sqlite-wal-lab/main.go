package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/lessons/go/sqlite-wal-lab/core"
)

const trials = 3

// measurement is one timed write made while a reader holds a snapshot open.
type measurement struct {
	writeMs float64
	result  string
}

func main() {
	results := lab.NewMeasurements("mode", "trial", "write_ms", "result")

	for _, mode := range []string{"DELETE", "WAL"} {
		for trial := 1; trial <= trials; trial++ {
			m := run(mode, trial)
			results.Write(
				mode,
				strconv.Itoa(trial),
				strconv.FormatFloat(m.writeMs, 'f', 2, 64),
				m.result,
			)
		}
	}

	results.Close()
}

func run(mode string, trial int) measurement {
	ctx := context.Background()

	// A fresh directory per run keeps one run's -wal and -shm files from
	// leaking into the next.
	dir, err := os.MkdirTemp("", "wal-lab-")
	lab.Check(err)
	defer os.RemoveAll(dir)

	db, err := sql.Open("sqlite", core.DSN(filepath.Join(dir, "events.db"), mode, 2000))
	lab.Check(err)
	defer db.Close()

	_, err = db.ExecContext(ctx, core.Schema)
	lab.Check(err)

	// Reading establishes a snapshot, held until the deferred rollback.
	tx, count, err := core.OpenSnapshot(ctx, db)
	lab.Check(err)
	defer tx.Rollback()
	fmt.Printf("\n%s trial %d: reader snapshot open (%d rows)\n", mode, trial, count)

	write, err := core.WriteEvent(ctx, db)
	lab.Check(err)
	fmt.Printf("write took: %v\n", write.Elapsed)

	result := "ok"
	if write.WriteErr != nil {
		result = write.WriteErr.Error() // The measured outcome, not a lab failure.
	}
	return measurement{lab.Ms(write.Elapsed), result}
}
