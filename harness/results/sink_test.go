package results

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSinkExportsParquet(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	run := RunRow{RunID: NewRunID(), Pattern: "counter", Language: "go", Engine: "memory", TargetURL: "http://x"}
	dir := filepath.Join(root, run.RunID)

	s, err := Open(ctx, dir, run)
	if err != nil {
		t.Fatal(err)
	}

	const workers, perWorker = 8, 12_500 // 100k samples
	start := time.Now()
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range perWorker {
				s.Sample(SampleRow{RunID: run.RunID, Test: "TestX", Phase: "main", Worker: w, Seq: i,
					Method: "POST", PathTemplate: "/counter/incr", Status: 200,
					LatencyNS: int64(1000 * (i%100 + 1)), StartedAt: time.Now()})
			}
		})
	}
	wg.Wait()
	s.Test(TestRow{RunID: run.RunID, Test: "TestX", Status: "pass", DurationNS: 42})
	s.Check(CheckRow{RunID: run.RunID, Test: "TestX", InvariantID: "INV-COUNTER-01", OK: true,
		Message: "final value equals increments", DetailsJSON: JSON(map[string]any{"got": 1, "want": 1}), At: time.Now()})
	s.Metric(MetricRow{RunID: run.RunID, Test: "TestX", Name: "throughput", Value: 12.5, Unit: "req/s", LabelsJSON: "{}", At: time.Now()})
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > time.Second {
		t.Errorf("100k samples took %v, want < 1s", el)
	}
	s.Sample(SampleRow{}) // after Close: dropped, no panic

	for _, tb := range Tables {
		if _, err := os.Stat(filepath.Join(dir, tb+".parquet")); err != nil {
			t.Errorf("missing %s.parquet: %v", tb, err)
		}
	}

	db, err := Query(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int64
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM samples_measured`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != workers*perWorker {
		t.Errorf("samples = %d, want %d", n, workers*perWorker)
	}
	var p99 float64
	if err := db.QueryRowContext(ctx, `SELECT quantile_cont(latency_ns, 0.99) FROM samples`).Scan(&p99); err != nil {
		t.Fatal(err)
	}
	if p99 < 98_000 || p99 > 100_000 {
		t.Errorf("p99 = %v, want ~99000", p99)
	}
	var finished time.Time
	var status string
	if err := db.QueryRowContext(ctx, `SELECT r.finished_at, t.status FROM runs r JOIN tests t USING (run_id)`).Scan(&finished, &status); err != nil {
		t.Fatal(err)
	}
	if finished.IsZero() || status != "pass" {
		t.Errorf("run/test rows: finished=%v status=%q", finished, status)
	}
	var got int64
	if err := db.QueryRowContext(ctx, `SELECT json_extract(details, '$.got')::BIGINT FROM checks`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("details.got = %d, want 1", got)
	}
}

func TestNewRunIDSortable(t *testing.T) {
	a := NewRunID()
	time.Sleep(1100 * time.Millisecond)
	b := NewRunID()
	if !(a < b) {
		t.Errorf("%s should sort before %s", a, b)
	}
	if len(a) != len("20060102T150405Z-abcd") {
		t.Errorf("unexpected format %q", a)
	}
}

func TestConcurrentCloseWaitsForExport(t *testing.T) {
	root := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(root, "run"), RunRow{RunID: "run"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 10_000 {
		s.Sample(SampleRow{RunID: "run", Seq: i, StartedAt: time.Now()})
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := s.Close(context.Background()); err != nil {
				t.Error(err)
				return
			}
			// Each returning caller must observe completed export, even if
			// another caller won the race to initiate it.
			for _, table := range Tables {
				if _, err := os.Stat(filepath.Join(root, "run", table+".parquet")); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}
