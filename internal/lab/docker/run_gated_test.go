package docker

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"pgregory.net/rapid"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// Gated launcher tests (TRINKETS_DOCKER=1, make check-docker). Each test
// uses its own lesson label h3-<test>-<random>; testLesson checks no-leak
// for it when the test ends and then sweeps it. The probe image is built
// once per package run and removed by TestMain.

var sharedProbe struct {
	once sync.Once
	tag  string
	err  error
}

// probeImage builds the probe fixture (with /work owned by 10001) the first
// time a gated test asks for it.
func probeImage(t *testing.T, ctx context.Context, cli *client.Client) string {
	t.Helper()
	sharedProbe.once.Do(func() {
		lesson := "h3-pkg-" + randomName(t)
		img := FixtureImage{Lesson: lesson, Tag: "trinkets-" + lesson + "-probe:dev", Package: "./probe",
			Dirs: []OwnedDir{{"/work", 10001, 10001}}}
		sharedProbe.tag = img.Tag // set first, so TestMain removes a half-built image too
		start := time.Now()
		sharedProbe.err = BuildFixture(ctx, cli, img)
		t.Logf("built shared %s in %v", img.Tag, time.Since(start).Round(10*time.Millisecond))
	})
	if sharedProbe.err != nil {
		t.Fatalf("BuildFixture: %v", sharedProbe.err)
	}
	return sharedProbe.tag
}

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedProbe.tag != "" {
		if err := removeSharedProbe(sharedProbe.tag); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = max(code, 1)
		}
	}
	os.Exit(code)
}

// removeSharedProbe removes the package's probe image, and only if it carries
// the harness label and an h3- lesson.
func removeSharedProbe(tag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cli := Connect(ctx)
	defer cli.Close()
	res, err := cli.ImageInspect(ctx, tag)
	if err != nil {
		return nil // never built
	}
	if res.Config == nil || res.Config.Labels[LabelHarness] != "1" || !strings.HasPrefix(res.Config.Labels[LabelLesson], "h3-") {
		return fmt.Errorf("image %s lacks the harness and h3- lesson labels; leaving it alone", tag)
	}
	if _, err := cli.ImageRemove(ctx, tag, client.ImageRemoveOptions{Force: true, PruneChildren: true}); err != nil {
		return fmt.Errorf("remove image %s: %w", tag, err)
	}
	return nil
}

// testLesson returns a fresh lesson label for one test. When the test ends
// it asserts no-leak (everything the test created was removed by its own
// handles), then sweeps the label as the net.
func testLesson(t *testing.T, cli *client.Client, name string) string {
	t.Helper()
	lesson := "h3-" + name + "-" + randomName(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if bad, err := CheckNoLeak(ctx, cli, lesson); err != nil {
			t.Errorf("no-leak check: %v", err)
		} else if len(bad) > 0 {
			t.Errorf("leaked: %v", bad)
		}
		if _, err := Sweep(ctx, cli, lesson); err != nil {
			t.Errorf("sweep: %v", err)
		}
	})
	return lesson
}

// removeOnCleanup removes a handle when the test ends, before testLesson's
// no-leak check (cleanups run last-registered first).
func removeOnCleanup(t *testing.T, c *Container) {
	t.Cleanup(func() {
		if err := c.Remove(context.Background()); err != nil {
			t.Errorf("remove %s: %v", c.Name, err)
		}
	})
}

// waitForLog polls a container's stdout until it contains want.
func waitForLog(t *testing.T, ctx context.Context, c *Container, want string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		stdout, _, err := c.Logs(ctx)
		if err == nil && strings.Contains(stdout, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never printed %q (stdout %q, err %v)", c.Name, want, stdout, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func mustLeftNothing(t *testing.T, ctx context.Context, cli *client.Client, lesson, after string) {
	t.Helper()
	left, err := ListLesson(ctx, cli, lesson)
	if err != nil {
		t.Fatal(err)
	}
	if !left.Empty() {
		t.Errorf("after %s: %+v still carries %s", after, left, lesson)
	}
}

func TestSpec_H3_GatedRunOneShotCreatesStartsWaitsLogsAndRemoves(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "oneshot")

	res, err := Run(ctx, cli, Restricted(lesson, "id", img, "id"))
	if err != nil {
		t.Fatal(err)
	}
	l, ok := probeout.Find(probeout.Parse(res.Stdout), "identity")
	if res.ExitCode != 0 || !ok || !strings.HasPrefix(l.Detail, "uid=10001 gid=10001 ") || res.Stderr != "" {
		t.Errorf("id: %+v", res)
	}
	if res.Duration <= 0 || res.OOMKilled {
		t.Errorf("duration %v, OOMKilled %v", res.Duration, res.OOMKilled)
	}
	t.Logf("one-shot id took %v", res.Duration)
	mustLeftNothing(t, ctx, cli, lesson, "a successful Run")

	// stdout and stderr arrive separately: a usage error writes only stderr
	res, err = Run(ctx, cli, Restricted(lesson, "usage", img, "alloc"))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 2 || res.Stdout != "" || !strings.Contains(res.Stderr, "usage: probe alloc") {
		t.Errorf("usage error: %+v", res)
	}
	// a DENIED line exits 1: the read-only root refuses a write
	res, err = Run(ctx, cli, Restricted(lesson, "write", img, "write", "/"))
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := probeout.Find(probeout.Parse(res.Stdout), "write"); res.ExitCode != 1 || !ok || !strings.Contains(l.Detail, "read-only file system") {
		t.Errorf("write / on a read-only root: %+v", res)
	}
	mustLeftNothing(t, ctx, cli, lesson, "three Runs")
}

func TestSpec_H3_GatedRunRemovesContainerOnCancel(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "cancel")
	short, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := Run(short, cli, Restricted(lesson, "sleeper", img, "sleep"))
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("Run of an endless sleep under a 2 s deadline: %v", err)
	}
	mustLeftNothing(t, ctx, cli, lesson, "a cancelled Run")
}

func TestSpec_H3_GatedExecExitCodes0And2(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "exec")
	c, err := Start(ctx, cli, Restricted(lesson, "worker", img, "sleep"))
	if err != nil {
		t.Fatal(err)
	}
	removeOnCleanup(t, c)

	res, err := c.Exec(ctx, FixtureBinary, "id")
	if err != nil {
		t.Fatal(err)
	}
	l, ok := probeout.Find(probeout.Parse(res.Stdout), "identity")
	if res.ExitCode != 0 || !ok || !strings.HasPrefix(l.Detail, "uid=10001 gid=10001 ") {
		t.Errorf("exec id: %+v (exec inherits the container's user)", res)
	}
	t.Logf("exec round trip %v", res.Duration)

	res, err = c.Exec(ctx, FixtureBinary, "alloc")
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 2 || !strings.Contains(res.Stderr, "usage: probe alloc") {
		t.Errorf("exec usage error: %+v", res)
	}
}

func TestSpec_H3_GatedStopWithinGraceOrKillAfterIt(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "stop")
	stop := func(grace time.Duration, cmd ...string) (code int, took time.Duration) {
		t.Helper()
		c, err := Start(ctx, cli, Restricted(lesson, "sleeper", img, cmd...))
		if err != nil {
			t.Fatal(err)
		}
		removeOnCleanup(t, c)
		waitForLog(t, ctx, c, "sleep: OK pid=") // the SIGTERM handler is installed
		start := time.Now()
		if err := c.Stop(ctx, grace); err != nil {
			t.Fatal(err)
		}
		took = time.Since(start)
		if code, err = c.Wait(ctx); err != nil {
			t.Fatal(err)
		}
		return code, took
	}
	// A handled SIGTERM ends it with exit 0 long before a 10 s grace. The
	// bound is loose because other items share the daemon.
	if code, took := stop(10*time.Second, "sleep"); code != 0 || took >= 10*time.Second {
		t.Errorf("sleep handling SIGTERM: exit %d after %v, want 0 inside the 10 s grace", code, took)
	} else {
		t.Logf("handled SIGTERM: stopped in %v", took)
	}
	// An ignored SIGTERM lasts the whole grace, then SIGKILL: exit 137.
	if code, took := stop(2*time.Second, "sleep", "--ignore-term"); code != 137 || took < 2*time.Second || took > 30*time.Second {
		t.Errorf("sleep --ignore-term: exit %d after %v, want 137 after the 2 s grace", code, took)
	} else {
		t.Logf("ignored SIGTERM: killed after %v", took)
	}
}

func TestSpec_H3_GatedUnknownRuntimeFailsAtCreate(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "runtime")
	s := Restricted(lesson, "r", img, "id")
	s.Runtime = "no-such-runtime"
	c, err := Create(ctx, cli, s)
	if err == nil {
		removeOnCleanup(t, c)
		t.Fatal("an unknown runtime was accepted at create")
	}
	if !strings.Contains(err.Error(), "unknown or invalid runtime name: no-such-runtime") {
		t.Errorf("error %q lacks the engine's message", err)
	}
	t.Log(err)
	mustLeftNothing(t, ctx, cli, lesson, "a refused create")
}

// TestSpec_H3_GatedHandlesLeaveNothingBehind walks every way out of the
// launcher (removed handle, finished Run, refused create, failed start, a
// config the harness refuses) and checks the lesson label is clean after
// each, before any Sweep.
func TestSpec_H3_GatedHandlesLeaveNothingBehind(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "noleak")

	c, err := Start(ctx, cli, Restricted(lesson, "a", img, "sleep"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx); err != nil {
		t.Errorf("a second Remove should be a no-op: %v", err)
	}
	mustLeftNothing(t, ctx, cli, lesson, "Start and Remove")

	if _, err := Run(ctx, cli, Spec{Lesson: lesson, Role: "b", Image: img, Cmd: []string{"id"}}); err != nil {
		t.Fatal(err)
	}
	mustLeftNothing(t, ctx, cli, lesson, "Run of a default spec")

	if _, err := Create(ctx, cli, Spec{Lesson: lesson, Role: "c", Image: "trinkets-h3-no-such-image:dev"}); err == nil {
		t.Error("create from a missing image succeeded")
	}
	mustLeftNothing(t, ctx, cli, lesson, "create from a missing image")

	// A PID mode naming a missing container is refused at create.
	if c, err := Start(ctx, cli, Spec{Lesson: lesson, Role: "d", Image: img, PIDMode: "container:trinkets-h3-missing"}); err == nil {
		removeOnCleanup(t, c)
		t.Error("joining the PID namespace of a missing container succeeded")
	} else {
		t.Logf("expected failure: %v", err)
	}
	mustLeftNothing(t, ctx, cli, lesson, "a missing PID namespace")

	// A seccomp profile the engine cannot use passes create and fails at
	// start; Start and Run must remove what they created.
	bogus := Spec{Lesson: lesson, Role: "f", Image: img, Cmd: []string{"id"}, Seccomp: `{"defaultAction":"SCMP_ACT_BOGUS"}`}
	if c, err := Start(ctx, cli, bogus); err == nil {
		removeOnCleanup(t, c)
		t.Error("an unusable seccomp profile started")
	} else if !strings.Contains(err.Error(), "docker: start ") {
		t.Errorf("want a start failure, got %v", err)
	} else {
		t.Logf("expected failure: %v", err)
	}
	mustLeftNothing(t, ctx, cli, lesson, "a failed Start")
	if _, err := Run(ctx, cli, bogus); err == nil || !strings.Contains(err.Error(), "docker: start ") {
		t.Errorf("Run with an unusable seccomp profile: %v, want a start failure", err)
	}
	mustLeftNothing(t, ctx, cli, lesson, "a failed Run")

	s := Spec{Lesson: lesson, Role: "e", Image: img, Mounts: []Mount{{Type: MountVolume, Source: "/var/run/docker.sock", Target: "/s"}}}
	if _, err := Create(ctx, cli, s); err == nil {
		t.Error("a socket mount was created")
	}
	mustLeftNothing(t, ctx, cli, lesson, "a refused socket mount")
}

func TestSpec_H3_GatedSweepCleansLeakedObjects(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := "h3-leak-" + randomName(t)
	sibling := testLesson(t, cli, "sibling")
	t.Cleanup(func() { _, _ = Sweep(context.Background(), cli, lesson) }) // only if the test fails early

	// Leak a running container, a created one, a volume (mounted, so the
	// container holds it) and a network, all with the lesson label.
	vol, err := CreateVolume(ctx, cli, lesson, "work")
	if err != nil {
		t.Fatal(err)
	}
	s := Restricted(lesson, "leaked", img, "sleep")
	s.Mounts = append(s.Mounts, Mount{Type: MountVolume, Source: vol, Target: "/work"})
	running, err := Start(ctx, cli, s)
	if err != nil {
		t.Fatal(err)
	}
	created, err := Create(ctx, cli, Restricted(lesson, "created", img, "id"))
	if err != nil {
		t.Fatal(err)
	}
	netName := ContainerName(lesson, "net", NameSuffix())
	if _, err := cli.NetworkCreate(ctx, netName, client.NetworkCreateOptions{Labels: Labels(lesson)}); err != nil {
		t.Fatal(err)
	}
	// A sibling lesson's container must survive the sweep.
	keep, err := Start(ctx, cli, Restricted(sibling, "keep", img, "sleep"))
	if err != nil {
		t.Fatal(err)
	}
	removeOnCleanup(t, keep)

	before, err := CheckNoLeak(ctx, cli, lesson)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 4 {
		t.Errorf("before Sweep, no-leak should report 4 objects: %q", before)
	}
	removed, err := Sweep(ctx, cli, lesson)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(removed.Containers)
	wantContainers := []string{running.Name, created.Name}
	slices.Sort(wantContainers)
	if !slices.Equal(removed.Containers, wantContainers) || !slices.Equal(removed.Volumes, []string{vol}) || !slices.Equal(removed.Networks, []string{netName}) {
		t.Errorf("Sweep removed %+v, want containers %v, volume %s, network %s", removed, wantContainers, vol, netName)
	}
	if bad, err := CheckNoLeak(ctx, cli, lesson); err != nil || len(bad) > 0 {
		t.Errorf("after Sweep: %v %v", bad, err)
	}
	if _, err := keep.Inspect(ctx); err != nil {
		t.Errorf("Sweep of %s touched the sibling lesson's container: %v", lesson, err)
	}
	if _, err := Sweep(ctx, cli, ""); err == nil {
		t.Error("Sweep with an empty lesson must refuse, not match everything")
	}
}

// TestSpec_H3_GatedCancelledExecKeepsRunning shows what cancelling an Exec
// does: Exec returns at once with ctx's error, and the process keeps running
// in the container (the Engine API cannot signal an exec) until the
// container stops.
func TestSpec_H3_GatedCancelledExecKeepsRunning(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "cancelexec")
	c, err := Start(ctx, cli, Restricted(lesson, "worker", img, "sleep"))
	if err != nil {
		t.Fatal(err)
	}
	removeOnCleanup(t, c)

	short, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	res, err := c.Exec(short, FixtureBinary, "sleep", "600")
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) || res.ExitCode != -1 {
		t.Fatalf("cancelled exec: %+v %v", res, err)
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Errorf("Exec returned %v after its 2 s deadline", took)
	}
	// "sleep: OK pid=<n> ignore-term=false", printed before the deadline
	var pid string
	if l, ok := probeout.Find(probeout.Parse(res.Stdout), "sleep"); ok {
		if f := strings.Fields(l.Detail); len(f) > 0 {
			pid, _ = strings.CutPrefix(f[0], "pid=")
		}
	}
	if pid == "" {
		t.Fatalf("the exec did not print its pid before the deadline: %q", res.Stdout)
	}

	pids := func() []string {
		t.Helper()
		r, err := c.Exec(ctx, FixtureBinary, "pids")
		if err != nil || r.ExitCode != 0 {
			t.Fatalf("pids: %+v %v", r, err)
		}
		l, _ := probeout.Find(probeout.Parse(r.Stdout), "pids")
		return strings.Fields(l.Detail)
	}
	time.Sleep(500 * time.Millisecond)
	if got := pids(); !slices.Contains(got, pid) {
		t.Errorf("after the cancel, pid %s is gone from %v: cancelling did stop the process", pid, got)
	} else {
		t.Logf("after the cancel, exec pid %s still runs: pids %v", pid, got)
	}

	if err := c.Stop(ctx, 0); err != nil {
		t.Fatal(err)
	}
	// Restart the stopped container: only PID 1 and the pids probe itself
	// are left, so the orphaned exec ended with the container.
	if _, err := cli.ContainerStart(ctx, c.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, ctx, c, "sleep: OK pid=1 ")
	if got := pids(); len(got) != 2 || got[0] != "1" {
		t.Errorf("after stop and restart, pids %v, want PID 1 and the probe only", got)
	} else {
		t.Logf("after stop and restart: pids %v", got)
	}
}

// TestSpec_H3_GatedObservedMatchesIntent: the engine records what Translate
// sent, for Restricted (with a volume) and for a default spec. A raw create
// with an empty HostConfig shows that private IPC and cgroup namespaces are
// the engine's own defaults, so always sending them keeps the zero Spec at
// Docker's default.
func TestSpec_H3_GatedObservedMatchesIntent(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "observed")
	vol, err := CreateVolume(ctx, cli, lesson, "work")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = cli.VolumeRemove(context.Background(), vol, client.VolumeRemoveOptions{Force: true})
	})
	r := Restricted(lesson, "restricted", img, "id")
	r.Mounts = append(r.Mounts, Mount{Type: MountVolume, Source: vol, Target: "/work", ReadOnly: true})
	r.Groups = []string{"30000"}
	for _, s := range []Spec{r, {Lesson: lesson, Role: "default", Image: img}} {
		c, err := Create(ctx, cli, s)
		if err != nil {
			t.Fatalf("%s: %v", s.Role, err)
		}
		removeOnCleanup(t, c)
		insp, err := c.Inspect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if bad := CheckObserved(insp, c.Request); len(bad) > 0 {
			t.Errorf("%s: %v", s.Role, bad)
		}
		if got := IsRestricted(*insp.Config, *insp.HostConfig); got != (s.Role == "restricted") {
			t.Errorf("%s: IsRestricted on the engine's record = %v", s.Role, got)
		}
		t.Logf("%s: engine recorded network %q, runtime %q, ipc %q, cgroupns %q", s.Role,
			insp.HostConfig.NetworkMode, insp.HostConfig.Runtime, insp.HostConfig.IpcMode, insp.HostConfig.CgroupnsMode)
	}

	raw, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:       ContainerName(lesson, "raw", NameSuffix()),
		Config:     &container.Config{Image: img, Labels: Labels(lesson)},
		HostConfig: &container.HostConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = cli.ContainerRemove(context.Background(), raw.ID, client.ContainerRemoveOptions{Force: true})
	})
	insp, err := cli.ContainerInspect(ctx, raw.ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hc := insp.Container.HostConfig; hc.IpcMode != "private" || hc.CgroupnsMode != "private" {
		t.Errorf("engine defaults: IpcMode %q, CgroupnsMode %q; Translate's private/private would differ from Docker's default", hc.IpcMode, hc.CgroupnsMode)
	}
}

// ---- TestProp_LifecycleNoLeak --------------------------------------------

// capRapid lowers rapid's checks and steps for a daemon-backed property
// unless the caller chose them (flag or RAPID_* environment variable).
func capRapid(t *testing.T, checks, steps int) {
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for name, v := range map[string]int{"rapid.checks": checks, "rapid.steps": steps} {
		env := "RAPID_" + strings.ToUpper(strings.TrimPrefix(name, "rapid."))
		f := flag.Lookup(name)
		if f == nil || set[name] || os.Getenv(env) != "" {
			continue
		}
		old := f.Value.String()
		if err := f.Value.Set(strconv.Itoa(v)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Value.Set(old) })
	}
}

// lifeModel is what the state machine expects the daemon to hold for the
// lesson: container name to state (created, running or exited).
type lifeModel struct {
	ctx     context.Context
	cli     *client.Client
	img     string
	lesson  string
	handles map[string]*Container
	state   map[string]container.ContainerState
	n       int
}

func (m *lifeModel) pick(t *rapid.T, states ...container.ContainerState) *Container {
	var names []string
	for name, s := range m.state {
		if slices.Contains(states, s) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Skip("no container in that state")
	}
	slices.Sort(names)
	return m.handles[rapid.SampledFrom(names).Draw(t, "container")]
}

func (m *lifeModel) create(t *rapid.T, start bool) {
	m.n++
	role := fmt.Sprintf("c%d", m.n)
	s := Spec{Lesson: m.lesson, Role: role, Image: m.img, Cmd: []string{"sleep"}}
	if rapid.Bool().Draw(t, "restricted") {
		s = Restricted(m.lesson, role, m.img, "sleep")
	}
	var c *Container
	var err error
	if start {
		c, err = Start(m.ctx, m.cli, s)
	} else {
		c, err = Create(m.ctx, m.cli, s)
	}
	if err != nil {
		t.Fatalf("create (start=%v): %v", start, err)
	}
	m.handles[c.Name] = c
	m.state[c.Name] = map[bool]container.ContainerState{true: "running", false: "created"}[start]
}

func (m *lifeModel) actions() map[string]func(*rapid.T) {
	return map[string]func(*rapid.T){
		"create": func(t *rapid.T) { m.create(t, false) },
		"start":  func(t *rapid.T) { m.create(t, true) },
		"startCreated": func(t *rapid.T) {
			c := m.pick(t, "created")
			if _, err := m.cli.ContainerStart(m.ctx, c.ID, client.ContainerStartOptions{}); err != nil {
				t.Fatalf("start %s: %v", c.Name, err)
			}
			m.state[c.Name] = "running"
		},
		"exec": func(t *rapid.T) {
			c := m.pick(t, "running")
			args, want := []string{FixtureBinary, "id"}, 0
			if rapid.Bool().Draw(t, "usage error") {
				args, want = []string{FixtureBinary, "alloc"}, 2
			}
			res, err := c.Exec(m.ctx, args...)
			if err != nil || res.ExitCode != want {
				t.Fatalf("exec %v in %s: %+v %v, want exit %d", args, c.Name, res, err, want)
			}
		},
		"stop": func(t *rapid.T) {
			c := m.pick(t, "running")
			if err := c.Stop(m.ctx, 0); err != nil {
				t.Fatal(err)
			}
			m.state[c.Name] = "exited"
		},
		"remove": func(t *rapid.T) {
			c := m.pick(t, "created", "running", "exited")
			if err := c.Remove(m.ctx); err != nil {
				t.Fatal(err)
			}
			delete(m.handles, c.Name)
			delete(m.state, c.Name)
		},
		"sweep": func(t *rapid.T) {
			removed, err := Sweep(m.ctx, m.cli, m.lesson)
			if err != nil {
				t.Fatal(err)
			}
			if len(removed.Containers) != len(m.state) {
				t.Fatalf("Sweep removed %v, the model held %v", removed.Containers, m.state)
			}
			clear(m.handles)
			clear(m.state)
		},
		"": m.check,
	}
}

// check compares the daemon with the model: the same names carry the
// lesson label, each in the modelled state.
func (m *lifeModel) check(t *rapid.T) {
	cs, err := m.cli.ContainerList(m.ctx, client.ContainerListOptions{All: true, Filters: lessonFilter(m.lesson)})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]container.ContainerState{}
	for _, c := range cs.Items {
		got[containerName(c.Names, c.ID)] = c.State
	}
	for name, want := range m.state {
		if got[name] != want {
			t.Fatalf("%s is %q on the daemon, %q in the model", name, got[name], want)
		}
	}
	if len(got) != len(m.state) {
		t.Fatalf("daemon holds %v, model %v", got, m.state)
	}
}

func TestProp_LifecycleNoLeak(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := testLesson(t, cli, "lifecycle")
	capRapid(t, 4, 12) // each step talks to the daemon; keep check-docker in minutes
	rapid.Check(t, func(t *rapid.T) {
		if _, err := Sweep(ctx, cli, lesson); err != nil { // a fresh start per case
			t.Fatal(err)
		}
		m := &lifeModel{ctx: ctx, cli: cli, img: img, lesson: lesson, handles: map[string]*Container{}, state: map[string]container.ContainerState{}}
		t.Repeat(m.actions())
		// Removing every handle must leave nothing: no-leak without Sweep.
		for _, c := range m.handles {
			if err := c.Remove(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if bad, err := CheckNoLeak(ctx, cli, lesson); err != nil || len(bad) > 0 {
			t.Fatalf("no-leak after removing every handle: %v %v", bad, err)
		}
	})
}
