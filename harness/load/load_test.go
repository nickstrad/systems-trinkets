package load

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"systems-trinkets/harness/harness"
	"systems-trinkets/harness/hx"
)

func newH(t *testing.T) *harness.H {
	return &harness.H{T: t, Ctx: context.Background()}
}

// TestClosedBarrier proves all workers are parked before any of them runs
// fn: each worker blocks, on its first fn call, until all `workers` of them
// have reported in. If they were released one at a time (no barrier) rather
// than all together, this would time out.
func TestClosedBarrier(t *testing.T) {
	h := newH(t)
	const workers, iters = 16, 3

	var arrivedCount atomic.Int64
	var firstCall sync.Map // worker id -> struct{}
	allArrived := make(chan struct{})
	var closeOnce sync.Once
	var timedOut atomic.Bool

	res := Closed(h, workers, iters, func(w *Worker) error {
		if _, seen := firstCall.LoadOrStore(w.ID, struct{}{}); !seen {
			if arrivedCount.Add(1) == int64(workers) {
				closeOnce.Do(func() { close(allArrived) })
			}
			select {
			case <-allArrived:
			case <-time.After(2 * time.Second):
				timedOut.Store(true)
			}
		}
		return nil
	})

	if timedOut.Load() {
		t.Fatal("not all workers were parked concurrently before fn ran (barrier did not release them together)")
	}
	if res.Total != 0 {
		t.Fatalf("Total = %d, want 0 (fn never touches the counter)", res.Total)
	}
}

// TestCountsAddUp checks Total calls, FnErrors, and the 100-entry Errs cap.
func TestCountsAddUp(t *testing.T) {
	h := newH(t)
	const workers, iters = 8, 50 // 400 calls total

	var calls atomic.Int64
	res := Closed(h, workers, iters, func(w *Worker) error {
		n := calls.Add(1)
		if n%2 == 0 {
			return errors.New("boom")
		}
		return nil
	})

	wantCalls := int64(workers * iters)
	if got := calls.Load(); got != wantCalls {
		t.Fatalf("fn called %d times, want %d", got, wantCalls)
	}
	wantErrs := int(wantCalls) / 2 // every even-numbered call errors
	if res.FnErrors != wantErrs {
		t.Fatalf("FnErrors = %d, want %d", res.FnErrors, wantErrs)
	}
	wantKept := wantErrs
	if wantKept > maxErrs {
		wantKept = maxErrs
	}
	if len(res.Errs) != wantKept {
		t.Fatalf("len(Errs) = %d, want %d", len(res.Errs), wantKept)
	}
}

// TestErrsCappedAt100 forces more than 100 fn errors and checks Errs is
// capped while FnErrors still counts them all.
func TestErrsCappedAt100(t *testing.T) {
	h := newH(t)
	const workers, iters = 10, 20 // 200 calls, all erroring

	res := Closed(h, workers, iters, func(w *Worker) error {
		return errors.New("always fails")
	})

	if res.FnErrors != workers*iters {
		t.Fatalf("FnErrors = %d, want %d", res.FnErrors, workers*iters)
	}
	if len(res.Errs) != 100 {
		t.Fatalf("len(Errs) = %d, want 100", len(res.Errs))
	}
}

// TestCounterShared proves every worker's ctx carries the same *hx.Counter,
// and that Result is built from it.
func TestCounterShared(t *testing.T) {
	h := newH(t)
	const workers, iters = 4, 5

	res := Closed(h, workers, iters, func(w *Worker) error {
		c := hx.CounterFrom(w.Ctx)
		if c == nil {
			t.Fatal("hx.CounterFrom(w.Ctx) is nil")
		}
		c.Total.Add(1)
		c.OK2xx.Add(1)
		return nil
	})

	want := workers * iters
	if res.Total != want {
		t.Fatalf("Total = %d, want %d", res.Total, want)
	}
	if res.OK2xx != want {
		t.Fatalf("OK2xx = %d, want %d", res.OK2xx, want)
	}
}

// TestIterPerWorker checks w.Iter runs 0..iters-1 per worker.
func TestIterPerWorker(t *testing.T) {
	h := newH(t)
	const workers, iters = 6, 10

	var mu sync.Mutex
	seen := make(map[int][]int, workers)

	Closed(h, workers, iters, func(w *Worker) error {
		s := w.Iter
		mu.Lock()
		seen[w.ID] = append(seen[w.ID], s)
		mu.Unlock()
		return nil
	})

	if len(seen) != workers {
		t.Fatalf("saw %d distinct workers, want %d", len(seen), workers)
	}
	for id, seq := range seen {
		if len(seq) != iters {
			t.Fatalf("worker %d: got %d calls, want %d", id, len(seq), iters)
		}
		for i, v := range seq {
			if v != i {
				t.Fatalf("worker %d: call %d had Iter %d, want %d", id, i, v, i)
			}
		}
	}
}

// TestClosedStopsOnCancel proves Closed with a huge iteration count returns
// promptly once h.Ctx is cancelled.
func TestClosedStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness.H{T: t, Ctx: ctx}

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	res := Closed(h, 8, 1_000_000_000, func(w *Worker) error {
		return nil
	})
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("Closed took %v after cancel, want well under 2s", elapsed)
	}
	if res.FnErrors != 0 {
		t.Fatalf("FnErrors = %d, want 0", res.FnErrors)
	}
}

// TestClosedForDuration checks ClosedFor returns after approximately d.
func TestClosedForDuration(t *testing.T) {
	h := newH(t)
	const d = 100 * time.Millisecond

	start := time.Now()
	res := ClosedFor(h, 4, d, func(w *Worker) error {
		return nil
	})
	elapsed := time.Since(start)

	if elapsed < d {
		t.Fatalf("ClosedFor returned after %v, want at least %v", elapsed, d)
	}
	if elapsed > d+2*time.Second {
		t.Fatalf("ClosedFor returned after %v, want close to %v", elapsed, d)
	}
	if res.Elapsed < d {
		t.Fatalf("res.Elapsed = %v, want at least %v", res.Elapsed, d)
	}
}

// TestClosedForStopsOnParentCancel proves ClosedFor also stops early when
// h.Ctx is cancelled, not only on its own timeout.
func TestClosedForStopsOnParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness.H{T: t, Ctx: ctx}

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	ClosedFor(h, 4, time.Hour, func(w *Worker) error {
		return nil
	})
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("ClosedFor took %v after parent cancel, want well under 2s", elapsed)
	}
}

// TestPhase checks Phase sets and restores h.Ctx, including on panic.
func TestPhase(t *testing.T) {
	h := newH(t)

	if got := hx.PhaseFrom(h.Ctx); got != "" {
		t.Fatalf("phase before Phase() = %q, want \"\"", got)
	}

	var inside string
	Phase(h, "warmup", func() {
		inside = hx.PhaseFrom(h.Ctx)
	})

	if inside != "warmup" {
		t.Fatalf("phase inside Phase() = %q, want %q", inside, "warmup")
	}
	if got := hx.PhaseFrom(h.Ctx); got != "" {
		t.Fatalf("phase after Phase() = %q, want \"\"", got)
	}
}

// TestPhaseRestoresOnPanic checks Phase restores h.Ctx even if fn panics.
func TestPhaseRestoresOnPanic(t *testing.T) {
	h := newH(t)

	func() {
		defer func() {
			recover()
		}()
		Phase(h, "main", func() {
			panic("boom")
		})
	}()

	if got := hx.PhaseFrom(h.Ctx); got != "" {
		t.Fatalf("phase after panicking Phase() = %q, want \"\"", got)
	}
}
