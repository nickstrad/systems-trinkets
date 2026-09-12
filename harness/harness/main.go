package harness

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"systems-trinkets/harness/hx"
	"systems-trinkets/harness/results"
)

// Per-process state set up by Main and read by New. A suite has exactly one
// TestMain, so plain package variables are fine.
var (
	target   *Target // nil → no target configured; New skips
	sink     *results.Sink
	recorder results.Recorder = results.Discard
	runID    string
)

// Option configures Main.
type Option func(*options)

type options struct {
	inProcess func() http.Handler
}

// InProcess gives the suite a fallback SUT: when no target is configured in
// the environment, Main serves newHandler() on a loopback httptest server
// and runs the suite against it, so `go test ./...` exercises the suite
// instead of skipping it. The synthesised target is {pattern: the suite
// directory name, language: go, engine: memory, label: in-process}. Results
// are written only if HARNESS_RESULTS is set; otherwise they are discarded —
// in-process runs are the harness's own test, not history. Crash tests skip
// (there is no process to kill).
func InProcess(newHandler func() http.Handler) Option {
	return func(o *options) { o.inProcess = newHandler }
}

// Main is the body of a suite's TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(harness.Main(m)) }
//
// It reads the target from the environment (see TargetFromEnv), waits for the
// SUT's /healthz, opens the results sink under HARNESS_RESULTS (default
// <module>/results/runs/<run_id>), runs the tests, and closes the sink — also
// on SIGINT and on a panic — so a partial run still exports what it has.
// With no target configured it just runs the tests, which all skip, unless
// InProcess was given.
func Main(m *testing.M, opts ...Option) int {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	t, ok, err := TargetFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness:", err)
		return 2
	}
	runID = envOr(EnvRunID, results.NewRunID())
	resultDir := envOr(EnvResults, "")
	switch {
	case ok:
		if resultDir == "" {
			resultDir = filepath.Join(RunsDir(), runID)
		}
	case o.inProcess != nil:
		srv := httptest.NewServer(o.inProcess())
		defer srv.Close()
		pattern := "in-process"
		if wd, err := os.Getwd(); err == nil {
			pattern = filepath.Base(wd)
		}
		t = Target{Pattern: pattern, Language: "go", Engine: "memory", URL: srv.URL, Label: "in-process"}
		fmt.Fprintf(os.Stderr, "harness: no HARNESS_TARGET or HARNESS_URL set; running in-process at %s\n", srv.URL)
	default:
		fmt.Fprintln(os.Stderr, "harness: no HARNESS_TARGET or HARNESS_URL set; suite will skip")
		return m.Run()
	}
	target = &t

	ctx := context.Background()
	if err := waitHealthy(ctx, t.URL, envDuration("HARNESS_HEALTH_TIMEOUT", 10*time.Second)); err != nil {
		fmt.Fprintln(os.Stderr, "harness:", err)
		return 2
	}

	if resultDir == "" {
		return m.Run() // in-process without HARNESS_RESULTS: recorder stays Discard
	}
	host, _ := os.Hostname()
	run := results.RunRow{
		RunID: runID, Pattern: t.Pattern, Language: t.Language, Engine: t.Engine, Label: t.Label,
		TargetURL: t.URL, HarnessSHA: gitSHA(), SUTRef: os.Getenv(EnvSUTRef),
		GoVersion: runtime.Version(), Host: host,
	}
	s, err := results.Open(ctx, resultDir, run)
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness:", err)
		return 2
	}
	sink, recorder = s, s

	closeSink := func() { // Sink.Close is idempotent, so both paths below may call this
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.Close(cctx); err != nil {
			fmt.Fprintln(os.Stderr, "harness: results:", err)
		}
		fmt.Fprintf(os.Stderr, "harness: run %s → %s\n", runID, resultDir)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Fprintln(os.Stderr, "harness: interrupted, exporting partial results")
		closeSink()
		os.Exit(130)
	}()

	defer func() {
		if r := recover(); r != nil {
			closeSink()
			panic(r)
		}
	}()
	code := m.Run()
	signal.Stop(sig)
	closeSink()
	return code
}

// waitHealthy polls GET /healthz (unrecorded) until it answers 2xx.
func waitHealthy(ctx context.Context, baseURL string, timeout time.Duration) error {
	c := hx.New(baseURL, results.Discard, "", "")
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		resp, err := c.Get(ctx, "/healthz", nil, nil, hx.WithTimeout(2*time.Second))
		switch {
		case err != nil:
			last = err
		case resp.OK():
			return nil
		default:
			last = fmt.Errorf("status %d", resp.Status)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("SUT at %s not healthy after %v: %v", baseURL, timeout, last)
}

// RunsDir is where runs are written: <module>/results/runs/<run_id>/.
func RunsDir() string { return Path("results", "runs") }

// Path joins elem onto the module root.
func Path(elem ...string) string { return filepath.Join(append([]string{ModuleRoot()}, elem...)...) }

// ModuleRoot walks up from the working directory to the nearest go.mod.
// Falls back to the working directory.
func ModuleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func envDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
