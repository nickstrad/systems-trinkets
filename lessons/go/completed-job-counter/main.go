package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/postgres"
	"github.com/nickstrad/systems-trinkets/lessons/go/completed-job-counter/core"
)

const (
	uniqueEvents = 100
	deliveries   = 3
)

// Expected totals belong to this experiment, not the reusable strategies.
var strategies = []struct {
	core.Strategy
	wantTotal int
}{
	{core.Naive(), uniqueEvents * deliveries},
	{core.Idempotent(), uniqueEvents},
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	db := postgres.Connect(ctx)
	defer db.Close(ctx)

	_, err := db.Exec(ctx, `
		create table if not exists job_counter (
			mode text primary key,
			total integer not null default 0
		);
		create table if not exists job_seen (
			event_id integer primary key
		);
	`)
	lab.Check(err)

	// Run each strategy once before measuring so pgx has already prepared its
	// statements; otherwise the first timed row of each mode includes that work.
	reset(ctx, db)
	for _, s := range strategies {
		_, _, err := core.Apply(ctx, db, s.Strategy, 0)
		lab.Check(err)
	}
	reset(ctx, db)

	out := lab.NewMeasurements(
		"mode", "event_id", "attempt", "applied", "observed_total", "latency_ms",
	)

	for _, s := range strategies {
		var total int
		for id := 1; id <= uniqueEvents; id++ {
			for attempt := 1; attempt <= deliveries; attempt++ {
				total = deliver(ctx, db, out, s.Strategy, id, attempt, total)
			}
		}

		fmt.Printf("%s: expected=%d actual=%d overcount=%d\n", s.Name, uniqueEvents, total, total-uniqueEvents)
		if total != s.wantTotal {
			panic(fmt.Sprintf("invariant failed: %s total is %d, want %d", s.Name, total, s.wantTotal))
		}
	}
	out.Close()
}

// reset empties both tables and seeds one counter row per strategy.
func reset(ctx context.Context, db *pgx.Conn) {
	names := make([]string, len(strategies))
	for i, s := range strategies {
		names[i] = s.Name
	}
	_, err := db.Exec(ctx, `truncate job_counter, job_seen`)
	lab.Check(err)
	_, err = db.Exec(ctx, `insert into job_counter(mode) select unnest($1::text[])`, names)
	lab.Check(err)
}

// deliver times one delivery of an event, records it as a CSV row, and
// returns the counter total after it. A skipped delivery leaves the total at
// prevTotal: this lab is the only writer.
func deliver(ctx context.Context, db *pgx.Conn, out *lab.Measurements, s core.Strategy, id, attempt, prevTotal int) int {
	start := time.Now()
	total, applied, err := core.Apply(ctx, db, s, id)
	lab.Check(err)
	elapsed := time.Since(start)

	appliedCol := "0"
	if applied {
		appliedCol = "1"
	} else {
		total = prevTotal
	}
	out.Write(
		s.Name,
		strconv.Itoa(id),
		strconv.Itoa(attempt),
		appliedCol,
		strconv.Itoa(total),
		strconv.FormatFloat(lab.Ms(elapsed), 'f', 3, 64),
	)
	return total
}
