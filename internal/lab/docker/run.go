package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// cleanupTimeout bounds a removal made on the way out. It runs on a context
// detached from the caller's, so a cancelled or expired ctx still cleans up.
const cleanupTimeout = 30 * time.Second

// execPollInterval is how often Exec re-reads an exec that has closed its
// output but is not yet recorded as exited.
const execPollInterval = 10 * time.Millisecond

// Result is the outcome of a one-shot container or an exec.
type Result struct {
	ExitCode  int // -1 when the process did not finish (cancelled exec)
	Stdout    string
	Stderr    string
	OOMKilled bool          // the container was killed for exceeding Memory (Run only)
	Duration  time.Duration // start to exit
}

// Container is a handle to one created container. Every method takes a
// context. Remove it when done (Run does that itself); Sweep is the net for a
// handle a crash lost.
type Container struct {
	ID, Name string
	// Request is what was sent to the engine, after Translate.
	Request Request
	cli     *client.Client
}

// Create translates spec, checks the config invariants, creates the
// container and checks inspect-matches-intent. Nothing is created when
// Translate or CheckConfig refuses; a container whose recorded config differs
// from the request is removed again. The container is not started.
func Create(ctx context.Context, cli *client.Client, spec Spec) (*Container, error) {
	socket := SocketPath(cli.DaemonHost())
	req, err := Translate(spec, socket, NameSuffix())
	if err != nil {
		return nil, err
	}
	if bad := CheckConfig(req, socket); len(bad) > 0 {
		return nil, fmt.Errorf("docker: refusing to create %s: %s", req.Name, strings.Join(bad, "; "))
	}
	cfg, hc := req.Config, req.HostConfig // the client takes pointers; keep req unshared
	res, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: req.Name, Config: &cfg, HostConfig: &hc})
	if err != nil {
		return nil, fmt.Errorf("docker: create %s: %w", req.Name, err)
	}
	c := &Container{ID: res.ID, Name: req.Name, Request: req, cli: cli}
	insp, err := c.Inspect(ctx)
	if err == nil {
		if bad := CheckObserved(insp, req); len(bad) > 0 {
			err = fmt.Errorf("docker: %s: %s", req.Name, strings.Join(bad, "; "))
		}
	}
	if err != nil {
		return nil, errors.Join(err, c.Remove(ctx))
	}
	return c, nil
}

// Start creates and starts a long-running container, for Exec and Stop. A
// start failure (a missing entrypoint, say) removes the container and is
// returned.
func Start(ctx context.Context, cli *client.Client, spec Spec) (*Container, error) {
	c, err := Create(ctx, cli, spec)
	if err != nil {
		return nil, err
	}
	if _, err := cli.ContainerStart(ctx, c.ID, client.ContainerStartOptions{}); err != nil {
		return nil, errors.Join(fmt.Errorf("docker: start %s: %w", c.Name, err), c.Remove(ctx))
	}
	return c, nil
}

// Run runs a one-shot container to its exit and returns its exit code,
// demultiplexed output and whether it was OOM-killed. The wait is registered
// before the start, so a container that exits within milliseconds is not
// missed. The container is always removed on the way out, also on error or
// when ctx is cancelled, using a fresh short context.
func Run(ctx context.Context, cli *client.Client, spec Spec) (res Result, err error) {
	c, err := Create(ctx, cli, spec)
	if err != nil {
		return Result{}, err
	}
	defer func() { err = errors.Join(err, c.Remove(ctx)) }()

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	wait := cli.ContainerWait(waitCtx, c.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNextExit})
	start := time.Now()
	if _, err := cli.ContainerStart(ctx, c.ID, client.ContainerStartOptions{}); err != nil {
		return Result{}, fmt.Errorf("docker: start %s: %w", c.Name, err)
	}
	select {
	case w := <-wait.Result:
		res.ExitCode = int(w.StatusCode)
		if w.Error != nil && w.Error.Message != "" {
			return res, fmt.Errorf("docker: wait %s: %s", c.Name, w.Error.Message)
		}
	case err := <-wait.Error:
		return Result{}, fmt.Errorf("docker: wait %s: %w", c.Name, err)
	case <-ctx.Done():
		return Result{}, fmt.Errorf("docker: run %s: %w", c.Name, ctx.Err())
	}
	res.Duration = time.Since(start)
	if res.Stdout, res.Stderr, err = c.Logs(ctx); err != nil {
		return res, err
	}
	insp, err := c.Inspect(ctx)
	if err != nil {
		return res, err
	}
	res.OOMKilled = insp.State != nil && insp.State.OOMKilled
	return res, nil
}

// Exec runs cmd in the running container and returns its exit code and
// output. It inherits the container's user and restrictions, and does not
// apply the image entrypoint, so a fixture command is
// Exec(ctx, FixtureBinary, "pids").
//
// Cancelling ctx stops the wait, not the process: the Engine API has no call
// to signal an exec, so Exec closes its stream and returns ctx's error with
// the output read so far (ExitCode -1), and the process keeps running inside
// the container until it exits by itself or the container stops
// (TestSpec_H3_GatedCancelledExecKeepsRunning shows it in the probe's pids).
// To bound a command, make it bound itself (sleep 5) or Stop the container.
func (c *Container) Exec(ctx context.Context, cmd ...string) (Result, error) {
	start := time.Now()
	ex, err := c.cli.ExecCreate(ctx, c.ID, client.ExecCreateOptions{Cmd: cmd, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return Result{ExitCode: -1}, fmt.Errorf("docker: exec %q in %s: %w", cmd, c.Name, err)
	}
	att, err := c.cli.ExecAttach(ctx, ex.ID, client.ExecAttachOptions{})
	if err != nil {
		return Result{ExitCode: -1}, fmt.Errorf("docker: exec %q in %s: %w", cmd, c.Name, err)
	}
	defer att.Close()
	// The hijacked connection ignores ctx once it is open; closing it is the
	// only way to end the read early.
	stop := context.AfterFunc(ctx, att.Close)
	var stdout, stderr bytes.Buffer
	_, copyErr := stdcopy.StdCopy(&stdout, &stderr, att.Reader)
	stop()
	res := Result{ExitCode: -1, Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() != nil {
		res.Duration = time.Since(start)
		return res, fmt.Errorf("docker: exec %q in %s: %w (the process keeps running until the container stops)", cmd, c.Name, ctx.Err())
	}
	if copyErr != nil {
		return res, fmt.Errorf("docker: exec %q in %s: reading output: %w", cmd, c.Name, copyErr)
	}
	// The stream can close a moment before the engine records the exit.
	for {
		insp, err := c.cli.ExecInspect(ctx, ex.ID, client.ExecInspectOptions{})
		if err != nil {
			return res, fmt.Errorf("docker: exec %q in %s: inspect: %w", cmd, c.Name, err)
		}
		if !insp.Running {
			res.ExitCode, res.Duration = insp.ExitCode, time.Since(start)
			return res, nil
		}
		select {
		case <-ctx.Done():
			return res, fmt.Errorf("docker: exec %q in %s: %w", cmd, c.Name, ctx.Err())
		case <-time.After(execPollInterval):
		}
	}
}

// Stop sends SIGTERM and, if the container is still running after grace
// (rounded up to whole seconds, the API's unit; 0 or less kills at once),
// SIGKILL. It returns when the container has stopped.
func (c *Container) Stop(ctx context.Context, grace time.Duration) error {
	secs := max(0, int(math.Ceil(grace.Seconds())))
	if _, err := c.cli.ContainerStop(ctx, c.ID, client.ContainerStopOptions{Timeout: &secs}); err != nil {
		return fmt.Errorf("docker: stop %s: %w", c.Name, err)
	}
	return nil
}

// Wait blocks until the container is not running and returns its exit code.
// Call it after Start; a container that was never started counts as not
// running at once.
func (c *Container) Wait(ctx context.Context) (int, error) {
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	wait := c.cli.ContainerWait(waitCtx, c.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case w := <-wait.Result:
		if w.Error != nil && w.Error.Message != "" {
			return int(w.StatusCode), fmt.Errorf("docker: wait %s: %s", c.Name, w.Error.Message)
		}
		return int(w.StatusCode), nil
	case err := <-wait.Error:
		return 0, fmt.Errorf("docker: wait %s: %w", c.Name, err)
	case <-ctx.Done():
		return 0, fmt.Errorf("docker: wait %s: %w", c.Name, ctx.Err())
	}
}

// Logs returns the container's demultiplexed stdout and stderr so far.
func (c *Container) Logs(ctx context.Context) (stdout, stderr string, err error) {
	logs, err := c.cli.ContainerLogs(ctx, c.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", "", fmt.Errorf("docker: logs %s: %w", c.Name, err)
	}
	defer logs.Close()
	var so, se bytes.Buffer
	if _, err := stdcopy.StdCopy(&so, &se, logs); err != nil {
		return "", "", fmt.Errorf("docker: logs %s: %w", c.Name, err)
	}
	return so.String(), se.String(), nil
}

// Inspect returns what the engine recorded for the container.
func (c *Container) Inspect(ctx context.Context) (container.InspectResponse, error) {
	res, err := c.cli.ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, fmt.Errorf("docker: inspect %s: %w", c.Name, err)
	}
	return res.Container, nil
}

// Remove force-removes the container, running or not, with its anonymous
// volumes. It is cleanup, so it runs on a fresh context bounded by
// cleanupTimeout that keeps ctx's values but not its cancellation: a deferred
// Remove still works after ctx expired. Removing a container that is already
// gone is not an error.
func (c *Container) Remove(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	_, err := c.cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	if err != nil && !containerGone(ctx, c.cli, c.ID) {
		return fmt.Errorf("docker: remove %s: %w", c.Name, err)
	}
	return nil
}
