package storetest

import (
	"context"
	"slices"
	"sync"
	"testing"
)

// Store describes the behavior tested here independently of the implementation.
// Structural typing lets same-package counter tests use this helper without a cycle.
type Store interface {
	Incr(context.Context, string, int64) (int64, error)
	Get(context.Context, string) (int64, error)
	Del(context.Context, string) error
	Reset(context.Context) error
	Ping(context.Context) error
	Close() error
}

// Run is the conformance test every Store implementation runs: the
// sequential semantics of each method and the atomicity of Incr under
// concurrent callers (the primitive behind INV-COUNTER-01/02/05). open must
// return a store that this test may wipe; the store is closed on cleanup.
func Run[S Store](t *testing.T, open func() S) {
	t.Helper()
	ctx := context.Background()
	s := open()
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if err := s.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	t.Run("sequential", func(t *testing.T) {
		MustGet(t, s, "a", 0)
		if v, err := s.Incr(ctx, "a", 1); err != nil || v != 1 {
			t.Fatalf("Incr(a, 1) = %d, %v; want 1", v, err)
		}
		if v, err := s.Incr(ctx, "a", 5); err != nil || v != 6 {
			t.Fatalf("Incr(a, 5) = %d, %v; want 6", v, err)
		}
		MustGet(t, s, "a", 6)
		MustGet(t, s, "b", 0)
		if v, err := s.Incr(ctx, "b", 42); err != nil || v != 42 {
			t.Fatalf("Incr(b, 42) = %d, %v; want 42", v, err)
		}
		MustGet(t, s, "b", 42)
		if err := s.Del(ctx, "a"); err != nil {
			t.Fatalf("Del: %v", err)
		}
		MustGet(t, s, "a", 0)
		MustGet(t, s, "b", 42)
		if err := s.Del(ctx, "never"); err != nil {
			t.Fatalf("Del of unknown name: %v", err)
		}
		if err := s.Reset(ctx); err != nil {
			t.Fatalf("Reset: %v", err)
		}
		MustGet(t, s, "a", 0)
		MustGet(t, s, "b", 0)
		if v, err := s.Incr(ctx, "a", 3); err != nil || v != 3 {
			t.Fatalf("Incr after Reset = %d, %v; want 3", v, err)
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		if err := s.Reset(ctx); err != nil {
			t.Fatalf("Reset: %v", err)
		}
		const workers, perWorker = 32, 200
		const n = workers * perWorker
		values := ConcurrentIncr(t, s, "hot", workers, perWorker)
		MustGet(t, s, "hot", n)
		slices.Sort(values)
		for i, v := range values {
			if v != int64(i+1) {
				t.Fatalf("returned values are not a permutation of 1..%d: sorted[%d] = %d", n, i, v)
			}
		}
		MustGet(t, s, "cold", 0)
	})

	t.Run("isolation", func(t *testing.T) {
		if err := s.Reset(ctx); err != nil {
			t.Fatalf("Reset: %v", err)
		}
		names := []string{"iso-a", "iso-b", "iso-c", "iso-d"}
		var wg sync.WaitGroup
		for _, name := range names {
			wg.Go(func() { ConcurrentIncr(t, s, name, 8, 100) })
		}
		wg.Wait()
		for _, name := range names {
			MustGet(t, s, name, 800)
		}
	})
}

// ConcurrentIncr runs workers×perWorker Incr(name, 1) calls at once and
// returns every value they got back.
func ConcurrentIncr(t *testing.T, s Store, name string, workers, perWorker int) []int64 {
	t.Helper()
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		values = make([]int64, 0, workers*perWorker)
	)
	for range workers {
		wg.Go(func() {
			got := make([]int64, 0, perWorker)
			for range perWorker {
				v, err := s.Incr(context.Background(), name, 1)
				if err != nil {
					t.Errorf("Incr: %v", err)
					return
				}
				got = append(got, v)
			}
			mu.Lock()
			values = append(values, got...)
			mu.Unlock()
		})
	}
	wg.Wait()
	return values
}

func MustGet(t *testing.T, s Store, name string, want int64) {
	t.Helper()
	v, err := s.Get(context.Background(), name)
	if err != nil {
		t.Fatalf("Get(%s): %v", name, err)
	}
	if v != want {
		t.Fatalf("Get(%s) = %d, want %d", name, v, want)
	}
}
