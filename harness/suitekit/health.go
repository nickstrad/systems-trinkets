package suitekit

import (
	"context"
	"fmt"
	"net/url"
	"systems-trinkets/harness/httpclient"
	"systems-trinkets/harness/results"
	"time"
)

// urlPort is the port of baseURL, for the lsof hint in the stale-answer
// message; "" for a URL with no explicit port (every target that carries a
// cmd names one).
func urlPort(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Port()
}

// answers reports whether something is already listening and answering
// GET /healthz at baseURL, regardless of status — the question is only
// "does anyone appear to own this already", asked once before Main starts
// its own SUT (a stale SUT from a run that never got to stop it) and again
// by Restart right after Kill (the kill did not reach the listener).
func answers(baseURL string, timeout time.Duration) bool {
	c := httpclient.New(baseURL, results.Discard, "", "")
	_, err := c.Get(context.Background(), "/healthz", nil, nil, httpclient.WithTimeout(timeout))
	return err == nil
}

// waitHealthy polls GET /healthz (unrecorded) until it answers 2xx. exited,
// if non-nil, is the SUT's process.Proc.Exited(): waitHealthy returns promptly
// (rather than waiting out the full timeout) if it closes first, so a SUT
// that crashes on startup fails fast.
func waitHealthy(ctx context.Context, baseURL string, timeout time.Duration, exited <-chan struct{}) error {
	c := httpclient.New(baseURL, results.Discard, "", "")
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return fmt.Errorf("SUT process exited before becoming healthy (last error: %v)", last)
		default:
		}
		resp, err := c.Get(ctx, "/healthz", nil, nil, httpclient.WithTimeout(2*time.Second))
		switch {
		case err != nil:
			last = err
		case resp.OK():
			return nil
		default:
			last = fmt.Errorf("status %d", resp.Status)
		}
		select {
		case <-exited:
			return fmt.Errorf("SUT process exited before becoming healthy (last error: %v)", last)
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("SUT at %s not healthy after %v: %v", baseURL, timeout, last)
}
