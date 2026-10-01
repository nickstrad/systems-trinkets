//go:build linux

// Command probe is the harness's own fixture binary: a static program that
// reports what it can see and do inside a container, in the line format of
// internal/lab/docker/probeout. The harness builds it into a FROM scratch
// image (BuildFixture) and its tests run it under different isolation
// settings. It reports facts, never verdicts: a check prints "name: OK
// <value>" or "name: DENIED (<error>)" and the lesson decides what that means.
//
// Exit status: 0 when every printed line is OK (and always for battery, which
// is a survey), 1 when a line is DENIED, 2 for a usage error, 3 when it
// refuses to run.
//
// Guard: commands that change the machine (write, setuid, chown, chmod,
// unlink, mount, unshare-user, keyctl, battery, alloc, fork, initdir) refuse
// to run unless TRINKETS_PROBE=1 is in the environment. BuildFixture bakes
// that variable into every fixture image, so inside a harness container it is
// always set, and running this binary by hand on a host (a stray go run, a
// host-side test) cannot mount, chown or fork there. It stops accidents; it is
// not a security boundary, since anyone can set the variable.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// markerEnv is the variable BuildFixture sets in every fixture image.
const markerEnv = "TRINKETS_PROBE"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// reporter prints probe lines as they happen (a process killed mid-run still
// leaves the lines it reached) and counts the denied ones.
type reporter struct {
	w      io.Writer
	denied int
}

func (r *reporter) line(l probeout.Line) {
	if !l.OK {
		r.denied++
	}
	fmt.Fprintln(r.w, l.String())
}

func (r *reporter) ok(name, format string, a ...any) {
	r.line(probeout.OK(name, fmt.Sprintf(format, a...)))
}

func (r *reporter) deny(name string, err error) { r.line(probeout.Denied(name, err)) }

// result reports ok(detail) when err is nil and the error otherwise.
func (r *reporter) result(name, detail string, err error) {
	if err != nil {
		r.deny(name, err)
		return
	}
	r.ok(name, "%s", detail)
}

// usageError makes run print the message and the usage text and exit 2.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }

type command struct {
	name, args, doc string
	survey          bool // exit 0 even when a line is DENIED
	mutates         bool // changes the machine: refused without TRINKETS_PROBE=1
	fn              func(args []string, r *reporter) error
}

var commands []command

func init() {
	commands = []command{
		{name: "id", doc: "uid, gid, effective ids and supplementary groups", fn: cmdID},
		{name: "status", doc: "CapEff, NoNewPrivs and Seccomp from /proc/self/status", fn: cmdStatus},
		{name: "pids", doc: "process IDs visible in /proc", fn: cmdPIDs},
		{name: "interfaces", doc: "network interface names", fn: cmdInterfaces},
		{name: "write", mutates: true, args: "<dir>", doc: "create and remove a file in dir", fn: cmdWrite},
		{name: "setuid", mutates: true, args: "[uid]", doc: "setuid(uid), default 0", fn: cmdSetuid},
		{name: "chown", mutates: true, args: "<path> <uid:gid>", doc: "chown path", fn: cmdChown},
		{name: "chmod", mutates: true, args: "<path> <octal>", doc: "chmod path, setgid and sticky bits included", fn: cmdChmod},
		{name: "unlink", mutates: true, args: "<path>", doc: "unlink path", fn: cmdUnlink},
		{name: "stat", args: "<path>", doc: "mode, owner and group of path (no symlink follow)", fn: cmdStat},
		{name: "mount", mutates: true, doc: "mount a tmpfs on /dev/shm, then unmount it", fn: cmdMount},
		{name: "unshare-user", mutates: true, doc: "start a child in a new user namespace (clone, not unshare: a Go process is multi-threaded)", fn: cmdUnshareUser},
		{name: "keyctl", mutates: true, doc: "keyctl(GET_KEYRING_ID) for the thread keyring", fn: cmdKeyctl},
		{name: "cgroup", doc: "memory.max, pids.max and cpu.max from /sys/fs/cgroup", fn: cmdCgroup},
		{name: "battery", mutates: true, doc: "id, status, pids, interfaces, write-root, mount, unshare-user, keyctl, cgroup, then setuid", survey: true, fn: cmdBattery},
		{name: "dial", args: "<unix|tcp> <addr> [message]", doc: "connect; with a message send it and report the first reply line", fn: cmdDial},
		{name: "peer-echo", args: "[--umask 007] [--count n] <socket>", doc: "listen on a unix socket and report each peer's SO_PEERCRED", fn: cmdPeerEcho},
		{name: "http-count", args: "[--name n] <addr>", doc: "HTTP server counting requests per path; GET /__counts returns JSON", fn: cmdHTTPCount},
		{name: "http-get", args: "<url>", doc: "one GET (honours HTTP_PROXY, follows no redirect); report status and first body line", fn: cmdHTTPGet},
		{name: "forward", args: "[--umask 007] <socket> <host:port>", doc: "copy every unix socket connection to one fixed TCP destination", fn: cmdForward},
		{name: "sleep", args: "[--ignore-term] [seconds]", doc: "wait; exit 0 on SIGTERM unless --ignore-term", fn: cmdSleep},
		{name: "alloc", mutates: true, args: "<bytes>", doc: "allocate and touch memory, then exit", fn: cmdAlloc},
		{name: "fork", mutates: true, args: "<n>", doc: "start up to n child processes and report how many started", fn: cmdFork},
		{name: "initdir", mutates: true, args: "<path> <uid:gid> <octal mode>", doc: "chown then chmod a directory (the volume init step)", fn: cmdInitDir},
		{name: "noop", doc: "exit 0 (used as a child process)", fn: func([]string, *reporter) error { return nil }},
	}
}

func usageText() string {
	var b strings.Builder
	b.WriteString("usage: probe <command> [args]\n")
	for _, c := range commands {
		fmt.Fprintf(&b, "  %-12s %s\n", c.name, strings.TrimSpace(c.args+"  "+c.doc))
	}
	return b.String()
}

// run executes one command and returns the exit status. Output goes to
// stdout; usage problems go to stderr.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText())
		return 2
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, usageText())
		return 0
	}
	for _, c := range commands {
		if c.name != args[0] {
			continue
		}
		if c.mutates && os.Getenv(markerEnv) != "1" {
			fmt.Fprintf(stderr, "probe %s: refusing to run outside a harness fixture image (%s=1 is not set)\n", c.name, markerEnv)
			return 3
		}
		r := &reporter{w: stdout}
		if err := c.fn(args[1:], r); err != nil {
			var ue usageError
			if errors.As(err, &ue) || errors.Is(err, flag.ErrHelp) {
				fmt.Fprintf(stderr, "probe %s: %v\nusage: probe %s %s\n", c.name, err, c.name, c.args)
				return 2
			}
			r.deny(c.name, err)
		}
		if r.denied > 0 && !c.survey {
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "probe: unknown command %q\n%s", args[0], usageText())
	return 2
}

// newFlagSet is a flag set that reports instead of exiting, with its own
// output discarded (run prints the usage line).
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags parses fs and enforces the positional count, turning problems
// into usage errors.
func parseFlags(fs *flag.FlagSet, args []string, min, max int) ([]string, error) {
	if err := fs.Parse(args); err != nil {
		return nil, usagef("%v", err)
	}
	if n := fs.NArg(); n < min || n > max {
		want := fmt.Sprintf("%d to %d", min, max)
		if min == max {
			want = strconv.Itoa(min)
		}
		return nil, usagef("got %d arguments, want %s", n, want)
	}
	return fs.Args(), nil
}
