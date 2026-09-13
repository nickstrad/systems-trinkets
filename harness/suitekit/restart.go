package suitekit

import (
	"context"
	"fmt"
	"systems-trinkets/harness/process"
	"time"
)

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
	var next *process.Proc
	if err == nil {
		next, err = process.Start(spec)
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
