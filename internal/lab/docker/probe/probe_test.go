//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// syncBuffer is a bytes.Buffer safe for a server goroutine to write while the
// test reads.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// probe runs a command in this process and returns its exit status and
// parsed lines, failing the test if any line is not a well-formed probe line.
func probe(t *testing.T, args ...string) (int, []probeout.Line) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	lines := probeout.Parse(out.String())
	for _, l := range probeout.Malformed(lines) {
		t.Errorf("probe %v printed a malformed line %q: %v", args, l.Raw, l.Err)
	}
	return code, lines
}

func wantLine(t *testing.T, lines []probeout.Line, name string, ok bool) probeout.Line {
	t.Helper()
	l, found := probeout.Find(lines, name)
	if !found {
		t.Fatalf("no %q line in %q", name, probeout.Format(lines))
	}
	if l.OK != ok {
		t.Fatalf("%s: OK=%v, want %v (%q)", name, l.OK, ok, l.Detail)
	}
	return l
}

// TestUsageErrorsExit2 runs only argument sets that fail before any action,
// so setting the marker (to get past the guard) cannot change the machine.
func TestUsageErrorsExit2(t *testing.T) {
	t.Setenv(markerEnv, "1")
	for _, args := range [][]string{
		nil, {"nope"}, {"write"}, {"write", "a", "b"}, {"chown", "/x", "nocolon"}, {"chmod", "/x", "9"},
		{"chmod", "/x", "17777"}, {"setuid", "root"}, {"alloc", "-1"}, {"alloc"}, {"fork", "0"},
		{"dial", "udp", "x"}, {"peer-echo"}, {"sleep", "-x"}, {"sleep", "abc"}, {"initdir", "/x", "1:2"},
		{"id", "extra"}, {"http-count"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 {
			t.Errorf("run(%v) = %d, want 2 (stderr %q)", args, code, errOut.String())
		}
		if out.Len() != 0 {
			t.Errorf("run(%v) wrote to stdout on a usage error: %q", args, out.String())
		}
	}
	var out bytes.Buffer
	if code := run([]string{"help"}, &out, io.Discard); code != 0 || !strings.Contains(out.String(), "peer-echo") {
		t.Errorf("help = %d %q", code, out.String())
	}
}

// TestMutatingCommandsRefuseWithoutMarker is the guard: without
// TRINKETS_PROBE=1 a command that changes the machine exits 3 and does
// nothing. The commands run here have harmless arguments (a temp file), so a
// broken guard cannot hurt the host; the table test below pins which commands
// are flagged, which is what makes the rest safe.
func TestMutatingCommandsRefuseWithoutMarker(t *testing.T) {
	t.Setenv(markerEnv, "")
	os.Unsetenv(markerEnv)
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	owner := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	for _, args := range [][]string{
		{"write", dir}, {"chmod", file, "0644"}, {"chown", file, owner},
		{"unlink", file}, {"initdir", dir, owner, "0755"}, {"alloc", "1"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 3 {
			t.Errorf("run(%v) without the marker = %d, want 3", args, code)
		}
		if out.Len() != 0 || !strings.Contains(errOut.String(), markerEnv) {
			t.Errorf("run(%v): stdout %q stderr %q", args, out.String(), errOut.String())
		}
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("a refused command changed the file: %v %v", info, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("a refused command left %d entries, want only the file", len(entries))
	}
}

func TestWhichCommandsAreGuarded(t *testing.T) {
	want := map[string]bool{
		"write": true, "setuid": true, "chown": true, "chmod": true, "unlink": true, "mount": true,
		"unshare-user": true, "keyctl": true, "battery": true, "alloc": true, "fork": true, "initdir": true,
	}
	for _, c := range commands {
		if c.mutates != want[c.name] {
			t.Errorf("%s: mutates=%v, want %v", c.name, c.mutates, want[c.name])
		}
		delete(want, c.name)
	}
	if len(want) != 0 {
		t.Errorf("guarded commands missing from the table: %v", want)
	}
}

func TestParseHelpers(t *testing.T) {
	if u, g, err := parseOwner("20000:30000"); err != nil || u != 20000 || g != 30000 {
		t.Errorf("parseOwner = %d %d %v", u, g, err)
	}
	for _, bad := range []string{"", "1", "1:", ":1", "a:1", "-1:1", "1:-1", "1:2:3"} {
		if _, _, err := parseOwner(bad); err == nil {
			t.Errorf("parseOwner(%q) accepted", bad)
		}
	}
	for in, want := range map[string]uint32{"0": 0, "2750": 0o2750, "7777": 0o7777, "007": 7} {
		if got, err := parseMode(in); err != nil || got != want {
			t.Errorf("parseMode(%q) = %o %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "8", "17777", "-1", "x"} {
		if _, err := parseMode(bad); err == nil {
			t.Errorf("parseMode(%q) accepted", bad)
		}
	}
	for mode, want := range map[uint32]string{
		syscall.S_IFSOCK | 0o770: "srwxrwx---",
		syscall.S_IFDIR | 0o2750: "drwxr-s---",
		syscall.S_IFDIR | 0o2640: "drw-r-S---",
		syscall.S_IFDIR | 0o1777: "drwxrwxrwt",
		syscall.S_IFDIR | 0o1770: "drwxrwx--T",
		syscall.S_IFREG | 0o4755: "-rwsr-xr-x",
		syscall.S_IFREG | 0o644:  "-rw-r--r--",
		syscall.S_IFLNK | 0o777:  "lrwxrwxrwx",
	} {
		if got := fileMode(mode); got != want {
			t.Errorf("fileMode(%o) = %q, want %q", mode, got, want)
		}
	}
}

func TestReadOnlyFacts(t *testing.T) {
	_, id := probe(t, "id")
	l := wantLine(t, id, "identity", true)
	if want := fmt.Sprintf("uid=%d gid=%d ", os.Getuid(), os.Getgid()); !strings.HasPrefix(l.Detail, want) {
		t.Errorf("identity detail %q does not start with %q", l.Detail, want)
	}
	_, st := probe(t, "status")
	for _, name := range []string{"cap-eff", "no-new-privs", "seccomp"} {
		wantLine(t, st, name, true)
	}
	_, pids := probe(t, "pids")
	if d := wantLine(t, pids, "pids", true).Detail; !strings.Contains(" "+d+" ", fmt.Sprintf(" %d ", os.Getpid())) {
		t.Errorf("pids %q does not list this process %d", d, os.Getpid())
	}
	_, ifs := probe(t, "interfaces")
	if d := wantLine(t, ifs, "interfaces", true).Detail; !strings.Contains(" "+d+" ", " lo ") {
		t.Errorf("interfaces %q has no lo", d)
	}
	// cgroup files may be absent on a cgroup v1 host; either answer is a line.
	_, cg := probe(t, "cgroup")
	for _, name := range []string{"memory.max", "pids.max", "cpu.max"} {
		if _, ok := probeout.Find(cg, name); !ok {
			t.Errorf("no %s line", name)
		}
	}
}

// waitFor polls until cond holds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestPeerEchoReportsPeerCredentials(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "s.sock")
	var out syncBuffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"peer-echo", "--count", "1", sock}, &out, io.Discard) }()
	waitFor(t, "listen line", func() bool { return strings.Contains(out.String(), "listen: OK") })

	_, stLines := probe(t, "stat", sock)
	if d := wantLine(t, stLines, "stat", true).Detail; !strings.Contains(d, "mode=srwxrwx---") {
		t.Errorf("socket mode %q, want srwxrwx--- from the default umask 007", d)
	}
	code, lines := probe(t, "dial", "unix", sock, "hi\n")
	if code != 0 {
		t.Errorf("dial = %d", code)
	}
	want := fmt.Sprintf("uid=%d gid=%d pid=%d", os.Getuid(), os.Getgid(), os.Getpid())
	if got := wantLine(t, lines, "reply", true).Detail; got != want {
		t.Errorf("reply %q, want %q (same process, so the PID is real)", got, want)
	}
	if c := <-done; c != 0 {
		t.Errorf("peer-echo exit = %d", c)
	}
	if got := probeout.Parse(out.String()); len(got) != 2 {
		t.Fatalf("peer-echo output %q", out.String())
	} else if p := wantLine(t, got, "peer", true); p.Detail != want {
		t.Errorf("peer line %q, want %q", p.Detail, want)
	}
}

func TestPeerEchoBindFailureIsDenied(t *testing.T) {
	code, lines := probe(t, "peer-echo", "--count", "1", filepath.Join(t.TempDir(), "no", "such", "dir.sock"))
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	wantLine(t, lines, "listen", false)
}

func TestDialRefusedIsDenied(t *testing.T) {
	code, lines := probe(t, "dial", "unix", filepath.Join(t.TempDir(), "none.sock"))
	if code != 1 {
		t.Errorf("exit = %d", code)
	}
	wantLine(t, lines, "dial", false)
}

func TestHTTPCountCountsPerPath(t *testing.T) {
	var out syncBuffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"http-count", "--name", "allowed", "127.0.0.1:0"}, &out, io.Discard) }()
	waitFor(t, "listen line", func() bool { return strings.Contains(out.String(), "listen: OK") })
	addr := wantLine(t, probeout.Parse(out.String()), "listen", true).Detail

	get := func(path string) string {
		resp, err := http.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if body := get("/a"); body != "name=allowed path=/a\n" {
		t.Errorf("body %q", body)
	}
	get("/a")
	get("/b/c")
	var counts struct {
		Total int
		Paths map[string]int
	}
	if err := json.Unmarshal([]byte(get(countsPath)), &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Total != 3 || counts.Paths["/a"] != 2 || counts.Paths["/b/c"] != 1 || len(counts.Paths) != 2 {
		t.Errorf("counts = %+v; the counts endpoint must not count itself", counts)
	}
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM) // http-count's handler stops it
	if c := <-done; c != 0 {
		t.Errorf("exit = %d", c)
	}
}

func TestSleepHandlesSIGTERM(t *testing.T) {
	var out syncBuffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"sleep"}, &out, io.Discard) }()
	waitFor(t, "start line", func() bool { return strings.Contains(out.String(), "sleep: OK pid=") })
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	select {
	case c := <-done:
		if c != 0 {
			t.Errorf("exit = %d", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sleep ignored SIGTERM")
	}
	wantLine(t, probeout.Parse(out.String()), "signal", true)
}

func TestSleepEndsAfterDuration(t *testing.T) {
	code, lines := probe(t, "sleep", "0.05")
	if code != 0 {
		t.Errorf("exit = %d", code)
	}
	if len(lines) != 2 || lines[1].Detail != "done" {
		t.Errorf("lines %q", probeout.Format(lines))
	}
}

func TestSleepIgnoreTermSurvivesSIGTERM(t *testing.T) {
	var out syncBuffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"sleep", "--ignore-term", "0.4"}, &out, io.Discard) }()
	waitFor(t, "start line", func() bool { return strings.Contains(out.String(), "ignore-term=true") })
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	if c := <-done; c != 0 {
		t.Errorf("exit = %d", c)
	}
	lines := probeout.Parse(out.String())
	if _, ok := probeout.Find(lines, "signal"); ok {
		t.Errorf("--ignore-term acted on SIGTERM: %q", out.String())
	}
	if last := lines[len(lines)-1]; last.Name != "sleep" || last.Detail != "done" {
		t.Errorf("did not sleep to the end: %q", out.String())
	}
}
