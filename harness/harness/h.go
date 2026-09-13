// Package harness is the glue a suite uses: Main opens the results sink for
// the run, New binds a test to the target and records its outcome.
package harness

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"systems-trinkets/harness/hx"
	"systems-trinkets/harness/results"
	"systems-trinkets/harness/sut"
)

// Target is one concrete server under test, loaded from targets/<name>.toml
// or synthesised from HARNESS_URL. When Cmd is set, Main starts it itself
// (via package sut) before waiting for /healthz, and H.Restartable is true.
type Target struct {
	Pattern  string             `toml:"pattern"`
	Language string             `toml:"language"`
	Engine   string             `toml:"engine"`
	URL      string             `toml:"url"`
	Label    string             `toml:"label"`
	Cmd      []string           `toml:"cmd"` // optional: harness starts/stops/restarts the SUT itself
	Cwd      string             `toml:"cwd"` // relative to the target file; resolved by LoadTarget
	Env      map[string]string  `toml:"env"` // merged over os.Environ(); later (this) wins
	Expect   map[string]float64 `toml:"expect"`
}

// H is the per-test handle. Ctx is the base context for every request the
// test makes; load.Phase swaps it for a labelled one for the duration of a
// phase, so keep reading it through h.Ctx rather than copying it.
//
// Run and test identity live on Client (RunID, Test, Rec).
type H struct {
	T      *testing.T
	Client *hx.Client
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
		Client: hx.New(target.URL, recorder, runID, t.Name()),
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
	ctx := hx.WithPhase(h.Ctx, results.PhaseSetup)
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

// Get, Post and Delete are hx wrappers that use h.Ctx.
func (h *H) Get(pathTemplate string, pathArgs map[string]string, out any, opts ...hx.Opt) (*hx.Resp, error) {
	return h.Client.Get(h.Ctx, pathTemplate, pathArgs, out, opts...)
}

func (h *H) Post(pathTemplate string, pathArgs map[string]string, body, out any, opts ...hx.Opt) (*hx.Resp, error) {
	return h.Client.Post(h.Ctx, pathTemplate, pathArgs, body, out, opts...)
}

func (h *H) Delete(pathTemplate string, pathArgs map[string]string, opts ...hx.Opt) (*hx.Resp, error) {
	return h.Client.Delete(h.Ctx, pathTemplate, pathArgs, opts...)
}

// Must fails the test immediately on a transport error or a non-2xx status;
// use it for setup steps whose failure makes the rest of the test meaningless.
func (h *H) Must(resp *hx.Resp, err error) *hx.Resp {
	h.T.Helper()
	if err != nil {
		h.T.Fatalf("harness: %v", err)
	}
	if !resp.OK() {
		h.T.Fatalf("harness: status %d: %s", resp.Status, resp.Body)
	}
	return resp
}

// Restartable reports whether Main started the SUT itself (Target.Cmd was
// set): only then can Restart kill and relaunch it. False for --url
// targets, in-process runs, and cmd-less targets.
func (h *H) Restartable() bool {
	sutMu.Lock()
	defer sutMu.Unlock()
	return proc != nil
}

// Restart kills the SUT's whole process group and starts it again from the
// same Spec, waiting for /healthz; it t.Fatals if the kill doesn't actually
// clear the listener, or the SUT never comes back healthy. It does NOT
// reset state — surviving (or not) a kill is the property under test.
// Requests in flight when the kill happens fail with transport errors;
// crash tests state their invariant as bounds, not via Result.Conclusive.
//
// Restart must be called from the test goroutine (it may call t.Fatal,
// which must run there) and is not safe to call concurrently with itself.
func (h *H) Restart() {
	h.T.Helper()

	// Kill, the post-kill stale-answer check, Start and the proc
	// reassignment all happen under sutMu, held continuously: if Main's
	// SIGINT handler (which also takes sutMu, in stopSUT) ran between Kill
	// and the reassignment, it would stop the now-current proc pointer
	// (the killed one), os.Exit, and leave the freshly started replacement
	// orphaned with nothing left to stop it.
	sutMu.Lock()
	p, spec := proc, sutSpec
	if p == nil {
		sutMu.Unlock()
		h.T.Fatal("harness: Restart called but Main did not start the SUT (target has no cmd)")
	}
	err := p.Kill()
	if err == nil && answers(h.Target.URL, time.Second) {
		err = fmt.Errorf("still answers at %s after Kill", h.Target.URL)
	}
	var next *sut.Proc
	if err == nil {
		next, err = sut.Start(spec)
	}
	if err == nil {
		proc = next
	}
	sutMu.Unlock()
	if err != nil {
		h.T.Fatalf("harness: restart: %v", err)
	}

	if err := waitHealthy(context.Background(), h.Target.URL, envDuration(healthTimeoutEnv, 10*time.Second), next.Exited()); err != nil {
		h.T.Fatalf("harness: restart: %v", err)
	}
}
