package docker

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"pgregory.net/rapid"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// Gated H6 tests: the Unix-socket egress topology. Two http-count fixtures
// ("allowed" and "blocked") and a broker share an internal FixtureNetwork;
// the broker runs the probe's forward (one fixed destination: allowed) on a
// socket in the SocketVolume; the worker has Network "none" and the worker
// mount only. Each test uses its own lesson label h6-<test>-<random>, and
// egressLesson sweeps it when the test ends, then asserts no-leak (the
// network and the volume have no handle that would remove them).
//
// These tests need SocketVolume, which H5 implements; until it lands they
// fail at that call.

const (
	egressBrokerUID  = 20000
	egressSocketGID  = 30000
	egressWorkerUser = "20001:20001"
	fixturePort      = 8080
	// proxyPort is where the worker's HTTP_PROXY points, on the broker's
	// network address. Nothing listens there; the worker cannot reach the
	// address anyway.
	proxyPort = 3128
)

// egressSocket is the forwarder's socket, inside the shared volume.
const egressSocket = SocketDir + "/egress.sock"

type egressTopology struct {
	network, volume                         string
	gateway, allowedIP, blockedIP, brokerIP netip.Addr
	allowed, blocked, broker, worker        *Container
}

// egressLesson returns a fresh lesson label h6-<name>-<random>. When the test
// ends it sweeps the label and asserts no-leak.
func egressLesson(t *testing.T, cli *client.Client, name string) string {
	t.Helper()
	lesson := "h6-" + name + "-" + randomName(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := Sweep(ctx, cli, lesson); err != nil {
			t.Errorf("sweep: %v", err)
		}
		if bad, err := CheckNoLeak(ctx, cli, lesson); err != nil {
			t.Errorf("no-leak check: %v", err)
		} else if len(bad) > 0 {
			t.Errorf("leaked after sweep: %v", bad)
		}
	})
	return lesson
}

// ipOn is a container's address on the topology's network.
func (tp *egressTopology) ipOn(t *testing.T, ctx context.Context, c *Container) netip.Addr {
	t.Helper()
	insp, err := c.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if insp.NetworkSettings == nil || insp.NetworkSettings.Networks[tp.network] == nil {
		t.Fatalf("%s is not on %s", c.Name, tp.network)
	}
	ip := insp.NetworkSettings.Networks[tp.network].IPAddress
	if !ip.IsValid() {
		t.Fatalf("%s has no address on %s", c.Name, tp.network)
	}
	return ip
}

// startEgress builds the whole topology and waits until every server
// listens.
func startEgress(t *testing.T, ctx context.Context, cli *client.Client, lesson string) *egressTopology {
	t.Helper()
	img := probeImage(t, ctx, cli)
	tp := &egressTopology{}
	var err error
	if tp.network, err = FixtureNetwork(ctx, cli, lesson); err != nil {
		t.Fatal(err)
	}
	ni, err := cli.NetworkInspect(ctx, tp.network, client.NetworkInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !ni.Network.Internal || ni.Network.Driver != "bridge" || ni.Network.Labels[LabelLesson] != lesson || len(ni.Network.IPAM.Config) == 0 {
		t.Fatalf("network %s: internal=%v driver=%q labels=%v ipam=%+v", tp.network,
			ni.Network.Internal, ni.Network.Driver, ni.Network.Labels, ni.Network.IPAM.Config)
	}
	// An internal network records no gateway on its endpoints; the address
	// the host's bridge holds is the IPAM gateway.
	tp.gateway = ni.Network.IPAM.Config[0].Gateway

	start := func(s Spec, ready string) *Container {
		c, err := Start(ctx, cli, s)
		if err != nil {
			t.Fatal(err)
		}
		waitForLog(t, ctx, c, ready)
		return c
	}
	fixture := func(role string) *Container {
		s := Restricted(lesson, role, img, "http-count", "--name", role, ":"+strconv.Itoa(fixturePort))
		s.Network = tp.network
		return start(s, "listen: OK")
	}
	tp.allowed, tp.blocked = fixture("allowed"), fixture("blocked")

	brokerMount, workerMount, err := SocketVolume(ctx, cli, lesson, egressBrokerUID, egressSocketGID)
	if err != nil {
		t.Fatalf("SocketVolume (H5) failed; the H6 socket tests need H5 merged: %v", err)
	}
	tp.volume = brokerMount.Source

	b := Restricted(lesson, "broker", img, "forward", egressSocket, net.JoinHostPort(tp.allowed.Name, strconv.Itoa(fixturePort)))
	b.User = fmt.Sprintf("%d:%d", egressBrokerUID, egressSocketGID)
	b.Network = tp.network
	b.Mounts = append(b.Mounts, brokerMount)
	tp.broker = start(b, "listen: OK")

	tp.allowedIP, tp.blockedIP, tp.brokerIP = tp.ipOn(t, ctx, tp.allowed), tp.ipOn(t, ctx, tp.blocked), tp.ipOn(t, ctx, tp.broker)

	w := Restricted(lesson, "worker", img, "sleep")
	w.User = egressWorkerUser
	w.Groups = []string{strconv.Itoa(egressSocketGID)}
	w.Mounts = append(w.Mounts, workerMount)
	proxy := "http://" + netip.AddrPortFrom(tp.brokerIP, proxyPort).String()
	w.Env = []string{"HTTP_PROXY=" + proxy, "http_proxy=" + proxy}
	tp.worker = start(w, "sleep: OK")
	t.Logf("topology: network %s gateway %s, allowed %s %s, blocked %s %s, broker %s %s, volume %s",
		tp.network, tp.gateway, tp.allowed.Name, tp.allowedIP, tp.blocked.Name, tp.blockedIP, tp.broker.Name, tp.brokerIP, tp.volume)
	return tp
}

func httpRequest(path string) string { return "GET " + path + " HTTP/1.0\r\n\r\n" }

// probeStep is one probe run in a container: cmd is the probe's arguments,
// line the name of the line that says whether the connection was made.
type probeStep struct {
	name string
	cmd  []string
	line string
	// why, when set: a denial must give one of these reasons, so a probe
	// that fails for the wrong reason (no listener on a reachable address)
	// does not pass as "no route".
	why []string
}

func dialStep(name, network, addr string, msg string, why ...string) probeStep {
	cmd := []string{"dial", network, addr}
	if msg != "" {
		cmd = append(cmd, msg)
	}
	return probeStep{name: name, cmd: cmd, line: "dial", why: why}
}

func httpGetStep(name, url string, why ...string) probeStep {
	return probeStep{name: name, cmd: []string{"http-get", url}, line: "http-get", why: why}
}

// run executes the step in c and returns its connection line.
func (s probeStep) run(ctx context.Context, c *Container) (probeout.Line, Result, error) {
	res, err := c.Exec(ctx, append([]string{FixtureBinary}, s.cmd...)...)
	if err != nil {
		return probeout.Line{}, res, err
	}
	l, ok := probeout.Find(probeout.Parse(res.Stdout), s.line)
	if !ok {
		return probeout.Line{}, res, fmt.Errorf("%s: no %s line (exit %d, stdout %q, stderr %q)", s.name, s.line, res.ExitCode, res.Stdout, res.Stderr)
	}
	return l, res, nil
}

// denied runs the step in c and returns an error unless the connection
// itself was refused (exit 1, the connection line DENIED, for one of the
// expected reasons).
func (s probeStep) denied(ctx context.Context, c *Container) (string, error) {
	l, res, err := s.run(ctx, c)
	if err != nil {
		return "", err
	}
	if l.OK || res.ExitCode != 1 {
		return "", fmt.Errorf("bypass %s %q reached something: exit %d, %q", s.name, s.cmd, res.ExitCode, res.Stdout)
	}
	if len(s.why) > 0 && !slices.ContainsFunc(s.why, func(w string) bool { return strings.Contains(l.Detail, w) }) {
		return "", fmt.Errorf("bypass %s failed for an unexpected reason %q, want one of %q", s.name, l.Detail, s.why)
	}
	return l.Detail, nil
}

const (
	noRoute  = "network is unreachable"
	noLookup = "lookup "
)

// viaProxy is the one reason a proxied request may fail with: the client
// tried the broker's address and found no route. Nothing listens on
// proxyPort, so a worker with a route would still fail, with "connection
// refused"; a looser match would pass it.
func (tp *egressTopology) viaProxy() string {
	return "proxyconnect tcp: dial tcp " + netip.AddrPortFrom(tp.brokerIP, proxyPort).String() + ": connect: " + noRoute
}

// bypassProbes is every path the plan names, from the worker: the two
// fixtures' addresses, alternate ports on them, the gateway,
// host.docker.internal, DNS lookups of fixture and outside names, the broker's
// network address, and HTTP_PROXY (set in the worker's environment) pointing
// at it. Each carries a real HTTP request, so one that got through would
// show up in a fixture's count.
func (tp *egressTopology) bypassProbes() []probeStep {
	hp := func(a netip.Addr, port int) string { return netip.AddrPortFrom(a, uint16(port)).String() }
	ps := []probeStep{
		dialStep("blocked-ip", "tcp", hp(tp.blockedIP, fixturePort), httpRequest("/bypass/blocked-ip"), noRoute),
		dialStep("allowed-ip", "tcp", hp(tp.allowedIP, fixturePort), httpRequest("/bypass/allowed-ip"), noRoute),
		dialStep("gateway-ssh", "tcp", hp(tp.gateway, 22), "", noRoute),
		dialStep("gateway-fixture-port", "tcp", hp(tp.gateway, fixturePort), httpRequest("/bypass/gateway"), noRoute),
		// Linux: the name does not resolve. Docker Desktop resolves it, and
		// then the dial must find no route.
		dialStep("host.docker.internal", "tcp", net.JoinHostPort("host.docker.internal", strconv.Itoa(fixturePort)), httpRequest("/bypass/hdi"), noLookup, noRoute),
		dialStep("dns-allowed-name", "tcp", net.JoinHostPort(tp.allowed.Name, strconv.Itoa(fixturePort)), httpRequest("/bypass/allowed-name"), noLookup),
		dialStep("dns-blocked-name", "tcp", net.JoinHostPort(tp.blocked.Name, strconv.Itoa(fixturePort)), httpRequest("/bypass/blocked-name"), noLookup),
		dialStep("dns-outside-name", "tcp", "example.com:80", httpRequest("/bypass/outside"), noLookup),
		dialStep("broker-ip", "tcp", hp(tp.brokerIP, proxyPort), httpRequest("/bypass/broker"), noRoute),
		httpGetStep("http-proxy-blocked", "http://"+hp(tp.blockedIP, fixturePort)+"/bypass/proxy", tp.viaProxy()),
		httpGetStep("http-proxy-allowed-name", "http://"+net.JoinHostPort(tp.allowed.Name, strconv.Itoa(fixturePort))+"/bypass/proxy", tp.viaProxy()),
	}
	for _, port := range []int{80, 443, 53, 2375} {
		for _, f := range []struct {
			name string
			ip   netip.Addr
		}{{"allowed", tp.allowedIP}, {"blocked", tp.blockedIP}} {
			ps = append(ps, dialStep(fmt.Sprintf("%s-port-%d", f.name, port), "tcp", hp(f.ip, port), httpRequest("/bypass/port"), noRoute))
		}
	}
	return ps
}

// viaSocket is the worker's request through the broker.
func viaSocket(path string) probeStep {
	return dialStep("via-socket", "unix", egressSocket, httpRequest(path))
}

// counts reads an http-count fixture's counts.
func counts(t *testing.T, ctx context.Context, c *Container) requestCounts {
	t.Helper()
	got, err := fixtureCounts(ctx, c, fixturePort)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mustBlockedUntouched(t *testing.T, ctx context.Context, tp *egressTopology) {
	t.Helper()
	bad, err := CheckBlockedFixtureUntouched(ctx, tp.blocked, fixturePort)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range bad {
		t.Error(v)
	}
}

func TestSpec_H6_GatedBypassProbesFailAndCountsStayZero(t *testing.T) {
	ctx, cli := gatedClient(t)
	tp := startEgress(t, ctx, cli, egressLesson(t, cli, "bypass"))

	// Controls from the broker, with no request sent so nothing is counted:
	// the targets exist and are reachable on the network, so the worker's
	// failures below are about the worker.
	for _, s := range []probeStep{
		dialStep("blocked-ip", "tcp", netip.AddrPortFrom(tp.blockedIP, fixturePort).String(), ""),
		dialStep("allowed-ip", "tcp", netip.AddrPortFrom(tp.allowedIP, fixturePort).String(), ""),
		dialStep("blocked-name", "tcp", net.JoinHostPort(tp.blocked.Name, strconv.Itoa(fixturePort)), ""),
		dialStep("allowed-name", "tcp", net.JoinHostPort(tp.allowed.Name, strconv.Itoa(fixturePort)), ""),
	} {
		if l, res, err := s.run(ctx, tp.broker); err != nil || !l.OK {
			t.Errorf("control from the broker: %s %q: %v %q", s.name, s.cmd, err, res.Stdout)
		}
	}
	// What the internal network still lets the broker reach, recorded, not
	// asserted: it differs by host (Linux has sshd on all addresses here).
	for _, s := range []probeStep{
		dialStep("gateway-ssh", "tcp", netip.AddrPortFrom(tp.gateway, 22).String(), ""),
		dialStep("host.docker.internal", "tcp", "host.docker.internal:22", ""),
		dialStep("outside-ip", "tcp", "1.1.1.1:443", ""),
		dialStep("outside-name", "tcp", "example.com:80", ""),
	} {
		l, _, err := s.run(ctx, tp.broker)
		t.Logf("broker on the internal network: %-20s ok=%v %s %v", s.name, l.OK, l.Detail, err)
	}

	for _, s := range tp.bypassProbes() {
		why, err := s.denied(ctx, tp.worker)
		if err != nil {
			t.Error(err)
			continue
		}
		t.Logf("worker bypass %-24s denied: %s", s.name, why)
	}
	mustBlockedUntouched(t, ctx, tp)
	if c := counts(t, ctx, tp.allowed); c.Total != 0 {
		t.Errorf("allowed fixture counted %d requests from bypass probes: %v", c.Total, c.Paths)
	}
}

func TestSpec_H6_GatedSocketRequestReachesAllowedOnce(t *testing.T) {
	ctx, cli := gatedClient(t)
	tp := startEgress(t, ctx, cli, egressLesson(t, cli, "socket"))

	l, res, err := viaSocket("/via-socket").run(ctx, tp.worker)
	if err != nil || !l.OK || res.ExitCode != 0 {
		t.Fatalf("request through the socket: %v exit %d %q", err, res.ExitCode, res.Stdout)
	}
	if reply, _ := probeout.Find(probeout.Parse(res.Stdout), "reply"); reply.Detail != "HTTP/1.0 200 OK" {
		t.Errorf("reply %q, want the allowed fixture's 200", reply.Detail)
	}
	// Exactly once: one request in total, on the path the worker sent.
	if c := counts(t, ctx, tp.allowed); c.Total != 1 || c.Paths["/via-socket"] != 1 || len(c.Paths) != 1 {
		t.Errorf("allowed counts %+v, want exactly one request to /via-socket", c)
	}
	mustBlockedUntouched(t, ctx, tp)
	stdout, _, err := tp.broker.Logs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(stdout, "forward: OK "); n != 1 {
		t.Errorf("broker logged %d forwards, want 1:\n%s", n, stdout)
	}
}

func TestSpec_H6_GatedWorkerInterfacesAreLoOnly(t *testing.T) {
	ctx, cli := gatedClient(t)
	tp := startEgress(t, ctx, cli, egressLesson(t, cli, "ifaces"))

	bad, err := CheckWorkerHasNoRoute(ctx, tp.worker)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range bad {
		t.Error(v)
	}
	// The same check on the broker must fail: it is on the network, so the
	// check can tell the two apart.
	if bad, err := CheckWorkerHasNoRoute(ctx, tp.broker); err != nil || len(bad) == 0 {
		t.Errorf("worker-has-no-route held for the broker (%v): the check cannot see a network", err)
	} else {
		t.Logf("broker, as expected: %v", bad)
	}
}

func TestSpec_H6_GatedStoppedBrokerLeavesNoFallback(t *testing.T) {
	ctx, cli := gatedClient(t)
	tp := startEgress(t, ctx, cli, egressLesson(t, cli, "stopped"))

	// The path works while the broker runs.
	if l, res, err := viaSocket("/before-stop").run(ctx, tp.worker); err != nil || !l.OK {
		t.Fatalf("request before the stop: %v %q", err, res.Stdout)
	}
	if err := tp.broker.Stop(ctx, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	// forward closes its listener on SIGTERM, which removes the socket file;
	// a killed broker would leave a file that refuses connections.
	why, err := viaSocket("/after-stop").denied(ctx, tp.worker)
	if err != nil {
		t.Fatalf("request after the broker stopped: %v", err)
	}
	t.Logf("through the socket after stop: %s", why)
	// No other way to the allowed fixture opens up either.
	for _, s := range tp.bypassProbes() {
		if why, err := s.denied(ctx, tp.worker); err != nil {
			t.Error(err)
		} else {
			t.Logf("after stop, %-24s denied: %s", s.name, why)
		}
	}
	if c := counts(t, ctx, tp.allowed); c.Total != 1 || c.Paths["/before-stop"] != 1 {
		t.Errorf("allowed counts %+v, want only the one request from before the stop", c)
	}
	mustBlockedUntouched(t, ctx, tp)
}

func TestSpec_H6_GatedSweepRemovesNetworkVolumeAndContainers(t *testing.T) {
	ctx, cli := gatedClient(t)
	lesson := egressLesson(t, cli, "sweep")
	tp := startEgress(t, ctx, cli, lesson)

	// No handle is removed: Sweep alone must take everything.
	removed, err := Sweep(ctx, cli, lesson)
	if err != nil {
		t.Fatal(err)
	}
	wantContainers := []string{tp.allowed.Name, tp.blocked.Name, tp.broker.Name, tp.worker.Name}
	slices.Sort(wantContainers)
	slices.Sort(removed.Containers)
	if !slices.Equal(removed.Containers, wantContainers) {
		t.Errorf("Sweep removed containers %q, want %q", removed.Containers, wantContainers)
	}
	if !slices.Equal(removed.Volumes, []string{tp.volume}) {
		t.Errorf("Sweep removed volumes %q, want [%s]", removed.Volumes, tp.volume)
	}
	if !slices.Equal(removed.Networks, []string{tp.network}) {
		t.Errorf("Sweep removed networks %q, want [%s]", removed.Networks, tp.network)
	}
	mustLeftNothing(t, ctx, cli, lesson, "Sweep")
}

// egressStep draws one worker step: a bypass attempt (a raw dial carrying an
// HTTP request, or http-get through HTTP_PROXY) at any topology address or
// name on a well-known or random port, or a request through the socket.
func egressStep(tp *egressTopology) *rapid.Generator[probeStep] {
	targets := []string{
		tp.blockedIP.String(), tp.allowedIP.String(), tp.gateway.String(), tp.brokerIP.String(),
		tp.blocked.Name, tp.allowed.Name, tp.broker.Name, "host.docker.internal", "example.com",
	}
	return rapid.Custom(func(t *rapid.T) probeStep {
		path := "/" + rapid.StringMatching(`[a-z0-9]{1,8}`).Draw(t, "path")
		kind := rapid.SampledFrom([]string{"dial", "http-get", "socket"}).Draw(t, "kind")
		if kind == "socket" {
			return viaSocket(path)
		}
		target := rapid.SampledFrom(targets).Draw(t, "target")
		port := rapid.OneOf(rapid.SampledFrom([]int{fixturePort, 80, 443, 53, 22, 2375, proxyPort}), rapid.IntRange(1, 65535)).Draw(t, "port")
		hostport := net.JoinHostPort(target, strconv.Itoa(port))
		if kind == "dial" {
			// An address finds no route; a name fails to resolve (or, for
			// host.docker.internal on Docker Desktop, resolves and then
			// finds no route).
			return dialStep("dial", "tcp", hostport, httpRequest(path), noRoute, noLookup)
		}
		return httpGetStep("http-get", "http://"+hostport+path, tp.viaProxy())
	})
}

// TestProp_EgressBypass: for random sequences of worker steps, every bypass
// attempt fails, every socket request reaches the allowed fixture once, and
// blocked-fixture-untouched holds. One topology serves all cases; it is
// capped at 5 cases of up to 8 steps unless -rapid.checks or RAPID_CHECKS
// says otherwise.
func TestProp_EgressBypass(t *testing.T) {
	ctx, cli := gatedClient(t)
	capRapid(t, 5, 8)
	tp := startEgress(t, ctx, cli, egressLesson(t, cli, "prop"))
	step := egressStep(tp)
	sent := 0 // requests through the socket, over all cases
	rapid.Check(t, func(rt *rapid.T) {
		for _, s := range rapid.SliceOfN(step, 1, 8).Draw(rt, "steps") {
			if s.name == "via-socket" {
				if l, res, err := s.run(ctx, tp.worker); err != nil || !l.OK {
					rt.Fatalf("request through the socket: %v %q", err, res.Stdout)
				}
				sent++
				continue
			}
			if _, err := s.denied(ctx, tp.worker); err != nil {
				rt.Fatal(err)
			}
		}
		bad, err := CheckBlockedFixtureUntouched(ctx, tp.blocked, fixturePort)
		if err != nil {
			rt.Fatal(err)
		}
		if len(bad) > 0 {
			rt.Fatal(strings.Join(bad, "; "))
		}
		got, err := fixtureCounts(ctx, tp.allowed, fixturePort)
		if err != nil {
			rt.Fatal(err)
		}
		if got.Total != sent {
			rt.Fatalf("allowed fixture counted %d requests, the worker sent %d through the socket", got.Total, sent)
		}
	})
}
