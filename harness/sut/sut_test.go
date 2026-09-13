package sut

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// counterBin is built once, in TestMain, into a scratch directory removed at
// exit.
var counterBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sut-test-*")
	if err != nil {
		fmt.Println("sut: mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	counterBin = filepath.Join(dir, "counter")
	build := exec.Command("go", "build", "-o", counterBin, "./example-sut/cmd/counter")
	build.Dir = moduleRoot()
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Printf("sut: build example-sut/cmd/counter: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// moduleRoot walks up from the working directory to the nearest go.mod. Kept
// local (rather than importing package harness, which imports sut) to avoid
// an import cycle from an internal test.
func moduleRoot() string {
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

// freePort returns a loopback TCP port that is free at the moment of the
// call. There is an inherent, accepted TOCTOU race between the Listen/Close
// here and the SUT binding it moments later.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// waitHealthy polls GET /healthz until it answers 200, or fails the test.
func waitHealthy(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s never became healthy: %v", addr, last)
}

// assertRefused checks that a GET fails at the transport level (connection
// refused), i.e. nothing is listening.
func assertRefused(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := http.Get("http://" + addr + "/healthz")
		if err != nil {
			return // any transport error is fine; the point is "nobody's listening"
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still answers after the SUT should have been killed", addr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartKillRestartStop(t *testing.T) {
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var log syncBuffer

	spec := Spec{
		Cmd:    []string{counterBin, "--addr", addr, "--engine", "memory"},
		Env:    os.Environ(),
		Stdout: &log,
		Stderr: &log,
	}

	// 1. Start → healthy; PID > 0; Exited() not closed.
	p1, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	if p1.PID() <= 0 {
		t.Fatalf("PID = %d, want > 0", p1.PID())
	}
	select {
	case <-p1.Exited():
		t.Fatal("Exited() closed right after Start")
	default:
	}
	waitHealthy(t, addr)

	// 2. Kill → Exited() closes promptly; /healthz refused.
	if err := p1.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	select {
	case <-p1.Exited():
	default:
		t.Fatal("Exited() not closed after Kill returned")
	}
	assertRefused(t, addr)

	// 3. Start again on the same port → healthy, a different pid.
	p2, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	if p2.PID() == p1.PID() {
		t.Fatalf("second Start reused pid %d", p2.PID())
	}
	waitHealthy(t, addr)

	// 4. Stop with grace → nil, Exited() closed, exit via SIGTERM (not
	// killed): the counter binary handles SIGTERM with http.Server.Shutdown.
	if err := p2.Stop(2 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-p2.Exited():
	default:
		t.Fatal("Exited() not closed after Stop returned")
	}
	assertNotKilled(t, p2)
	assertRefused(t, addr)

	if !containsSoon(&log, "listening on") {
		t.Errorf("SUT log missing \"listening on\": %q", log.String())
	}
}

// assertNotKilled fails the test if p's exit state shows it was terminated
// by a signal (i.e. Stop fell through to Kill instead of a clean exit).
func assertNotKilled(t *testing.T, p *Proc) {
	t.Helper()
	state := p.ExitState()
	if state == nil {
		t.Fatal("ExitState() is nil after Exited() closed")
	}
	ws, ok := state.Sys().(syscall.WaitStatus)
	if !ok {
		return // platform doesn't expose WaitStatus; nothing to assert
	}
	if ws.Signaled() {
		t.Errorf("process was killed by signal %v, want a clean exit", ws.Signal())
	}
}

// TestKillReachesGrandchild starts the counter via a "go run" wrapper — the
// real server is a grandchild — and checks that Kill's process-group signal
// reaches it too, not just the "go run" parent.
func TestKillReachesGrandchild(t *testing.T) {
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var log syncBuffer

	spec := Spec{
		Cmd:    []string{"go", "run", "./example-sut/cmd/counter", "--addr", addr, "--engine", "memory"},
		Dir:    moduleRoot(),
		Env:    os.Environ(),
		Stdout: &log,
		Stderr: &log,
	}
	p, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	waitHealthy(t, addr) // only healthy once the grandchild is up

	if err := p.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	assertRefused(t, addr)
}

// TestStopFallsThroughToKill uses a process that ignores SIGTERM; Stop must
// give up after the grace period and SIGKILL it instead of hanging.
func TestStopFallsThroughToKill(t *testing.T) {
	spec := Spec{
		Cmd: []string{"sh", "-c", `trap "" TERM; sleep 60`},
		Env: os.Environ(),
	}
	p, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := p.Stop(300 * time.Millisecond); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Stop took %v, want it to fall through to Kill promptly after the grace period", elapsed)
	}
	select {
	case <-p.Exited():
	default:
		t.Fatal("Exited() not closed after Stop fell through to Kill")
	}
	if state := p.ExitState(); state != nil {
		if ws, ok := state.Sys().(syscall.WaitStatus); ok && !ws.Signaled() {
			t.Error("want the process to have been killed by a signal")
		}
	}
}

func TestKillIsIdempotent(t *testing.T) {
	spec := Spec{Cmd: []string{"sh", "-c", "sleep 60"}, Env: os.Environ()}
	p, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Kill(); err != nil {
		t.Fatalf("first Kill: %v", err)
	}
	if err := p.Kill(); err != nil {
		t.Fatalf("second Kill (on an already-dead process): %v", err)
	}
	if err := p.Stop(time.Second); err != nil {
		t.Fatalf("Stop (on an already-dead process): %v", err)
	}
}

// TestConcurrentKillAndStop is the race-detector's view of the arrangement
// harness.Main relies on: its signal handler may Stop the process while a
// test goroutine is inside H.Restart's Kill. Both must return, and the
// process must be gone.
func TestConcurrentKillAndStop(t *testing.T) {
	spec := Spec{Cmd: []string{"sh", "-c", "sleep 60"}, Env: os.Environ()}
	p, err := Start(spec)
	if err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 4)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); errs <- p.Kill() }()
		go func() { defer wg.Done(); errs <- p.Stop(2 * time.Second) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Kill/Stop: %v", err)
		}
	}
	select {
	case <-p.Exited():
	default:
		t.Fatal("process still running after concurrent Kill/Stop")
	}
}

func TestStartRejectsEmptyCmd(t *testing.T) {
	if _, err := Start(Spec{}); err == nil {
		t.Error("want an error for an empty Cmd")
	}
}

// syncBuffer is a concurrency-safe bytes buffer: the SUT writes to it from
// its own OS pipe-reading goroutine while the test reads it concurrently.
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// containsSoon polls b for substr for a bit, since the SUT logs
// asynchronously with respect to /healthz answering.
func containsSoon(b *syncBuffer, substr string) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(b.String(), substr) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return strings.Contains(b.String(), substr)
}
