//go:build linux

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// cmdSleep waits, announcing its PID on stdout so a test can tell it started.
// By default SIGTERM ends it with exit 0; with --ignore-term SIGTERM is
// received and discarded, so only SIGKILL stops it. Both are explicit: as PID 1 with no
// handler, the kernel would drop SIGTERM whatever the Go runtime does.
func cmdSleep(args []string, r *reporter) error {
	fs := newFlagSet("sleep")
	ignore := fs.Bool("ignore-term", false, "ignore SIGTERM")
	rest, err := parseFlags(fs, args, 0, 1)
	if err != nil {
		return err
	}
	var timeout <-chan time.Time // nil blocks forever
	if len(rest) == 1 {
		secs, err := strconv.ParseFloat(rest[0], 64)
		if err != nil || secs < 0 {
			return usagef("seconds %q is not a non-negative number", rest[0])
		}
		timeout = time.After(time.Duration(secs * float64(time.Second)))
	}
	// Both modes install a handler. Ignoring means receiving SIGTERM and
	// discarding it; signal.Ignore would leave nothing that can wake the
	// program, and the runtime would abort it with "all goroutines are asleep".
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	r.ok("sleep", "pid=%d ignore-term=%t", os.Getpid(), *ignore)
	for {
		select {
		case <-term:
			if !*ignore {
				r.ok("signal", "SIGTERM")
				return nil
			}
		case <-timeout:
			r.ok("sleep", "done")
			return nil
		}
	}
}

// cmdAlloc touches n bytes, one write per page, so the memory is resident and
// counts against a cgroup limit. A container that exceeds its limit is killed
// before the OK line prints.
func cmdAlloc(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("alloc"), args, 1, 1)
	if err != nil {
		return err
	}
	n, err := strconv.ParseInt(rest[0], 10, 64)
	if err != nil || n < 0 || n > 1<<40 {
		return usagef("bytes %q is not an integer in 0..2^40", rest[0])
	}
	buf := make([]byte, n)
	for i := 0; i < len(buf); i += os.Getpagesize() {
		buf[i] = 1
	}
	r.ok("alloc", "%d", len(buf))
	return nil
}

const maxFork = 1024

// cmdFork starts up to n children (each a sleeping copy of this binary) until
// one fails, then stops them all. Threads count against a PID limit too, and
// every Go child has several, so the count it reaches is a fact about this
// binary under the limit, not a round number.
func cmdFork(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("fork"), args, 1, 1)
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(rest[0])
	if err != nil || n < 1 || n > maxFork {
		return usagef("n %q is not an integer in 1..%d", rest[0], maxFork)
	}
	var children []*exec.Cmd
	var startErr error
	for len(children) < n {
		cmd := exec.Command("/proc/self/exe", "sleep", "60")
		if startErr = cmd.Start(); startErr != nil {
			break
		}
		children = append(children, cmd)
	}
	for _, c := range children {
		_ = c.Process.Kill()
		_ = c.Wait()
	}
	r.ok("fork-started", "%d", len(children))
	r.result("fork", strconv.Itoa(n)+" requested", startErr)
	return nil
}

// cmdInitDir is the volume init step: give a directory to a numeric owner and
// set its mode. chmod comes last because chown clears the setgid bit.
func cmdInitDir(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("initdir"), args, 3, 3)
	if err != nil {
		return err
	}
	uid, gid, err := parseOwner(rest[1])
	if err != nil {
		return err
	}
	mode, err := parseMode(rest[2])
	if err != nil {
		return err
	}
	if err := os.Chown(rest[0], uid, gid); err != nil {
		r.deny("initdir", err)
		return nil
	}
	r.result("initdir", rest[0]+" "+rest[1]+" "+rest[2], pathErr("chmod", rest[0], syscall.Chmod(rest[0], mode)))
	return nil
}
