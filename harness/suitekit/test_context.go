// Package suitekit connects invariant suites to targets, sampled HTTP requests,
// and recorded results. Main manages a suite; New binds an individual test.
package suitekit

import (
	"context"
	"strings"
	"sync"
	"systems-trinkets/harness/httpclient"
	"systems-trinkets/harness/results"
	"testing"
	"time"
)

// H is the per-test handle. Ctx is the base context for every request the
// test makes; load.Phase swaps it for a labelled one for the duration of a
// phase, so keep reading it through h.Ctx rather than copying it.
//
// Run and test identity live on Client (RunID, Test, Rec).
type H struct {
	T      *testing.T
	Client *httpclient.Client
	Target Target
	Ctx    context.Context

	mu       sync.Mutex
	failures []string // filled by Fail; becomes TestRow.Error
}

// New binds t to the configured target: it resets the SUT (POST /_reset),
// confirms /healthz, and registers a cleanup that records the TestRow and
// flushes the sink. Skips the test when no target is configured.
func New(t *testing.T) *H {
	t.Helper()
	if target == nil {
		t.Skip("harness: no target configured (set HARNESS_TARGET or HARNESS_URL)")
	}
	h := &H{
		T:      t,
		Client: httpclient.New(target.URL, recorder, runID, t.Name()),
		Target: *target,
		Ctx:    context.Background(),
	}
	if d, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		h.Ctx, cancel = context.WithDeadline(h.Ctx, d)
		t.Cleanup(cancel)
	}

	start := time.Now()
	t.Cleanup(func() {
		row := results.TestRow{RunID: runID, Test: t.Name(), Status: "pass", DurationNS: int64(time.Since(start))}
		switch {
		case t.Skipped():
			row.Status = "skip"
		case t.Failed():
			row.Status = "fail"
			h.mu.Lock()
			row.Error = strings.Join(h.failures, "; ")
			h.mu.Unlock()
		}
		recorder.Test(row)
		if sink != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = sink.Flush(ctx)
		}
	})

	h.Reset()
	return h
}

// Reset wipes the SUT's state via POST /_reset and confirms /healthz. Both
// requests are sampled under results.PhaseSetup so reports can exclude them.
func (h *H) Reset() {
	h.T.Helper()
	ctx := httpclient.WithPhase(h.Ctx, results.PhaseSetup)
	h.Must(h.Client.Post(ctx, "/_reset", nil, nil, nil))
	h.Must(h.Client.Get(ctx, "/healthz", nil, nil))
}

// Fail marks the test failed with a message that is also stored in the
// TestRow (testing.T does not expose its own failure text). check.Invariant
// calls this; tests may too.
func (h *H) Fail(msg string) {
	h.T.Helper()
	h.mu.Lock()
	h.failures = append(h.failures, msg)
	h.mu.Unlock()
	h.T.Error(msg)
}

// Get, Post and Delete are httpclient wrappers that use h.Ctx.
func (h *H) Get(pathTemplate string, pathArgs map[string]string, out any, opts ...httpclient.Opt) (*httpclient.Resp, error) {
	return h.Client.Get(h.Ctx, pathTemplate, pathArgs, out, opts...)
}

func (h *H) Post(pathTemplate string, pathArgs map[string]string, body, out any, opts ...httpclient.Opt) (*httpclient.Resp, error) {
	return h.Client.Post(h.Ctx, pathTemplate, pathArgs, body, out, opts...)
}

func (h *H) Delete(pathTemplate string, pathArgs map[string]string, opts ...httpclient.Opt) (*httpclient.Resp, error) {
	return h.Client.Delete(h.Ctx, pathTemplate, pathArgs, opts...)
}

// Must fails the test immediately on a transport error or a non-2xx status;
// use it for setup steps whose failure makes the rest of the test meaningless.
func (h *H) Must(resp *httpclient.Resp, err error) *httpclient.Resp {
	h.T.Helper()
	if err != nil {
		h.T.Fatalf("harness: %v", err)
	}
	if !resp.OK() {
		h.T.Fatalf("harness: status %d: %s", resp.Status, resp.Body)
	}
	return resp
}
