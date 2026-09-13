package counter

import (
	"sync"
	"sync/atomic"
	"testing"

	"systems-trinkets/harness/check"
	"systems-trinkets/harness/harness"
	"systems-trinkets/harness/load"
)

// TestCrashRestart is the kind-3 test for INV-COUNTER-06: while workers
// increment one name, the SUT is SIGKILLed and restarted; the final value
// must then be bounded by
//
//	all 2xx ≤ final ≤ all 2xx + transport errors
//
// Every acknowledged increment survives, and only requests that got no
// answer (in flight at the kill) are ambiguous. The kill is triggered by a
// count of acknowledged increments, not by time, so the test is the same
// shape on a fast and a slow engine. It needs a target the harness started
// itself (cmd in the target file) and a store that outlives the process.
func TestCrashRestart(t *testing.T) {
	h := harness.New(t)
	if !h.Restartable() {
		t.Skip("INV-COUNTER-06 needs a target the harness starts itself (cmd in the target file)")
	}
	if h.Target.Engine == "memory" {
		t.Skip("INV-COUNTER-06 does not apply to the memory engine: its state is the process")
	}

	const workers, perWorker = 16, 300
	const killAfter = workers * perWorker / 3 // acknowledged increments before the kill

	var (
		acked     atomic.Int64
		killOnce  sync.Once
		killNow   = make(chan struct{}) // closed by the worker that sees the killAfter-th ack
		restarted = make(chan struct{}) // closed by the test goroutine once the SUT is healthy again
	)
	args := name("crash")

	// The load runs in its own goroutine because Restart may t.Fatal, and
	// that must happen on the test goroutine. Workers keep hammering through
	// the kill — that is what puts requests in flight — and only pause when
	// a request fails after the kill was requested.
	done := make(chan load.Result, 1)
	go func() {
		done <- load.Closed(h, workers, perWorker, func(w *load.Worker) error {
			resp, err := h.Client.Post(w.Ctx, "/counters/{name}/incr", args, nil, nil)
			if err != nil {
				select {
				case <-killNow: // the SUT is down on purpose: wait for it to come back
					select {
					case <-restarted:
					case <-w.Ctx.Done():
					}
					return nil
				default:
					return err // an error before the kill was asked for is a real failure
				}
			}
			if resp.OK() && acked.Add(1) == killAfter {
				killOnce.Do(func() { close(killNow) })
			}
			return nil
		})
	}()

	select {
	case <-killNow:
	case r := <-done:
		t.Fatalf("load finished with %d acknowledged increments before reaching %d; nothing to kill", r.OK2xx, killAfter)
	}
	h.Restart() // SIGKILL the process group, start again, wait for /healthz
	ackedAtRestart := acked.Load()
	close(restarted)
	r := <-done

	check.Metric(h, "crash_acked_before_restart", float64(ackedAtRestart), "req", nil)
	check.Metric(h, "crash_errored", float64(r.Errors), "req", nil)
	if r.FnErrors > 0 {
		h.Fail("crash: transport errors before the kill was requested; the run is inconclusive")
		return
	}

	final := get(h, "crash")
	lo, hi := int64(r.OK2xx), int64(r.OK2xx+r.Errors)
	check.Invariant(h, "INV-COUNTER-06", lo <= final && final <= hi,
		"final value is bounded by acknowledged increments and acknowledged+errored",
		map[string]any{"final": final, "acked": r.OK2xx, "errored": r.Errors, "non2xx": r.Non2xx,
			"acked_at_restart": ackedAtRestart, "lost": lo - final, "sent": r.Total})
}
