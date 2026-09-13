package counterfault

import (
	"context"
	"testing"
	"time"

	"systems-trinkets/examples/counter"
)

func NewMemory() Store { return overwrite{counter.NewMemory()} }

func TestSlowStoreConforms(t *testing.T) {
	runStoreTests(t, func() Store { return Slow(NewMemory(), 100*time.Microsecond) })
}

func TestWriteBehindStoreConforms(t *testing.T) {
	runStoreTests(t, func() Store { return WriteBehind(NewMemory(), 5*time.Millisecond) })
}

// Each bug breaks exactly what it claims.

func TestLostUpdateBreaksOnlyConcurrency(t *testing.T) {
	ctx := context.Background()
	s := LostUpdate(NewMemory())

	// Sequentially it is indistinguishable from a correct store.
	if v, _ := s.Incr(ctx, "a", 1); v != 1 {
		t.Fatalf("Incr = %d, want 1", v)
	}
	if v, _ := s.Incr(ctx, "a", 5); v != 6 {
		t.Fatalf("Incr = %d, want 6", v)
	}
	mustGet(t, s, "a", 6)
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	mustGet(t, s, "a", 0)

	// Concurrently it loses increments.
	const attempts = 5
	for range attempts {
		_ = s.Reset(ctx)
		concurrentIncr(t, s, "race", 32, 200)
		if v, _ := s.Get(ctx, "race"); v < 32*200 {
			return
		}
	}
	t.Fatalf("lost-update bug did not lose any increments in %d attempts", attempts)
}

func TestDropResetBreaksOnlyReset(t *testing.T) {
	ctx := context.Background()
	s := DropReset(NewMemory())

	concurrentIncr(t, s, "a", 8, 50)
	mustGet(t, s, "a", 400)
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	mustGet(t, s, "a", 400) // the bug: reset reported ok and did nothing
	if err := s.Del(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, s, "a", 0) // Del still works
}

func TestWriteBehindLosesUnflushedWrites(t *testing.T) {
	ctx := context.Background()
	under := NewMemory()
	s := WriteBehind(under, time.Hour)  // never flushes on its own during the test
	t.Cleanup(func() { _ = s.Close() }) // idempotent; stops the flusher even if a check below fails
	w := s.(*writeBehind)

	if v, _ := s.Incr(ctx, "a", 3); v != 3 {
		t.Fatalf("Incr = %d, want 3", v)
	}
	mustGet(t, s, "a", 3)     // the client sees its write…
	mustGet(t, under, "a", 0) // …the store has not: a kill here loses it (INV-06)

	w.flushNow()
	mustGet(t, under, "a", 3)

	// Increments after a flush start from the shadow, not the store.
	if v, _ := s.Incr(ctx, "a", 1); v != 4 {
		t.Fatalf("Incr after flush = %d, want 4", v)
	}
	mustGet(t, under, "a", 3)

	// Del and Reset write through immediately; a later Incr reloads from the store.
	if err := s.Del(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, under, "a", 0)
	mustGet(t, s, "a", 0)
	_ = under.Set(ctx, "b", 10)
	if v, _ := s.Incr(ctx, "b", 1); v != 11 {
		t.Fatalf("Incr on a name only the store knows = %d, want 11", v)
	}
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	mustGet(t, under, "b", 0)
	mustGet(t, s, "b", 0)

	// Close flushes what is pending: only a kill loses data.
	_, _ = s.Incr(ctx, "c", 7)
	mustGet(t, under, "c", 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	mustGet(t, under, "c", 7)
}

func TestSlowAddsLatency(t *testing.T) {
	const d = 20 * time.Millisecond
	s := Slow(NewMemory(), d)
	start := time.Now()
	if _, err := s.Incr(context.Background(), "a", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := time.Since(start); got < 2*d {
		t.Fatalf("two ops took %v, want ≥ %v", got, 2*d)
	}
}
