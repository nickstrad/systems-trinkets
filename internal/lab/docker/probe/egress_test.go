//go:build linux

package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

func TestEgressUsageErrorsExit2(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "s.sock")
	for _, args := range [][]string{
		{"forward"}, {"forward", sock}, {"forward", sock, "no-port"}, {"forward", "--umask", "9", sock, "a:1"},
		{"http-get"}, {"http-get", "ftp://x/"}, {"http-get", "https://x/"}, {"http-get", "/path"}, {"http-get", "a", "b"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 || out.Len() != 0 {
			t.Errorf("run(%v) = %d, stdout %q; want 2 and no stdout", args, code, out.String())
		}
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("a usage error created the socket: %v", err)
	}
}

// redirector answers every request with a 302 to other and counts them;
// other counts what reaches it. A forwarder or client that followed the
// redirect would show up as a hit on other.
func redirector(t *testing.T) (front, other *httptest.Server, frontHits, otherHits *atomic.Int32) {
	frontHits, otherHits = new(atomic.Int32), new(atomic.Int32)
	other = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		io.WriteString(w, "other\n")
	}))
	front = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frontHits.Add(1)
		http.Redirect(w, r, other.URL+"/moved", http.StatusFound)
	}))
	t.Cleanup(front.Close)
	t.Cleanup(other.Close)
	return front, other, frontHits, otherHits
}

func TestForwardCopiesToOneDestinationAndFollowsNoRedirect(t *testing.T) {
	front, _, frontHits, otherHits := redirector(t)
	sock := filepath.Join(t.TempDir(), "egress.sock")
	var out syncBuffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"forward", sock, front.Listener.Addr().String()}, &out, io.Discard) }()
	waitFor(t, "listen line", func() bool { return strings.Contains(out.String(), "listen: OK") })

	info, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	// umask 007 on a socket created 0777: srwxrwx---.
	if got := info.Mode().Perm(); got != 0o770 {
		t.Errorf("socket mode %o, want 770", got)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(conn, "GET /go HTTP/1.0\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	conn.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("status %d, want the 302 passed through", resp.StatusCode)
	}
	if f, o := frontHits.Load(), otherHits.Load(); f != 1 || o != 0 {
		t.Errorf("hits: destination %d (want 1), redirect target %d (want 0)", f, o)
	}

	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM) // forward's handler closes the listener
	select {
	case c := <-done:
		if c != 0 {
			t.Errorf("exit = %d (%q)", c, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("forward ignored SIGTERM")
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("socket file still there after SIGTERM: %v", err)
	}
	lines := probeout.Parse(out.String())
	if l := wantLine(t, lines, "forward", true); l.Detail != front.Listener.Addr().String() {
		t.Errorf("forward line %q", l.Detail)
	}
}

func TestForwardReportsAnUnreachableDestination(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := ln.Addr().String()
	ln.Close()
	sock := filepath.Join(t.TempDir(), "egress.sock")
	var out syncBuffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"forward", sock, dead}, &out, io.Discard) }()
	waitFor(t, "listen line", func() bool { return strings.Contains(out.String(), "listen: OK") })
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if n, _ := conn.Read(make([]byte, 1)); n != 0 {
		t.Errorf("read %d bytes from a connection with no upstream", n)
	}
	conn.Close()
	waitFor(t, "forward line", func() bool { return strings.Contains(out.String(), "forward: DENIED") })
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	if c := <-done; c != 1 {
		t.Errorf("exit = %d, want 1 after a DENIED line", c)
	}
}

func TestHTTPGetReportsStatusAndFollowsNoRedirect(t *testing.T) {
	front, other, frontHits, otherHits := redirector(t)
	code, lines := probe(t, "http-get", front.URL+"/x")
	if code != 0 {
		t.Errorf("exit = %d", code)
	}
	if l := wantLine(t, lines, "http-get", true); !strings.HasPrefix(l.Detail, "302 ") {
		t.Errorf("detail %q, want the 302 itself", l.Detail)
	}
	if f, o := frontHits.Load(), otherHits.Load(); f != 1 || o != 0 {
		t.Errorf("hits: front %d (want 1), redirect target %d (want 0)", f, o)
	}
	if _, lines := probe(t, "http-get", other.URL+"/y"); wantLine(t, lines, "http-get", true).Detail != "200 other" {
		t.Errorf("200 detail %q", probeout.Format(lines))
	}
	other.Close()
	if code, lines := probe(t, "http-get", other.URL+"/z"); code != 1 {
		t.Errorf("exit = %d for a closed server", code)
	} else {
		wantLine(t, lines, "http-get", false)
	}
}

// A client that connects and then sends nothing must not hold the
// forwarder's shutdown: its connection ends at forwardTimeout. The
// destination is an HTTP server that would wait for a request forever.
func TestForwardDeadlineEndsAStalledConnection(t *testing.T) {
	old := forwardTimeout
	forwardTimeout = 300 * time.Millisecond
	t.Cleanup(func() { forwardTimeout = old })
	front, _, _, _ := redirector(t)
	sock := filepath.Join(t.TempDir(), "egress.sock")
	var out syncBuffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"forward", sock, front.Listener.Addr().String()}, &out, io.Discard) }()
	waitFor(t, "listen line", func() bool { return strings.Contains(out.String(), "listen: OK") })

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	waitFor(t, "forward line", func() bool { return strings.Contains(out.String(), "forward: OK") })
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	select {
	case c := <-done:
		if c != 0 {
			t.Errorf("exit = %d (%q)", c, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled connection held the forwarder past its deadline")
	}
	// The forwarder closed its side: the client reads EOF, not a timeout.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, err := conn.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Errorf("stalled client read %d bytes, err %v; want EOF", n, err)
	}
}
