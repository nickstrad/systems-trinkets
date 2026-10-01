package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/moby/moby/client"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// FixtureNetwork creates a labelled user-defined bridge network,
// trinkets-<lesson>-net-<random>, and returns its name for Spec.Network. It
// is the H6 egress topology's fixture side: fixture services and the broker
// join it, the worker does not (Network "none", the socket volume only).
//
// The network is Internal: containers on it reach each other by IP and by
// container name, and nothing else. On Linux Engine 29.7.2 an internal
// network still gives the host's bridge address (the IPAM gateway) a route,
// so the broker can reach host services that listen on all addresses there;
// it has no default route, so other host addresses and the internet are
// unreachable and names outside the network do not resolve. That leftover
// host route is why the worker gets no network at all.
//
// Sweep removes the network once its containers are gone.
func FixtureNetwork(ctx context.Context, cli *client.Client, lesson string) (string, error) {
	if err := validLesson(lesson); err != nil {
		return "", err
	}
	name := ContainerName(lesson, "net", NameSuffix())
	if _, err := cli.NetworkCreate(ctx, name, client.NetworkCreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels:   Labels(lesson),
	}); err != nil {
		return "", fmt.Errorf("docker: create network %s: %w", name, err)
	}
	return name, nil
}

// CheckWorkerHasNoRoute is worker-has-no-route on a running container from a
// probe image: it runs the probe's interfaces command there and returns a
// violation unless lo is the only interface. An error means the check could
// not run, not that it failed.
func CheckWorkerHasNoRoute(ctx context.Context, worker *Container) ([]string, error) {
	res, err := worker.Exec(ctx, FixtureBinary, "interfaces")
	if err != nil {
		return nil, err
	}
	l, ok := probeout.Find(probeout.Parse(res.Stdout), "interfaces")
	if !ok || !l.OK {
		return nil, fmt.Errorf("docker: interfaces in %s: exit %d, output %q", worker.Name, res.ExitCode, res.Stdout)
	}
	return workerHasNoRoute(strings.Fields(l.Detail)), nil
}

// CheckBlockedFixtureUntouched is blocked-fixture-untouched on a running
// probe http-count fixture listening on port: it returns a violation unless
// the fixture has counted no request. An error means the count could not be
// read.
func CheckBlockedFixtureUntouched(ctx context.Context, blocked *Container, port int) ([]string, error) {
	counts, err := fixtureCounts(ctx, blocked, port)
	if err != nil {
		return nil, err
	}
	return blockedFixtureUntouched(counts.Total), nil
}

// requestCounts is the JSON http-count serves at /__counts.
type requestCounts struct {
	Total int            `json:"total"`
	Paths map[string]int `json:"paths"`
}

// fixtureCounts reads an http-count fixture's counts from inside the fixture
// (the probe's http-get on 127.0.0.1), so it needs no route from the host,
// works the same on Docker Desktop, and is not itself counted.
func fixtureCounts(ctx context.Context, fixture *Container, port int) (requestCounts, error) {
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/__counts"
	res, err := fixture.Exec(ctx, FixtureBinary, "http-get", url)
	if err != nil {
		return requestCounts{}, err
	}
	l, ok := probeout.Find(probeout.Parse(res.Stdout), "http-get")
	body, isOK := strings.CutPrefix(l.Detail, "200 ")
	var c requestCounts
	if !ok || !l.OK || !isOK || json.Unmarshal([]byte(body), &c) != nil {
		return requestCounts{}, fmt.Errorf("docker: counts of %s: exit %d, output %q", fixture.Name, res.ExitCode, res.Stdout)
	}
	return c, nil
}
