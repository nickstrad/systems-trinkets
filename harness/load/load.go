// Package load provides concurrency shapes for driving a server under test:
// closed-loop workers released together on a barrier, and a way to run part
// of a test under a named phase.
package load

import (
	"context"
	"fmt"
	"sync"
	"time"

	"systems-trinkets/harness/harness"
	"systems-trinkets/harness/hx"
)

// maxErrs caps how many fn errors Result.Errs retains; FnErrors still counts
// every one.
const maxErrs = 100

// Worker is one goroutine's view of a load run.
type Worker struct {
	ID   int             // 0..workers-1
	Ctx  context.Context // h.Ctx + hx.WithWorker(ID) + hx.WithCounter(shared counter); cancelled when the run ends
	Iter int             // 0-based index of this fn call for this worker (samples.seq counts requests, which may differ)
}

// Result aggregates one Closed/ClosedFor call.
type Result struct {
	Total, OK2xx, Non2xx, Errors int // HTTP tallies from the hx.Counter installed in every worker ctx (Errors = transport errors)
	FnErrors                     int // number of non-nil errors returned by fn
	Errs                         []error
	Elapsed                      time.Duration // from barrier release until the last worker returned
}

// Conclusive reports whether final-state invariants can be judged from this
// run: a request that errored may or may not have been applied, so any
// transport or worker error fails the test as inconclusive rather than
// letting an invariant pass or fail by luck.
func (r Result) Conclusive(h *harness.H) bool {
	h.T.Helper()
	if r.Errors == 0 && r.FnErrors == 0 {
		return true
	}
	first := ""
	if len(r.Errs) > 0 {
		first = ": " + r.Errs[0].Error()
	}
	h.Fail(fmt.Sprintf("load: %d transport errors, %d worker errors; final-state invariants are inconclusive%s", r.Errors, r.FnErrors, first))
	return false
}

// Closed runs `workers` goroutines, each calling fn iterationsPerWorker
// times, all released together on a barrier (every goroutine is started and
// parked before any runs fn). A worker stops early if its ctx is done.
func Closed(h *harness.H, workers, iterationsPerWorker int, fn func(*Worker) error) Result {
	return run(h, workers, iterationsPerWorker, 0, fn)
}

// ClosedFor is Closed but each worker loops until d has elapsed, counted
// from the barrier release.
func ClosedFor(h *harness.H, workers int, d time.Duration, fn func(*Worker) error) Result {
	return run(h, workers, 0, d, fn)
}

// Phase runs fn with h.Ctx temporarily replaced by hx.WithPhase(h.Ctx, name),
// restoring it afterwards (also on panic). Not goroutine-safe: call from the
// test goroutine only.
func Phase(h *harness.H, name string, fn func()) {
	prev := h.Ctx
	h.Ctx = hx.WithPhase(prev, name)
	defer func() { h.Ctx = prev }()
	fn()
}

// run is the barrier + fan-out shared by Closed (iters > 0) and ClosedFor
// (d > 0). The run context is derived only after every worker is parked at
// the barrier, so a ClosedFor deadline starts counting at release.
func run(h *harness.H, workers, iters int, d time.Duration, fn func(*Worker) error) Result {
	var ready sync.WaitGroup
	ready.Add(workers)
	release := make(chan struct{})
	counter := &hx.Counter{}

	var (
		mu       sync.Mutex
		errs     []error
		fnErrors int
		calls    int
	)
	record := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if err == nil {
			return
		}
		fnErrors++
		if len(errs) < maxErrs {
			errs = append(errs, err)
		}
	}

	// Written once, before close(release); workers read it after <-release,
	// so the channel close supplies the happens-before edge.
	var runCtx context.Context

	var wg sync.WaitGroup
	for id := range workers {
		wg.Go(func() {
			ready.Done()
			<-release
			w := &Worker{ID: id, Ctx: hx.WithCounter(hx.WithWorker(runCtx, id), counter)}
			for ; (iters == 0 || w.Iter < iters) && w.Ctx.Err() == nil; w.Iter++ {
				record(fn(w))
			}
		})
	}

	ready.Wait()
	ctx, cancel := context.WithCancel(h.Ctx)
	if d > 0 {
		ctx, cancel = context.WithTimeout(h.Ctx, d)
	}
	defer cancel()
	runCtx = ctx

	start := time.Now()
	close(release)
	wg.Wait()

	res := Result{
		Total:    int(counter.Total.Load()),
		OK2xx:    int(counter.OK2xx.Load()),
		Non2xx:   int(counter.Non2xx.Load()),
		Errors:   int(counter.Errors.Load()),
		FnErrors: fnErrors,
		Errs:     errs,
		Elapsed:  time.Since(start),
	}
	h.T.Logf("load: %d workers, %d calls: total=%d ok=%d non2xx=%d errors=%d fnErrors=%d in %v",
		workers, calls, res.Total, res.OK2xx, res.Non2xx, res.Errors, res.FnErrors, res.Elapsed)
	return res
}
