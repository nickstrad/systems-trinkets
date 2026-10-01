//go:build linux

package main

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// forwardTimeout bounds each forwarded connection from accept to close. A
// fixture request through the broker takes milliseconds; the cap keeps a
// stalled client from holding the forwarder's shutdown (it waits for open
// connections) until the engine kills it. A variable so tests can shorten it.
var forwardTimeout = 10 * time.Second

// cmdForward is the minimal egress broker for the H6 topology: it listens on a
// unix socket and copies every connection, byte for byte, to one fixed TCP
// destination. It never reads what it forwards, so it cannot choose another
// destination or follow a redirect; destination and redirect policy are the
// lesson's (worker-egress-grants), not this fixture's. Each connection lives
// at most forwardTimeout. SIGTERM closes the listener, which removes the
// socket file; the command exits 0 once open connections end.
func cmdForward(args []string, r *reporter) error {
	fs := newFlagSet("forward")
	umaskStr := fs.String("umask", "007", "octal umask applied while the socket is created")
	rest, err := parseFlags(fs, args, 2, 2)
	if err != nil {
		return err
	}
	socket, dest := rest[0], rest[1]
	if _, _, err := net.SplitHostPort(dest); err != nil {
		return usagef("destination %q is not host:port: %v", dest, err)
	}
	umask, err := parseMode(*umaskStr)
	if err != nil {
		return err
	}
	old := syscall.Umask(int(umask))
	ln, err := net.Listen("unix", socket)
	syscall.Umask(old)
	if err != nil {
		r.deny("listen", err)
		return nil
	}
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(term)
	go func() { <-term; _ = ln.Close() }()

	var mu sync.Mutex // connections report from their own goroutines
	report := func(f func()) { mu.Lock(); defer mu.Unlock(); f() }
	r.ok("listen", "%s -> %s", socket, dest)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		conn, err := ln.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			report(func() { r.deny("accept", err) })
			return nil
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conn.Close()
			deadline := time.Now().Add(forwardTimeout)
			_ = conn.SetDeadline(deadline)
			up, err := net.DialTimeout("tcp", dest, ioTimeout)
			if err != nil {
				report(func() { r.deny("forward", err) })
				return
			}
			defer up.Close()
			_ = up.SetDeadline(deadline)
			report(func() { r.ok("forward", "%s", dest) })
			pipe(conn.(*net.UnixConn), up.(*net.TCPConn))
		}()
	}
}

// pipe copies both ways until each side has finished sending, passing a
// half-close on so an HTTP/1.0 client and server both see the end.
func pipe(client *net.UnixConn, up *net.TCPConn) {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(up, client)
		_ = up.CloseWrite()
		close(done)
	}()
	_, _ = io.Copy(client, up)
	_ = client.CloseWrite()
	<-done
}

// cmdHTTPGet sends one GET and reports the status code and the first line of
// the body. Like any Go client it honours HTTP_PROXY, HTTPS_PROXY and
// NO_PROXY from the environment; it follows no redirect and reports a 3xx as
// it came.
func cmdHTTPGet(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("http-get"), args, 1, 1)
	if err != nil {
		return err
	}
	u, err := url.Parse(rest[0])
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return usagef("%q is not an http://host[:port]/path URL", rest[0])
	}
	cl := &http.Client{
		Timeout:       ioTimeout,
		Transport:     &http.Transport{Proxy: http.ProxyFromEnvironment},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := cl.Get(u.String())
	if err != nil {
		r.deny("http-get", err)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	first, _, _ := strings.Cut(string(body), "\n")
	r.ok("http-get", "%d %s", resp.StatusCode, trimEOL(first))
	return nil
}
