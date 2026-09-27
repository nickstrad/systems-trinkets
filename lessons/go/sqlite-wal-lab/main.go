package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nickstrad/systems-trinkets/internal/lab"
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

	// Pragmas in the DSN run on every connection the pool opens. With db.Exec
	// they would reach only whichever pooled connection ran them.
	dsn := "file:" + filepath.Join(dir, "events.db") +
		"?_pragma=busy_timeout(2000)&_pragma=journal_mode(" + mode + ")"
	db, err := sql.Open("sqlite", dsn)
	lab.Check(err)
	defer db.Close()

	_, err = db.ExecContext(ctx, `
create table events (
  id integer primary key,
  payload text
);
insert into events(payload) values ('seed');
`)
	lab.Check(err)

	reader, err := db.Conn(ctx)
	lab.Check(err)
	defer reader.Close()

	tx, err := reader.BeginTx(ctx, nil)
	lab.Check(err)
	defer tx.Rollback()

	// The first read starts the snapshot; the transaction holds it until Rollback.
	var count int
	lab.Check(tx.QueryRowContext(ctx, `select count(*) from events`).Scan(&count))
	fmt.Printf("\n%s trial %d: reader snapshot open (%d rows)\n", mode, trial, count)

	writer, err := db.Conn(ctx)
	lab.Check(err)
	defer writer.Close()

	// Preparing loads the schema and compiles the insert before the timer
	// starts, so the timer measures only the lock wait and the write.
	insert, err := writer.PrepareContext(ctx, `insert into events(payload) values ('new event')`)
	lab.Check(err)
	defer insert.Close()

	start := time.Now()
	_, err = insert.ExecContext(ctx)
	elapsed := time.Since(start)
	fmt.Printf("write took: %v\n", elapsed)

	result := "ok"
	if err != nil {
		result = err.Error() // The measured outcome, not a lab failure.
	}
	return measurement{lab.Ms(elapsed), result}
}
