package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"systems-trinkets/harness/results"
)

// maxBodyBytes caps how much of a response body Do will read and keep.
const maxBodyBytes = 1 << 20 // 1 MiB

// transport is shared by every Client. http.DefaultTransport keeps only two
// idle connections per host, so 32 workers would redial every few requests
// and the dials would show up in the recorded latencies.
var transport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 1024
	t.MaxIdleConnsPerHost = 512
	return t
}()

// Client is bound to one target URL and one (run, test) identity; every
// request it makes is recorded as a results.SampleRow through Rec.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Rec     results.Recorder
	RunID   string
	Test    string

	mu   sync.Mutex
	seqs map[int]int // next SampleRow.Seq per worker
}

// Resp is the observed response. Non-2xx is not an error.
type Resp struct {
	Status  int
	Latency time.Duration
	Body    []byte
}

// OK reports whether the status is 2xx — the harness's one definition of a
// successful request.
func (r *Resp) OK() bool { return Is2xx(r.Status) }

// Is2xx is OK for a bare status code.
func Is2xx(status int) bool { return status >= 200 && status < 300 }

// Opt tunes a single request.
type Opt func(*reqOpts)

type reqOpts struct {
	timeout time.Duration
}

// WithTimeout gives this request its own deadline, applied via context.
func WithTimeout(d time.Duration) Opt {
	return func(o *reqOpts) { o.timeout = d }
}

// New returns a Client bound to baseURL, recording every request into rec
// under (runID, test). The underlying http.Client has a 10s timeout.
func New(baseURL string, rec results.Recorder, runID, test string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 10 * time.Second, Transport: transport},
		Rec:     rec,
		RunID:   runID,
		Test:    test,
		seqs:    map[int]int{},
	}
}

func (c *Client) Get(ctx context.Context, pathTemplate string, pathArgs map[string]string, out any, opts ...Opt) (*Resp, error) {
	return c.Do(ctx, http.MethodGet, pathTemplate, pathArgs, nil, out, opts...)
}

func (c *Client) Post(ctx context.Context, pathTemplate string, pathArgs map[string]string, body, out any, opts ...Opt) (*Resp, error) {
	return c.Do(ctx, http.MethodPost, pathTemplate, pathArgs, body, out, opts...)
}

func (c *Client) Delete(ctx context.Context, pathTemplate string, pathArgs map[string]string, opts ...Opt) (*Resp, error) {
	return c.Do(ctx, http.MethodDelete, pathTemplate, pathArgs, nil, nil, opts...)
}

// Do expands pathTemplate against pathArgs (each {key} is replaced with
// url.PathEscape(pathArgs[key]); a missing key is an error), sends body (nil
// for none, []byte as-is, anything else JSON-marshalled), and, on a 2xx
// response with a non-empty body, JSON-unmarshals into out (if non-nil).
//
// Non-2xx is not an error: tests inspect Resp.Status. Transport errors,
// timeouts and context cancellation are errors and return a nil *Resp.
//
// Exactly one results.SampleRow is recorded per call, including on error.
func (c *Client) Do(ctx context.Context, method, pathTemplate string, pathArgs map[string]string, body any, out any, opts ...Opt) (*Resp, error) {
	var o reqOpts
	for _, opt := range opts {
		opt(&o)
	}

	worker := WorkerFrom(ctx)
	sample := results.SampleRow{
		RunID:        c.RunID,
		Test:         c.Test,
		Phase:        PhaseFrom(ctx),
		Worker:       worker,
		Seq:          c.nextSeq(worker),
		Method:       method,
		PathTemplate: pathTemplate,
		StartedAt:    time.Now().UTC(),
	}

	resp, err := c.do(ctx, method, pathTemplate, pathArgs, body, out, o)

	if resp != nil {
		sample.LatencyNS = resp.Latency.Nanoseconds()
		sample.Status = resp.Status
	}
	if err != nil {
		sample.Err = err.Error()
	}
	c.Rec.Sample(sample)
	c.bumpCounter(ctx, sample.Status, err)

	return resp, err
}

func (c *Client) do(ctx context.Context, method, pathTemplate string, pathArgs map[string]string, body any, out any, o reqOpts) (*Resp, error) {
	path, err := expandPath(pathTemplate, pathArgs)
	if err != nil {
		return nil, err
	}
	fullURL := c.BaseURL + path

	var bodyReader io.Reader
	var contentType string
	switch b := body.(type) {
	case nil:
		// no body
	case []byte:
		bodyReader = bytes.NewReader(b)
	default:
		encoded, err := json.Marshal(b)
		if err != nil {
			return nil, fmt.Errorf("httpclient: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
		contentType = "application/json"
	}

	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("httpclient: build request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	httpResp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpclient: %s %s: %w", method, path, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, maxBodyBytes))
	latency := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("httpclient: read body: %w", err)
	}

	resp := &Resp{Status: httpResp.StatusCode, Latency: latency, Body: respBody}

	if out != nil && resp.OK() && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp, fmt.Errorf("httpclient: decode response: %w", err)
		}
	}
	return resp, nil
}

// expandPath replaces each {key} in template with url.PathEscape(pathArgs[key]).
// A key present in the template but missing from pathArgs is an error.
func expandPath(template string, pathArgs map[string]string) (string, error) {
	if !strings.Contains(template, "{") {
		return template, nil
	}
	var b strings.Builder
	i := 0
	for i < len(template) {
		open := strings.IndexByte(template[i:], '{')
		if open < 0 {
			b.WriteString(template[i:])
			break
		}
		open += i
		b.WriteString(template[i:open])
		close := strings.IndexByte(template[open:], '}')
		if close < 0 {
			return "", fmt.Errorf("httpclient: unterminated {} in path template %q", template)
		}
		close += open
		key := template[open+1 : close]
		val, ok := pathArgs[key]
		if !ok {
			return "", fmt.Errorf("httpclient: missing path arg %q for template %q", key, template)
		}
		b.WriteString(url.PathEscape(val))
		i = close + 1
	}
	return b.String(), nil
}

// nextSeq returns the next per-worker sequence number, starting at 0.
func (c *Client) nextSeq(worker int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	seq := c.seqs[worker]
	c.seqs[worker] = seq + 1
	return seq
}

func (c *Client) bumpCounter(ctx context.Context, status int, err error) {
	counter := CounterFrom(ctx)
	if counter == nil {
		return
	}
	counter.Total.Add(1)
	switch {
	case err != nil:
		counter.Errors.Add(1)
	case Is2xx(status):
		counter.OK2xx.Add(1)
	default:
		counter.Non2xx.Add(1)
	}
}
