// Package check records invariant evaluations and metrics. Every Invariant
// call writes a results.CheckRow whether it passed or not, and fails the test
// when it did not; the id must come from the suite's INVARIANTS.md so a
// failure points at the primitive that was supposed to guarantee it.
package check

import (
	"fmt"
	"time"

	"systems-trinkets/harness/results"
	"systems-trinkets/harness/suitekit"
)

// Invariant records one evaluation of invariant id and fails the test if !ok.
// details is free-form context for the report (got/want, counts, ids).
func Invariant(h *suitekit.H, id string, ok bool, msg string, details map[string]any) {
	h.T.Helper()
	c := h.Client
	c.Rec.Check(results.CheckRow{
		RunID: c.RunID, Test: c.Test, InvariantID: id, OK: ok, Message: msg,
		DetailsJSON: results.JSON(details), At: time.Now().UTC(),
	})
	if ok {
		h.T.Logf("ok   %s: %s", id, msg)
		return
	}
	h.Fail(fmt.Sprintf("FAIL %s: %s %s", id, msg, results.JSON(details)))
}

// Eventually polls cond every 50ms until it returns true or timeout passes,
// then records the result as one check. Use it for "should hold once the
// system settles" invariants (visibility after reset, expiry), never for
// timing assertions.
func Eventually(h *suitekit.H, id string, timeout time.Duration, cond func() (bool, map[string]any), msg string) {
	h.T.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ok, details := cond()
		if ok || time.Now().After(deadline) {
			if details == nil {
				details = map[string]any{}
			}
			details["timeout"] = timeout.String()
			Invariant(h, id, ok, msg, details)
			return
		}
		select {
		case <-h.Ctx.Done():
			Invariant(h, id, false, msg+" (context done)", details)
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Metric records a number for reports; it never fails the test.
func Metric(h *suitekit.H, name string, value float64, unit string, labels map[string]any) {
	c := h.Client
	c.Rec.Metric(results.MetricRow{
		RunID: c.RunID, Test: c.Test, Name: name, Value: value, Unit: unit,
		LabelsJSON: results.JSON(labels), At: time.Now().UTC(),
	})
	h.T.Logf("metric %s = %g %s", name, value, unit)
}
