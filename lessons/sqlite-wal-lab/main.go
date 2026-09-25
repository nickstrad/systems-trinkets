package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"time"
	"context"
)

func main() {
	out, _ := os.Create("measurements.csv")
	defer out.Close()

	csv := csv.NewWriter(out)
	defer csv.Flush()
	csv.Write([]string{"mode", "write_ms", "result"})

	run("DELETE", csv)
	run("WAL", csv)
}

func run(mode string, results *csv.Writer) {
	os.Remove("events.db")
	db, _ := sql.Open("sqlite", "events.db")

	defer db.Close()

	db.Exec("PRAGMA busy_timeout = 2000")
	db.Exec("PRAGMA journal_mode = " + mode)

	db.Exec(`
create table events (
  id integer primary key,
  payload text
);
`)

	db.Exec(`insert into events(payload) values ('seed');`)

	ctx := context.Background()
	reader, _ := db.Conn(ctx)
	defer reader.Close()

	tx, _ := reader.BeginTx(ctx, nil)

	var count int
	tx.QueryRow(`select count(*) from events;`).Scan(&count)
	fmt.Printf("\n%s: reader snapshot open\n", mode)

	writer, _ := db.Conn(ctx)

	defer writer.Close()
	start := time.Now()

	_, err := writer.ExecContext(
		nil,
		`insert into events(payload) values ('new event')`,
	)

	elapsed := time.Since(start)

	result := "ok"

	if err != nil {
		result = err.Error()
	}

	results.Write([]string{
		mode,
		fmt.Sprintf("%.2f", float64(elapsed.Microseconds())/1000),
		result,
	})

	fmt.Printf("write took: %v\n", elapsed)
	tx.Rollback()
}
