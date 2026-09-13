package suitekit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"systems-trinkets/harness/results"
)

// freePort returns a loopback TCP port that is free at the moment of the
// call; there is an inherent, accepted race until the SUT binds it.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// portFree reports whether nothing answers a TCP dial to addr.
func portFree(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return true
	}
	conn.Close()
	return false
}

func buildCounterBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "counter")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/counter")
	cmd.Dir = filepath.Join(ModuleRoot(), "..", "examples", "counter")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build standalone counter: %v\n%s", err, out)
	}
	return bin
}

// writeTarget writes a target file with a cmd, one directory below dir, so
// `cwd = ".."` exercises LoadTarget resolving cwd against the target file's
// own directory rather than the process's working directory.
func writeTarget(t *testing.T, dir, addr string, cmd []string) string {
	t.Helper()
	targetDir := filepath.Join(dir, "targets")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	quoted := make([]string, len(cmd))
	for i, a := range cmd {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	path := filepath.Join(targetDir, "counter-integration.toml")
	body := fmt.Sprintf(`
pattern  = "counter"
language = "go"
engine   = "sqlite"
url      = "http://%s"
label    = "integration"
cmd      = [%s]
cwd      = ".."
`, addr, strings.Join(quoted, ", "))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// childSuite runs the counter suite in a child `go test` with the HARNESS_*
// environment stripped and replaced by env. It returns the combined output
// and go test's error (non-nil when the suite, or Main itself, failed), so
// callers can assert on both a passing and a refusing run.
func childSuite(t *testing.T, runRegexp string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", "test", "./suites/counter/", "-run", runRegexp, "-count=1", "-v")
	cmd.Dir = ModuleRoot()
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HARNESS_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestMainStartsAndStopsSUT runs the counter suite's contract and crash
// tests in a child `go test` against a target file with a Cmd, so Main
// itself starts, restarts and stops the SUT. The engine is sqlite (not
// memory) precisely so TestCrashRestart runs rather than skipping: that is
// the only test that exercises H.Restart end to end. It checks Main's
// "started SUT pid" line, that both tests PASSed, that sut.log carries a
// "listening on" line per SUT process (the original and the restarted one),
// and that the port is free again once the child exits.
func TestMainStartsAndStopsSUT(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go test and a real SUT process")
	}
	bin := buildCounterBinary(t)
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))

	root := t.TempDir()
	dsn := filepath.Join(root, "crash.db")
	targetPath := writeTarget(t, root, addr,
		[]string{bin, "--addr", addr, "--engine", "sqlite", "--dsn", dsn})

	resultsDir := filepath.Join(root, "results")
	out, err := childSuite(t, "TestContract$|TestCrashRestart$",
		EnvTarget+"="+targetPath, EnvResults+"="+resultsDir, EnvRunID+"=run-sut-integration")
	if err != nil {
		t.Fatalf("go test ./suites/counter: %v\n%s", err, out)
	}
	if !strings.Contains(out, "started SUT pid") {
		t.Errorf("output missing \"started SUT pid\":\n%s", out)
	}
	for _, name := range []string{"TestContract", "TestCrashRestart"} {
		// PASS, not just "not FAIL": a skipped crash test would mean the
		// restart path was never exercised at all.
		if !strings.Contains(out, "--- PASS: "+name) {
			t.Errorf("%s did not PASS (skipped?):\n%s", name, out)
		}
	}

	logBytes, err := os.ReadFile(filepath.Join(resultsDir, "sut.log"))
	if err != nil {
		t.Fatalf("read sut.log: %v", err)
	}
	if n := strings.Count(string(logBytes), "listening on"); n != 2 {
		t.Errorf("sut.log has %d \"listening on\" lines, want 2 (start + restart):\n%s", n, logBytes)
	}

	deadline := time.Now().Add(3 * time.Second)
	for !portFree(addr) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !portFree(addr) {
		t.Errorf("port %s still in use after the run exited: SUT was not stopped", addr)
	}
}

// TestMainRefusesWhenSomethingAlreadyAnswers is the other half: a stranger
// already answering /healthz on the target's port (in the real case, a SUT
// leaked by a panicked run) must make Main refuse before it starts anything,
// rather than run the whole suite against the stranger while its own SUT
// dies at bind.
func TestMainRefusesWhenSomethingAlreadyAnswers(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go test and a real SUT process")
	}
	bin := buildCounterBinary(t)

	// A stranger that answers /healthz, holding the port for the whole test.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })

	root := t.TempDir()
	targetPath := writeTarget(t, root, addr,
		[]string{bin, "--addr", addr, "--engine", "sqlite", "--dsn", filepath.Join(root, "c.db")})

	resultsDir := filepath.Join(root, "results")
	out, err := childSuite(t, "TestContract$",
		EnvTarget+"="+targetPath, EnvResults+"="+resultsDir, EnvRunID+"=run-sut-stale")
	if err == nil {
		t.Fatalf("go test succeeded against a stale answerer; want a refusal:\n%s", out)
	}
	if !strings.Contains(out, "something already answers at http://"+addr) {
		t.Errorf("output missing the stale-answerer refusal:\n%s", out)
	}
	if strings.Contains(out, "started SUT pid") {
		t.Errorf("Main started a SUT anyway:\n%s", out)
	}
	if strings.Contains(out, "--- PASS: TestContract") {
		t.Errorf("the suite ran against the stale answerer:\n%s", out)
	}
}

// Signal the test binary itself, as a terminal interrupt does, both while
// Main waits for health and while a slow suite is collecting results.
func TestMainInterruptStopsSUTAndExports(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and signals a child test binary")
	}
	bin := filepath.Join(t.TempDir(), "counter-fault")
	faultBuild := exec.Command("go", "build", "-o", bin, "./cmd/counter-fault")
	faultBuild.Dir = ModuleRoot()
	if out, err := faultBuild.CombinedOutput(); err != nil {
		t.Fatalf("build counter-fault: %v\n%s", err, out)
	}
	suite := filepath.Join(t.TempDir(), "counter.test")
	build := exec.Command("go", "test", "-race", "-c", "-o", suite, "./suites/counter")
	build.Dir = ModuleRoot()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build suite: %v\n%s", err, out)
	}
	for _, stage := range []string{"health", "load"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
			command := []string{bin, "--addr", addr, "--engine", "memory", "--bug", "slow"}
			if stage == "health" {
				// Keep a live process answering 503 throughout the health wait.
				t.Setenv("SUT_TEST_UNHEALTHY_ADDR", addr)
				command = []string{os.Args[0], "-test.run=TestUnhealthySUTHelper$"}
			}
			target := writeTarget(t, root, addr, command)
			resultDir := filepath.Join(root, "results")
			logPath := filepath.Join(root, "suite.log")
			log, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			child := exec.Command(suite, "-test.v", "-test.run=TestIncrementConcurrent$")
			for _, kv := range os.Environ() {
				if !strings.HasPrefix(kv, "HARNESS_") {
					child.Env = append(child.Env, kv)
				}
			}
			child.Env = append(child.Env, EnvTarget+"="+target, EnvResults+"="+resultDir, healthTimeoutEnv+"=30s")
			child.Stdout, child.Stderr = log, log
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- child.Wait() }()
			t.Cleanup(func() { _ = child.Process.Kill() })
			marker := "started SUT pid"
			if stage == "load" {
				marker = "=== RUN   TestIncrementConcurrent"
			}
			deadline := time.Now().Add(20 * time.Second)
			for {
				out, _ := os.ReadFile(logPath)
				if strings.Contains(string(out), marker) && (stage != "health" || !portFree(addr)) {
					break
				}
				select {
				case err := <-done:
					t.Fatalf("child exited before %s: %v\n%s", stage, err, out)
				default:
				}
				if time.Now().After(deadline) {
					t.Fatalf("waiting for %s:\n%s", stage, out)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := child.Process.Signal(syscall.SIGINT); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Error("interrupted child returned success")
				}
			case <-time.After(15 * time.Second):
				t.Fatal("interrupted child did not exit")
			}
			out, _ := os.ReadFile(logPath)
			if strings.Contains(string(out), "DATA RACE") {
				t.Fatalf("signal race:\n%s", out)
			}
			if !strings.Contains(string(out), "exporting partial results") {
				t.Fatalf("signal not handled:\n%s", out)
			}
			if !portFree(addr) {
				t.Errorf("SUT still listening after interrupt: %s", addr)
			}
			if stage == "load" {
				db, err := results.Query(context.Background(), root)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var n int
				if err := db.QueryRow("SELECT count(*) FROM runs").Scan(&n); err != nil || n != 1 {
					t.Fatalf("partial run not exported: count=%d err=%v\n%s", n, err, out)
				}
			}
		})
	}
}

func TestUnhealthySUTHelper(t *testing.T) {
	addr := os.Getenv("SUT_TEST_UNHEALTHY_ADDR")
	if addr == "" {
		t.Skip("subprocess helper")
	}
	t.Fatal(http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})))
}

func TestCrashSkipsExternalSUT(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a suite")
	}
	srv := fakeSUT(t)
	out, err := childSuite(t, "TestCrashRestart$", EnvURL+"="+srv.URL,
		EnvEngine+"=sqlite", EnvResults+"="+t.TempDir())
	if err != nil || !strings.Contains(out, "--- SKIP: TestCrashRestart") {
		t.Fatalf("external target must skip crash tests: %v\n%s", err, out)
	}
}
