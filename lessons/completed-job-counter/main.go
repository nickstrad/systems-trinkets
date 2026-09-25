package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultDSN   = "postgres://trinkets:trinkets@localhost:5432/trinkets"
	uniqueEvents = 100
	deliveries   = 3
)

// A strategy decides whether one delivery of an event may increment the
// counter row named after it.
type strategy struct {
	name      string
	wantTotal int
	claim     func(ctx context.Context, tx pgx.Tx, id int) bool
}

var strategies = []strategy{
	{
		name:      "naive",
		wantTotal: uniqueEvents * deliveries, // Counts delivery attempts.
		claim:     func(context.Context, pgx.Tx, int) bool { return true },
	},
	{
		name:      "idempotent",
		wantTotal: uniqueEvents, // Counts each unique event once.
		claim: func(ctx context.Context, tx pgx.Tx, id int) bool {
			tag, err := tx.Exec(ctx, `
			insert into job_seen(event_id) values ($1)
			on conflict (event_id) do nothing`, id)
			check(err)
			return tag.RowsAffected() == 1
		},
	},
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = defaultDSN
	}

	db, err := pgx.Connect(ctx, dsn)
	check(err)
	defer db.Close(ctx)

	_, err = db.Exec(ctx, `
		create table if not exists job_counter (
			mode text primary key,
			total integer not null default 0
		);
		create table if not exists job_seen (
			event_id integer primary key
		);
	`)
	check(err)

	// Run each strategy once before measuring so pgx has already prepared its
	// statements; otherwise the first timed row of each mode includes that work.
	reset(ctx, db)
	for _, s := range strategies {
		apply(ctx, db, s, 0)
	}
	reset(ctx, db)

	file, err := os.Create("measurements.csv")
	check(err)
	out := csv.NewWriter(file)
	check(out.Write([]string{
		"mode", "event_id", "attempt", "applied", "observed_total", "latency_ms",
	}))

	for _, s := range strategies {
		var total int
		for id := 1; id <= uniqueEvents; id++ {
			for attempt := 1; attempt <= deliveries; attempt++ {
				total = deliver(ctx, db, out, s, id, attempt, total)
			}
		}

		fmt.Printf("%s: expected=%d actual=%d overcount=%d\n", s.name, uniqueEvents, total, total-uniqueEvents)
		if total != s.wantTotal {
			panic(fmt.Sprintf("invariant failed: %s total is %d, want %d", s.name, total, s.wantTotal))
		}
	}
	out.Flush()
	check(out.Error())
	check(file.Close())
	fmt.Println("Wrote measurements.csv")
}

// reset empties both tables and seeds one counter row per strategy.
func reset(ctx context.Context, db *pgx.Conn) {
	names := make([]string, len(strategies))
	for i, s := range strategies {
		names[i] = s.name
	}
	_, err := db.Exec(ctx, `truncate job_counter, job_seen`)
	check(err)
	_, err = db.Exec(ctx, `insert into job_counter(mode) select unnest($1::text[])`, names)
	check(err)
}

// deliver times one delivery of an event, records it as a CSV row, and
// returns the counter total after it. A skipped delivery leaves the total at
// prevTotal: this lab is the only writer.
func deliver(ctx context.Context, db *pgx.Conn, out *csv.Writer, s strategy, id, attempt, prevTotal int) int {
	start := time.Now()
	total, applied := apply(ctx, db, s, id)
	elapsed := time.Since(start)

	appliedCol := "0"
	if applied {
		appliedCol = "1"
	} else {
		total = prevTotal
	}
	check(out.Write([]string{
		s.name,
		strconv.Itoa(id),
		strconv.Itoa(attempt),
		appliedCol,
		strconv.Itoa(total),
		strconv.FormatFloat(ms(elapsed), 'f', 3, 64),
	}))
	return total
}

// apply runs one delivery in a transaction. When the strategy claims the
// delivery, it increments the counter and returns the new total.
func apply(ctx context.Context, db *pgx.Conn, s strategy, id int) (total int, applied bool) {
	tx, err := db.Begin(ctx)
	check(err)
	defer tx.Rollback(ctx)

	if s.claim(ctx, tx, id) {
		err := tx.QueryRow(ctx, `
		update job_counter set total = total + 1 where mode = $1
		returning total`, s.name).Scan(&total)
		if err == pgx.ErrNoRows {
			panic("counter row is missing")
		}
		check(err)
		applied = true
	}

	check(tx.Commit(ctx))
	return total, applied
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func check(err error) {
	if err != nil {
		panic(err) // Fail fast; never silently report database errors as success.
	}
}
