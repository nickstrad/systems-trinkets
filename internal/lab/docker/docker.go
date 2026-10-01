// Package docker holds the Docker Engine helpers the container lessons
// repeat. It lives beside lab rather than in it so lab stays standard-library
// only and a lesson that never talks to a daemon does not compile the moby
// client.
package docker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"

	"github.com/nickstrad/systems-trinkets/internal/lab"
)

// pingTimeout bounds Connect's Ping, because client.New never contacts the
// daemon and a stopped or wedged one would otherwise hang the lesson. A
// variable so a test can shorten it.
var pingTimeout = 5 * time.Second

// contextLookupTimeout bounds the docker CLI call behind Host.
const contextLookupTimeout = 5 * time.Second

// contextHost returns the active Docker CLI context's endpoint, or "" when it
// cannot tell. A variable so tests cover Host without a docker CLI.
var contextHost = dockerContextHost

// dockerContextHost asks the docker CLI, which is already a repo requirement
// for make up-<service>. Any failure (no CLI, no daemon context, a timeout)
// yields "": Host then falls back to the client default.
func dockerContextHost() string {
	ctx, cancel := context.WithTimeout(context.Background(), contextLookupTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "context", "inspect",
		"--format", "{{.Endpoints.docker.Host}}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Host is the Docker endpoint to use: DOCKER_HOST, else the active Docker
// CLI context's endpoint, else client.DefaultDockerHost. client.FromEnv alone
// ignores the CLI context, which matters on Docker Desktop where the socket is
// not /var/run/docker.sock. A context answer the client cannot parse (such as
// "<no value>") counts as no answer.
func Host() string {
	if h := lab.Env(client.EnvOverrideHost, ""); h != "" {
		return h
	}
	if h := contextHost(); h != "" {
		if _, err := client.ParseHostURL(h); err == nil {
			return h
		}
	}
	return client.DefaultDockerHost
}

// Connect opens a client to Host() and pings the daemon, failing fast on
// error: a stopped daemon panics with the connection error within pingTimeout
// instead of surfacing later as a confusing failure mid-lesson. Set
// DOCKER_HOST to point a lesson at another daemon. The ping also negotiates
// the API version. Close the client when done.
func Connect(ctx context.Context) *client.Client {
	host := Host()
	cli, err := client.New(client.FromEnv, client.WithHost(host))
	lab.Check(err)
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if _, err := cli.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true}); err != nil {
		_ = cli.Close()
		lab.Check(fmt.Errorf("docker: ping %s: %w", host, err))
	}
	return cli
}

// EngineArch is the GOARCH to cross-compile fixtures for: the engine's
// architecture, which on Docker Desktop for Mac is the Linux VM's (arm64 on
// Apple silicon), not the machine running this code. It panics on an
// architecture it does not know.
func EngineArch(ctx context.Context, cli *client.Client) string {
	res, err := cli.Info(ctx, client.InfoOptions{})
	lab.Check(err)
	arch, err := archFromInfo(res.Info)
	lab.Check(err)
	return arch
}

// archFromInfo maps an engine's Info to a GOARCH.
func archFromInfo(info system.Info) (string, error) { return archToGOARCH(info.Architecture) }

// archToGOARCH maps the kernel architecture name Info reports (uname -m) to a
// GOARCH. The match is exact, and anything else is an error: a fixture built
// for a guessed architecture fails later as an opaque "exec format error".
func archToGOARCH(kernel string) (string, error) {
	switch kernel {
	case "x86_64":
		return "amd64", nil
	case "aarch64":
		return "arm64", nil
	}
	return "", fmt.Errorf("docker: unsupported engine architecture %q", kernel)
}
