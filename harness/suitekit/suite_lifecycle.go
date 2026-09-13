package suitekit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"systems-trinkets/harness/process"
	"systems-trinkets/harness/results"
	"testing"
	"time"
)

// healthTimeoutEnv is read by both Main and H.Restart, so it is named once.
const healthTimeoutEnv = "HARNESS_HEALTH_TIMEOUT"

// Per-process state set up by Main and read by New. A suite has exactly one
// TestMain, so plain package variables are fine.
var (
	target   *Target // nil → no target configured; New skips
	sink     *results.Sink
	recorder results.Recorder = results.Discard
	runID    string

	sutMu    sync.Mutex    // guards proc, sutSpec, sink publication, and stopping
	proc     *process.Proc // non-nil once Main has started the SUT from Target.Cmd
	sutSpec  process.Spec
	stopping bool
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
//	func TestMain(m *testing.M) { os.Exit(suitekit.Main(m)) }
//
// It reads the target from the environment (see TargetFromEnv); if the
// target has a Cmd, refuses to run if something already answers at its URL
// (almost always a stale SUT the harness itself failed to stop last time —
// see below), otherwise starts it (package process) and logs to
// <results dir>/sut.log; waits for the SUT's /healthz (failing if the
// harness's own SUT process exits, or turns out to have already exited,
// before or during that wait — a stale answerer again); opens the results
// sink under HARNESS_RESULTS (default <module>/artifacts/runs/<run_id>); runs
// the tests; stops the SUT; and closes the sink. The SUT is also stopped,
// and the sink closed, on SIGINT/SIGTERM — signal handling is installed
// before the SUT is even started, so Ctrl-C during a slow "go run" build or
// the health wait still reaches the child.
//
// What this does NOT cover: a panicking test, or a `go test -timeout`
// expiry. Both kill the process from the test's own goroutine, so none of
// Main's deferred or trailing code runs: the SUT is left running and
// nothing is recorded. Do not read a recover here as covering that — there
// is none, because it could not see such a panic anyway. The stale-answer
// check above is what surfaces a SUT left behind that way, on the *next*
// run, rather than letting it silently test against the leftover.
//
// With no target configured Main just runs the tests, which all skip,
// unless InProcess was given.
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

	stopSUT := func() {
		sutMu.Lock()
		defer sutMu.Unlock()
		p := proc
		stopping = true
		proc = nil
		if p != nil {
			if err := p.Stop(5 * time.Second); err != nil {
				fmt.Fprintln(os.Stderr, "harness: stop SUT:", err)
			}
		}
	}
	closeSink := func() { // guards nil/already-closed sink so every exit path may call this
		sutMu.Lock()
		s := sink
		sutMu.Unlock()
		if s == nil {
			return
		}
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.Close(cctx); err != nil {
			fmt.Fprintln(os.Stderr, "harness: results:", err)
		}
		fmt.Fprintf(os.Stderr, "harness: run %s → %s\n", runID, resultDir)
	}

	// Installed before the SUT is even started (not after results.Open, as
	// before): a "go run" build or a slow health wait can run long, and
	// Ctrl-C during either must still reach the child process group instead
	// of leaking it.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		<-sig
		fmt.Fprintln(os.Stderr, "harness: interrupted, stopping the SUT and exporting partial results")
		stopSUT()
		closeSink()
		os.Exit(130)
	}()

	// Start the SUT before waiting for health, if the target says to. Only
	// file-loaded targets (EnvTarget) ever carry a Cmd; ad-hoc --url targets
	// and InProcess never do, so proc stays nil and H.Restartable is false.
	var sutLog *os.File
	var startedProc *process.Proc
	if ok && len(t.Cmd) > 0 {
		if answers(t.URL, time.Second) {
			fmt.Fprintf(os.Stderr, "harness: something already answers at %s; stop it (lsof -nP -iTCP:%s) or use --url\n",
				t.URL, urlPort(t.URL))
			return 2
		}
		if err := os.MkdirAll(resultDir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "harness:", err)
			return 2
		}
		logPath := filepath.Join(resultDir, "sut.log")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "harness:", err)
			return 2
		}
		sutLog = f
		defer f.Close()

		env := os.Environ()
		for k, v := range t.Env {
			env = append(env, k+"="+v) // appended after os.Environ(): later wins
		}
		spec := process.Spec{Cmd: t.Cmd, Dir: t.Cwd, Env: env, Stdout: sutLog, Stderr: sutLog}
		// Publish the child under the same lock as signal-driven shutdown.
		sutMu.Lock()
		if stopping {
			sutMu.Unlock()
			return 130
		}
		p, err := process.Start(spec)
		if err != nil {
			sutMu.Unlock()
			fmt.Fprintln(os.Stderr, "harness:", err)
			return 2
		}
		proc, sutSpec = p, spec
		sutMu.Unlock()
		startedProc = p
		fmt.Fprintf(os.Stderr, "harness: started SUT pid %d (%s)\n", p.PID(), strings.Join(t.Cmd, " "))
	}

	var exited <-chan struct{}
	if startedProc != nil {
		exited = startedProc.Exited()
	}
	ctx := context.Background()
	if err := waitHealthy(ctx, t.URL, envDuration(healthTimeoutEnv, 10*time.Second), exited); err != nil {
		fmt.Fprintln(os.Stderr, "harness:", err)
		if exited != nil {
			select {
			case <-exited:
				fmt.Fprintln(os.Stderr, "harness: SUT log tail:")
				logTail(os.Stderr, filepath.Join(resultDir, "sut.log"), 4096)
			default:
			}
		}
		stopSUT()
		return 2
	}
	// waitHealthy succeeding is not proof our own SUT answered: if it died
	// (e.g. lost a bind race) right around when a stale process took over
	// the port, health still looks fine. Confirm our own process is still
	// the one running.
	if startedProc != nil {
		select {
		case <-startedProc.Exited():
			fmt.Fprintf(os.Stderr, "harness: %s answers /healthz, but the harness's own SUT process already exited — something else is listening there\n", t.URL)
			logTail(os.Stderr, filepath.Join(resultDir, "sut.log"), 4096)
			stopSUT()
			return 2
		default:
		}
	}

	if resultDir == "" {
		stopSUT() // in-process without HARNESS_RESULTS: recorder stays Discard; never has a Cmd, but stop is a safe no-op
		return m.Run()
	}
	host, _ := os.Hostname()
	run := results.RunRow{
		RunID: runID, Pattern: t.Pattern, Language: t.Language, Engine: t.Engine, Label: t.Label,
		TargetURL: t.URL, HarnessSHA: gitSHA(), SUTRef: os.Getenv(EnvSUTRef),
		GoVersion: runtime.Version(), Host: host,
	}
	sutMu.Lock()
	if stopping {
		sutMu.Unlock()
		return 130
	}
	s, err := results.Open(ctx, resultDir, run)
	if err != nil {
		sutMu.Unlock()
		fmt.Fprintln(os.Stderr, "harness:", err)
		stopSUT()
		return 2
	}
	sink, recorder = s, s
	sutMu.Unlock()

	code := m.Run()
	stopSUT() // before closeSink: the SUT is done producing results by the time the run is finalised
	closeSink()
	return code
}

// logTail writes the last n bytes of path to w, used to explain a SUT that
// exited before becoming healthy.
func logTail(w io.Writer, path string, n int64) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(w, "(no sut.log: %v)\n", err)
		return
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > n {
		f.Seek(-n, io.SeekEnd)
	}
	buf := make([]byte, n)
	nr, _ := f.Read(buf)
	w.Write(buf[:nr])
	if nr > 0 && buf[nr-1] != '\n' {
		fmt.Fprintln(w)
	}
}

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
