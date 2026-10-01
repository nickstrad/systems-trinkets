//go:build linux

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const ioTimeout = 3 * time.Second

func cmdDial(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("dial"), args, 2, 3)
	if err != nil {
		return err
	}
	network, addr := rest[0], rest[1]
	if network != "unix" && network != "tcp" {
		return usagef("network %q is not unix or tcp", network)
	}
	conn, err := net.DialTimeout(network, addr, ioTimeout)
	if err != nil {
		r.deny("dial", err)
		return nil
	}
	defer conn.Close()
	r.ok("dial", "%s %s", network, addr)
	if len(rest) < 3 {
		return nil
	}
	_ = conn.SetDeadline(time.Now().Add(ioTimeout))
	if _, err := io.WriteString(conn, rest[2]); err != nil {
		r.deny("reply", err)
		return nil
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if reply == "" && err != nil {
		r.deny("reply", err)
		return nil
	}
	r.ok("reply", "%s", trimEOL(reply))
	return nil
}

func trimEOL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// peerCred reads the kernel's record of who is on the other end of a unix
// socket connection (SO_PEERCRED). The PID is 0 when the peer lives in another
// PID namespace, which is why a broker can rely on the UID and GID only.
func peerCred(c *net.UnixConn) (*syscall.Ucred, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *syscall.Ucred
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		cred, sockErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	return cred, sockErr
}

// cmdPeerEcho listens on a unix socket and answers every connection with the
// credentials the kernel reports for it, both to the peer and as a "peer"
// line on its own output, once it has read the client's request line. The
// umask applies to the socket file, so the
// default 007 gives srwxrwx--- (the group may connect, others may not).
func cmdPeerEcho(args []string, r *reporter) error {
	fs := newFlagSet("peer-echo")
	umaskStr := fs.String("umask", "007", "octal umask applied while the socket is created")
	count := fs.Int("count", 0, "exit after this many connections (0 = until stopped)")
	rest, err := parseFlags(fs, args, 1, 1)
	if err != nil {
		return err
	}
	umask, err := parseMode(*umaskStr)
	if err != nil {
		return err
	}
	if *count < 0 {
		return usagef("count %d is negative", *count)
	}
	old := syscall.Umask(int(umask))
	ln, err := net.Listen("unix", rest[0])
	syscall.Umask(old)
	if err != nil {
		r.deny("listen", err)
		return nil
	}
	defer ln.Close()
	r.ok("listen", "%s", rest[0])
	for served := 0; *count == 0 || served < *count; served++ {
		conn, err := ln.Accept()
		if err != nil {
			r.deny("accept", err)
			return nil
		}
		uc := conn.(*net.UnixConn)
		cred, err := peerCred(uc)
		if err != nil {
			r.deny("peer", err)
		} else {
			detail := fmt.Sprintf("uid=%d gid=%d pid=%d", cred.Uid, cred.Gid, cred.Pid)
			r.ok("peer", "%s", detail)
			// Read the request line (or EOF) before answering. Answering and
			// closing first raced a client that writes then reads: its write
			// hit the closed socket (EPIPE) and the reply was never read.
			_ = conn.SetReadDeadline(time.Now().Add(ioTimeout))
			_, _ = bufio.NewReader(conn).ReadString('\n')
			_ = conn.SetWriteDeadline(time.Now().Add(ioTimeout))
			_, _ = io.WriteString(conn, detail+"\n")
		}
		conn.Close()
	}
	return nil
}

// cmdReplace tries to put a socket of its own where path is: bind a unix
// socket beside it, then rename that over path. Both steps need write access
// to the directory, which is what a broker's socket directory withholds from
// workers.
func cmdReplace(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("replace"), args, 1, 1)
	if err != nil {
		return err
	}
	ln, err := net.Listen("unix", rest[0]+".replace")
	if err != nil {
		r.deny("replace", err)
		return nil
	}
	defer ln.Close() // unlinks the temporary name if the rename failed
	r.result("replace", rest[0], os.Rename(rest[0]+".replace", rest[0]))
	return nil
}

// counter counts requests per URL path.
type counter struct {
	mu    sync.Mutex
	paths map[string]int
	total int
}

func (c *counter) hit(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paths[path]++
	c.total++
}

func (c *counter) snapshot() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	paths := make(map[string]int, len(c.paths))
	for p, n := range c.paths {
		paths[p] = n
	}
	return map[string]any{"total": c.total, "paths": paths}
}

// countsPath returns the counts as JSON and is not itself counted.
const countsPath = "/__counts"

func newCountHandler(name string, c *counter) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(countsPath, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(c.snapshot())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		c.hit(req.URL.Path)
		fmt.Fprintf(w, "name=%s path=%s\n", name, req.URL.Path)
	})
	return mux
}

func cmdHTTPCount(args []string, r *reporter) error {
	fs := newFlagSet("http-count")
	name := fs.String("name", "fixture", "name echoed in every answer")
	rest, err := parseFlags(fs, args, 1, 1)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", rest[0])
	if err != nil {
		r.deny("listen", err)
		return nil
	}
	srv := &http.Server{Handler: newCountHandler(*name, &counter{paths: map[string]int{}}), ReadHeaderTimeout: ioTimeout}
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-term; _ = srv.Close() }()
	r.ok("listen", "%s", ln.Addr())
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		r.deny("serve", err)
	}
	return nil
}
