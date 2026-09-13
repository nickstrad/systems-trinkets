// Package sut starts, stops and restarts one system-under-test process.
// It is independent of package harness (harness imports sut, not the
// reverse), so it takes a Spec rather than a harness.Target — the caller
// (harness.Main) is responsible for turning a Target into a Spec (resolving
// Cwd, merging Env, opening the log file).
//
// A common Spec.Cmd is "go run ./cmd/foo", whose real server is a *child* of
// the process this package starts. Every kill and stop below therefore
// targets the whole process group, not just the direct child, so a listener
// started by a grandchild is actually closed.
package sut

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Spec describes one SUT process to start. Stdout and Stderr, if nil, get
// exec.Cmd's default (discarded); harness.Main points them at
// <results dir>/sut.log.
type Spec struct {
	Cmd    []string
	Dir    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// Proc is one started SUT process (and, via its process group, whatever it
// forked). Kill and Stop are safe to call concurrently and repeatedly: each
// only signals the group and waits on exited. harness.Main needs that —
// its signal handler may Stop while a test goroutine is inside H.Restart's
// Kill.
type Proc struct {
	cmd    *exec.Cmd
	exited chan struct{}

	sigMu sync.Mutex // serialises signalGroup so concurrent Kill/Stop don't interleave

	mu      sync.Mutex // guards waitErr, read only after exited is closed
	waitErr error
}

// Start execs spec.Cmd with a new process group (Setpgid) and returns
// immediately; it does not wait for the SUT to become healthy — the caller
// polls GET /healthz.
func Start(spec Spec) (*Proc, error) {
	if len(spec.Cmd) == 0 {
		return nil, errors.New("sut: empty Cmd")
	}
	cmd := exec.Command(spec.Cmd[0], spec.Cmd[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sut: start %v: %w", spec.Cmd, err)
	}

	p := &Proc{cmd: cmd, exited: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.waitErr = err
		p.mu.Unlock()
		close(p.exited)
	}()
	return p, nil
}

// PID is the direct child's process id (also its process group id, since
// Start sets Setpgid).
func (p *Proc) PID() int { return p.cmd.Process.Pid }

// Exited is closed once the process has been reaped, whether it was killed,
// stopped cleanly, or died on its own — the last case is what lets Main
// notice a SUT that crashed on its own.
func (p *Proc) Exited() <-chan struct{} { return p.exited }

// ExitState is the process's exit state once Exited() has closed (nil
// before then). Tests use it to tell a clean SIGTERM exit from a SIGKILL —
// e.g. via ExitState().Sys().(syscall.WaitStatus).Signal().
func (p *Proc) ExitState() *os.ProcessState {
	select {
	case <-p.exited:
	default:
		return nil
	}
	return p.cmd.ProcessState
}

// Err is cmd.Wait's error once Exited() has closed (nil before then, and
// nil for a clean exit) — e.g. "signal: killed" after Kill, or the reason a
// SUT that Main did not touch died on its own.
func (p *Proc) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

// Kill sends SIGKILL to the whole process group and waits for it to be
// reaped. It is safe to call on a process that has already exited, and safe
// to call concurrently with Stop.
func (p *Proc) Kill() error {
	if p.alreadyExited() {
		return nil
	}
	if err := p.signalGroup(syscall.SIGKILL); err != nil {
		return fmt.Errorf("sut: kill: %w", err)
	}
	<-p.exited
	return nil
}

// Stop sends SIGTERM to the whole process group and waits up to grace for it
// to exit on its own; past that it falls through to Kill. It is safe to call
// on a process that has already exited, and safe to call concurrently with
// Kill.
func (p *Proc) Stop(grace time.Duration) error {
	if p.alreadyExited() {
		return nil
	}
	if err := p.signalGroup(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sut: stop: %w", err)
	}
	select {
	case <-p.exited:
		return nil
	case <-time.After(grace):
		return p.Kill()
	}
}

func (p *Proc) alreadyExited() bool {
	select {
	case <-p.exited:
		return true
	default:
		return false
	}
}

// signalGroup signals the process group led by p's direct child so a
// grandchild (e.g. the real server under "go run") receives it too. ESRCH
// (already gone) is not an error — Kill/Stop are meant to be safe to call on
// a process that exited on its own.
func (p *Proc) signalGroup(sig syscall.Signal) error {
	p.sigMu.Lock()
	defer p.sigMu.Unlock()
	if p.alreadyExited() {
		return nil // a concurrent Kill/Stop already reaped it
	}
	// Start set Setpgid, so our group id is our child's pid. Confirming that
	// before signalling -pid is what keeps a concurrent Kill from signalling
	// a *stranger*: if the child has been reaped since alreadyExited above,
	// its pid may already belong to someone else, whose group id then almost
	// never matches. os.Process.Signal is the safe fallback — Go refuses it
	// once cmd.Wait has reaped the process.
	pid := p.cmd.Process.Pid
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid {
		if err := syscall.Kill(-pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
	if err := p.cmd.Process.Signal(sig); err != nil &&
		!errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
