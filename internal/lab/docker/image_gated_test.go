package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// gatedClient connects for a daemon test and closes the client afterwards.
func gatedClient(t *testing.T) (context.Context, *client.Client) {
	t.Helper()
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	cli := Connect(ctx)
	t.Cleanup(func() { _ = cli.Close() })
	return ctx, cli
}

func randomName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// removeHarnessImage force-removes a test's image when the test ends, but only
// an image that carries the harness label: anything else is not this test's to
// delete.
func removeHarnessImage(t *testing.T, cli *client.Client, tag string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		res, err := cli.ImageInspect(ctx, tag)
		if err != nil {
			return // never built, or already gone
		}
		if res.Config == nil || res.Config.Labels[LabelHarness] != "1" {
			t.Errorf("image %s lacks the %s label; leaving it alone", tag, LabelHarness)
			return
		}
		if _, err := cli.ImageRemove(ctx, tag, client.ImageRemoveOptions{Force: true, PruneChildren: true}); err != nil {
			t.Errorf("remove image %s: %v", tag, err)
		}
	})
}

// buildProbeImage builds the probe fixture under a unique tag and removes it
// when the test ends.
func buildProbeImage(t *testing.T, ctx context.Context, cli *client.Client, dirs ...OwnedDir) FixtureImage {
	t.Helper()
	img := FixtureImage{
		Lesson:  "harness",
		Tag:     "trinkets-harness-probe-" + randomName(t) + ":dev",
		Package: "./probe",
		Dirs:    dirs,
	}
	removeHarnessImage(t, cli, img.Tag)
	start := time.Now()
	if err := BuildFixture(ctx, cli, img); err != nil {
		t.Fatalf("BuildFixture: %v", err)
	}
	t.Logf("built %s in %v", img.Tag, time.Since(start).Round(10*time.Millisecond))
	return img
}

// testRun describes a throwaway container for a test; H3's launcher will
// replace this. It always carries the harness labels.
type testRun struct {
	Image      string
	Entrypoint []string
	Cmd        []string
	User       string
	Host       container.HostConfig
}

type testRunResult struct {
	ID       string
	ExitCode int64
	Stdout   string
	Stderr   string
}

// createTestContainer creates a labelled container and force-removes it when
// the test ends.
func createTestContainer(t *testing.T, ctx context.Context, cli *client.Client, tr testRun) string {
	t.Helper()
	res, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: "trinkets-harness-test-" + randomName(t),
		Config: &container.Config{
			Image:      tr.Image,
			Entrypoint: tr.Entrypoint,
			Cmd:        tr.Cmd,
			User:       tr.User,
			Labels:     Labels("harness"),
		},
		HostConfig: &tr.Host,
	})
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if _, err := cli.ContainerRemove(ctx, res.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
			t.Errorf("remove container %s: %v", res.ID, err)
		}
	})
	return res.ID
}

// containerLogs returns the demultiplexed stdout and stderr of a container.
func containerLogs(ctx context.Context, cli *client.Client, id string) (stdout, stderr string, err error) {
	logs, err := cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", "", err
	}
	defer logs.Close()
	var so, se bytes.Buffer
	if _, err := stdcopy.StdCopy(&so, &se, logs); err != nil {
		return "", "", err
	}
	return so.String(), se.String(), nil
}

// runOnce creates, starts and waits for a container (wait registered before
// start, so a fast exit is not missed) and returns its output. A start
// failure comes back as the error with the container still removed at
// cleanup.
func runOnce(t *testing.T, ctx context.Context, cli *client.Client, tr testRun) (testRunResult, error) {
	t.Helper()
	id := createTestContainer(t, ctx, cli, tr)
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	wait := cli.ContainerWait(waitCtx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNextExit})
	if _, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		return testRunResult{ID: id}, err
	}
	var code int64
	select {
	case r := <-wait.Result:
		code = r.StatusCode
	case err := <-wait.Error:
		t.Fatalf("wait: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for the container")
	}
	stdout, stderr, err := containerLogs(ctx, cli, id)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	return testRunResult{ID: id, ExitCode: code, Stdout: stdout, Stderr: stderr}, nil
}

// imageLayerFiles lists every regular file in an image's layers (and every
// directory, marked with a trailing slash) by saving the image and reading
// its manifest. It works for the classic and the containerd image store.
func imageLayerFiles(t *testing.T, ctx context.Context, cli *client.Client, tag string) map[string]*tar.Header {
	t.Helper()
	save, err := cli.ImageSave(ctx, []string{tag})
	if err != nil {
		t.Fatalf("ImageSave: %v", err)
	}
	defer save.Close()
	blobs := map[string][]byte{}
	tr := tar.NewReader(save)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading saved image: %v", err)
		}
		if h.Typeflag == tar.TypeReg {
			if blobs[h.Name], err = io.ReadAll(tr); err != nil {
				t.Fatal(err)
			}
		}
	}
	var manifest []struct{ Layers []string }
	if err := json.Unmarshal(blobs["manifest.json"], &manifest); err != nil || len(manifest) != 1 {
		t.Fatalf("manifest.json: %v (%d entries)", err, len(manifest))
	}
	files := map[string]*tar.Header{}
	for _, layer := range manifest[0].Layers {
		var r io.Reader = bytes.NewReader(blobs[layer])
		if gz, err := gzip.NewReader(bytes.NewReader(blobs[layer])); err == nil {
			r = gz
		}
		lr := tar.NewReader(r)
		for {
			h, err := lr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("reading layer %s: %v", layer, err)
			}
			files["/"+strings.TrimPrefix(path.Clean(h.Name), "/")] = h
		}
	}
	return files
}

var numericUserRE = regexp.MustCompile(`^[0-9]+(:[0-9]+)?$`)

func TestSpec_H2_GatedImageHasLabelsNumericUserAndEngineArch(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := buildProbeImage(t, ctx, cli, OwnedDir{"/work", 10001, 10001})
	res, err := cli.ImageInspect(ctx, img.Tag)
	if err != nil {
		t.Fatal(err)
	}
	if res.Config == nil {
		t.Fatal("inspect returned no config")
	}
	if got := res.Config.Labels[LabelHarness]; got != "1" {
		t.Errorf("label %s = %q, want 1", LabelHarness, got)
	}
	if got := res.Config.Labels[LabelLesson]; got != "harness" {
		t.Errorf("label %s = %q, want harness", LabelLesson, got)
	}
	if !numericUserRE.MatchString(res.Config.User) || res.Config.User != DefaultFixtureUser {
		t.Errorf("USER = %q, want the numeric default %s", res.Config.User, DefaultFixtureUser)
	}
	if want := EngineArch(ctx, cli); res.Architecture != want || res.Os != "linux" {
		t.Errorf("image is %s/%s, engine arch is linux/%s", res.Os, res.Architecture, want)
	}
	if !slices.Contains(res.Config.Env, probeMarker+"=1") {
		t.Errorf("image env %v lacks %s=1, so the probe would refuse its mutating commands", res.Config.Env, probeMarker)
	}
	if fmt.Sprint(res.Config.Entrypoint) != "[/fixture]" {
		t.Errorf("entrypoint = %v", res.Config.Entrypoint)
	}
	files := imageLayerFiles(t, ctx, cli, img.Tag)
	if h := files["/work"]; h == nil || h.Uid != 10001 || h.Gid != 10001 {
		t.Errorf("/work in the image = %+v, want a directory owned by 10001:10001", h)
	}
}

func TestSpec_H2_GatedImageHasOneBinaryAndNoShell(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := buildProbeImage(t, ctx, cli, OwnedDir{"/work", 10001, 10001})

	var regular []string
	for name, h := range imageLayerFiles(t, ctx, cli, img.Tag) {
		if h.Typeflag == tar.TypeReg {
			regular = append(regular, name)
		}
	}
	if fmt.Sprint(regular) != "[/fixture]" {
		t.Errorf("regular files in the image = %v, want only /fixture", regular)
	}

	// docker run --rm <tag> id
	run, err := runOnce(t, ctx, cli, testRun{Image: img.Tag, Cmd: []string{"id"}})
	if err != nil {
		t.Fatal(err)
	}
	lines := probeout.Parse(run.Stdout)
	l, ok := probeout.Find(lines, "identity")
	if run.ExitCode != 0 || !ok || !strings.HasPrefix(l.Detail, "uid=10001 gid=10001 ") {
		t.Errorf("id: exit %d stdout %q stderr %q; want identity of 10001:10001", run.ExitCode, run.Stdout, run.Stderr)
	}

	// docker run --rm --entrypoint sh <tag>
	run, err = runOnce(t, ctx, cli, testRun{Image: img.Tag, Entrypoint: []string{"sh"}})
	if err == nil {
		t.Fatalf("entrypoint sh ran (exit %d): the image must not contain a shell", run.ExitCode)
	}
	if !strings.Contains(err.Error(), "executable file not found") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("start failed, but not for a missing executable: %v", err)
	}
}

func TestSpec_H2_GatedBrokenDockerfileIsAnError(t *testing.T) {
	ctx, cli := gatedClient(t)
	tests := []struct {
		name, dockerfile, want string
	}{
		{"unknown instruction", "FROM scratch\nBOGUS line\n", "BOGUS"},
		{"missing source file", "FROM scratch\nCOPY nope /nope\n", "nope"},
		{"failing run", "FROM scratch\nRUN /nonexistent\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tag := "trinkets-harness-broken-" + randomName(t) + ":dev"
			removeHarnessImage(t, cli, tag)
			var buf bytes.Buffer
			if err := writeTar(&buf, []contextFile{{Name: "Dockerfile", Mode: 0o644, Data: []byte(tc.dockerfile)}}); err != nil {
				t.Fatal(err)
			}
			err := buildContext(ctx, cli, &buf, tag, "harness")
			if err == nil {
				t.Fatal("a broken Dockerfile built without error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
			t.Log(err)
			if _, err := cli.ImageInspect(ctx, tag); err == nil {
				t.Error("a failed build left the tag behind")
			}
		})
	}
}

// restrictedHost is the isolation the plan calls Restricted, written out by
// hand until H3's preset exists.
func restrictedHost() container.HostConfig {
	pids := int64(64)
	return container.HostConfig{
		NetworkMode:    "none",
		ReadonlyRootfs: true,
		CapDrop:        []string{"ALL"},
		SecurityOpt:    []string{"no-new-privileges:true"},
		Tmpfs:          map[string]string{"/tmp": "rw,noexec,nosuid,size=16m"},
		Resources:      container.Resources{Memory: 64 << 20, MemorySwap: 64 << 20, NanoCPUs: 500_000_000, PidsLimit: &pids},
	}
}

// TestSpec_H2_GatedProbeReportsIsolationRows checks that the probe can produce
// the rows H4 will assert (plan, "Isolation, same image and probe"). It does
// not assert the whole table: that is H4's job.
func TestSpec_H2_GatedProbeReportsIsolationRows(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := buildProbeImage(t, ctx, cli, OwnedDir{"/work", 10001, 10001})

	batteryLines := func(user string, host container.HostConfig) []probeout.Line {
		t.Helper()
		run, err := runOnce(t, ctx, cli, testRun{Image: img.Tag, Cmd: []string{"battery"}, User: user, Host: host})
		if err != nil {
			t.Fatal(err)
		}
		if run.ExitCode != 0 {
			t.Errorf("battery exit %d, want 0 (it is a survey): %s", run.ExitCode, run.Stderr)
		}
		lines := probeout.Parse(run.Stdout)
		for _, l := range probeout.Malformed(lines) {
			t.Errorf("malformed probe line %q: %v", l.Raw, l.Err)
		}
		t.Logf("battery as %q:\n%s", user, probeout.Format(lines))
		return lines
	}
	detail := func(lines []probeout.Line, name string, ok bool) string {
		t.Helper()
		l, found := probeout.Find(lines, name)
		if !found || l.OK != ok {
			t.Errorf("%s: found=%v OK=%v detail=%q, want OK=%v", name, found, l.OK, l.Detail, ok)
		}
		return l.Detail
	}

	root := batteryLines("0:0", container.HostConfig{})
	if d := detail(root, "identity", true); !strings.HasPrefix(d, "uid=0 gid=0 ") {
		t.Errorf("default identity %q", d)
	}
	if d := detail(root, "cap-eff", true); d == "0000000000000000" {
		t.Errorf("default container has no capabilities: %q", d)
	}
	detail(root, "no-new-privs", true)
	detail(root, "seccomp", true)
	detail(root, "setuid", true)
	detail(root, "write-root", true)
	if d := detail(root, "interfaces", true); !strings.Contains(" "+d+" ", " eth0 ") {
		t.Errorf("default network interfaces %q", d)
	}
	for _, denied := range []string{"mount", "unshare-user", "keyctl"} {
		detail(root, denied, false) // the engine's default seccomp profile and capability set
	}

	r := batteryLines("10001:10001", restrictedHost())
	if d := detail(r, "identity", true); !strings.HasPrefix(d, "uid=10001 gid=10001 ") {
		t.Errorf("restricted identity %q", d)
	}
	if d := detail(r, "cap-eff", true); d != "0000000000000000" {
		t.Errorf("restricted CapEff = %q, want zero", d)
	}
	if d := detail(r, "no-new-privs", true); d != "1" {
		t.Errorf("restricted NoNewPrivs = %q", d)
	}
	if d := detail(r, "pids", true); d != "1" {
		t.Errorf("restricted visible pids = %q", d)
	}
	if d := detail(r, "interfaces", true); d != "lo" {
		t.Errorf("restricted interfaces = %q", d)
	}
	if d := detail(r, "write-root", false); !strings.Contains(d, "read-only file system") {
		t.Errorf("restricted root write: %q", d)
	}
	detail(r, "setuid", false)
	for _, name := range []string{"memory.max", "pids.max", "cpu.max"} {
		detail(r, name, true)
	}
	if d := detail(r, "memory.max", true); d != "67108864" {
		t.Errorf("memory.max = %q", d)
	}
	if d := detail(r, "pids.max", true); d != "64" {
		t.Errorf("pids.max = %q", d)
	}
	if d := detail(r, "cpu.max", true); d != "50000 100000" {
		t.Errorf("cpu.max = %q", d)
	}

	// The argument-taking checks, one exec each, in the restricted container.
	one := func(args ...string) (int64, []probeout.Line) {
		t.Helper()
		run, err := runOnce(t, ctx, cli, testRun{Image: img.Tag, Cmd: args, User: "10001:10001", Host: restrictedHost()})
		if err != nil {
			t.Fatal(err)
		}
		return run.ExitCode, probeout.Parse(run.Stdout)
	}
	if code, lines := one("write", "/work"); code != 1 {
		// /work is on the read-only root filesystem without a volume mounted
		t.Errorf("write /work on a read-only root: exit %d %q", code, probeout.Format(lines))
	}
	if code, lines := one("write", "/tmp"); code != 0 {
		t.Errorf("write /tmp (tmpfs): exit %d %q", code, probeout.Format(lines))
	}
	if code, lines := one("stat", "/work"); code != 0 || detail(lines, "stat", true) == "" ||
		!strings.Contains(detail(lines, "stat", true), "uid=10001 gid=10001") {
		t.Errorf("stat /work: exit %d %q", code, probeout.Format(lines))
	}
	if code, _ := one("chown", "/work", "0:0"); code != 1 {
		t.Errorf("chown as 10001 with no capabilities: exit %d, want 1", code)
	}
	if code, lines := one("dial", "tcp", "1.1.1.1:53"); code != 1 {
		t.Errorf("dial with network none: exit %d %q", code, probeout.Format(lines))
	} else if d := detail(lines, "dial", false); !strings.Contains(d, "network is unreachable") {
		t.Errorf("dial detail %q", d)
	}
	if code, _ := one("alloc"); code != 2 {
		t.Errorf("usage error exit = %d, want 2", code)
	}
	if code, lines := one("fork", "200"); code != 1 {
		t.Errorf("fork 200 under PidsLimit 64: exit %d %q", code, probeout.Format(lines))
	} else {
		d := detail(lines, "fork-started", true)
		t.Logf("fork started %s children under PidsLimit 64", d)
		detail(lines, "fork", false)
	}
}

// TestSpec_H2_GatedProbeSleepHandlesOrIgnoresSIGTERM checks the two fixtures H3
// stops: one exits on SIGTERM within the grace period, one has to be killed.
func TestSpec_H2_GatedProbeSleepHandlesOrIgnoresSIGTERM(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := buildProbeImage(t, ctx, cli)
	stop := func(name string, cmd ...string) (code int64, took time.Duration) {
		t.Helper()
		id := createTestContainer(t, ctx, cli, testRun{Image: img.Tag, Cmd: cmd})
		if _, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			stdout, _, err := containerLogs(ctx, cli, id)
			if err == nil && strings.Contains(stdout, "sleep: OK pid=") {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s never printed its start line", name)
			}
			time.Sleep(50 * time.Millisecond)
		}
		grace := 2
		start := time.Now()
		if _, err := cli.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &grace}); err != nil {
			t.Fatal(err)
		}
		took = time.Since(start)
		insp, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return int64(insp.Container.State.ExitCode), took
	}
	if code, took := stop("handler", "sleep"); code != 0 || took >= 2*time.Second {
		t.Errorf("sleep handling SIGTERM: exit %d after %v, want 0 well inside the 2 s grace", code, took)
	}
	if code, took := stop("ignorer", "sleep", "--ignore-term"); code != 137 || took < 2*time.Second {
		t.Errorf("sleep --ignore-term: exit %d after %v, want 137 (SIGKILL) after the 2 s grace", code, took)
	}
}

// execIn runs a command in a started container through the Engine API and
// returns its exit code and demultiplexed output.
func execIn(t *testing.T, ctx context.Context, cli *client.Client, id string, cmd ...string) (code int, stdout, stderr string) {
	t.Helper()
	ex, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{Cmd: cmd, AttachStdout: true, AttachStderr: true})
	if err != nil {
		t.Fatalf("exec create %v: %v", cmd, err)
	}
	att, err := cli.ExecAttach(ctx, ex.ID, client.ExecAttachOptions{})
	if err != nil {
		t.Fatalf("exec attach %v: %v", cmd, err)
	}
	defer att.Close()
	var so, se bytes.Buffer
	if _, err := stdcopy.StdCopy(&so, &se, att.Reader); err != nil {
		t.Fatalf("exec output %v: %v", cmd, err)
	}
	insp, err := cli.ExecInspect(ctx, ex.ID, client.ExecInspectOptions{})
	if err != nil {
		t.Fatalf("exec inspect %v: %v", cmd, err)
	}
	return insp.ExitCode, so.String(), se.String()
}

// TestSpec_H2_GatedOwnedDirsKeepPathAndOwner builds directories using every
// character an owned-directory path may contain, and a parent listed after its
// child, then reads the image layers back: each path and uid:gid must be the
// requested one. Unlisted parents are root-owned.
func TestSpec_H2_GatedOwnedDirsKeepPathAndOwner(t *testing.T) {
	ctx, cli := gatedClient(t)
	dirs := []OwnedDir{
		{"/x/y", 3, 4}, // child before parent: COPY --chown only owns what it creates
		{"/x", 5, 6},
		{"/a.b_c@d+e-f", 1, 2},
		{"/A0.9/-lead", 7, 8},
		{"/.hidden", 9, 10},
		{"/m-n/o+p/q@r", 11, 12},
		{"/dots..in", 13, 14},
		{"/deep/er/est", 4294967294, 4294967294},
	}
	img := buildProbeImage(t, ctx, cli, dirs...)
	files := imageLayerFiles(t, ctx, cli, img.Tag)
	for _, d := range dirs {
		h := files[d.Path]
		switch {
		case h == nil:
			t.Errorf("%s is not in the image", d.Path)
		case h.Typeflag != tar.TypeDir:
			t.Errorf("%s is type %q, want a directory", d.Path, h.Typeflag)
		case h.Uid != d.UID || h.Gid != d.GID:
			t.Errorf("%s is owned %d:%d, want %d:%d", d.Path, h.Uid, h.Gid, d.UID, d.GID)
		}
	}
	for _, parent := range []string{"/m-n", "/m-n/o+p", "/deep", "/deep/er"} {
		if h := files[parent]; h == nil || h.Uid != 0 || h.Gid != 0 {
			t.Errorf("unlisted parent %s = %+v, want a root-owned directory", parent, h)
		}
	}
	// Nothing extra: the paths the builder would have mangled ($x -> "", \x)
	// are the rejected ones, so the image holds only the requested tree.
	for name, h := range files {
		if h.Typeflag == tar.TypeReg && name != "/fixture" {
			t.Errorf("unexpected file %s", name)
		}
	}
}

// TestSpec_H2_GatedProbeActionsRunInsideContainers runs the probe's
// machine-changing commands where they belong, in a default root container, and
// checks their effect with stat. The host-side tests never run these.
func TestSpec_H2_GatedProbeActionsRunInsideContainers(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := buildProbeImage(t, ctx, cli, OwnedDir{"/work", 10001, 10001})
	id := createTestContainer(t, ctx, cli, testRun{Image: img.Tag, Cmd: []string{"sleep"}, User: "0:0"})
	if _, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	probe := func(wantCode int, args ...string) []probeout.Line {
		t.Helper()
		// exec does not apply the image entrypoint
		code, stdout, stderr := execIn(t, ctx, cli, id, append([]string{fixtureBinary}, args...)...)
		if code != wantCode {
			t.Errorf("%v: exit %d, want %d (stdout %q stderr %q)", args, code, wantCode, stdout, stderr)
		}
		lines := probeout.Parse(stdout)
		for _, l := range probeout.Malformed(lines) {
			t.Errorf("%v: malformed line %q", args, l.Raw)
		}
		return lines
	}
	stat := func(path string) string {
		t.Helper()
		l, ok := probeout.Find(probe(0, "stat", path), "stat")
		if !ok || !l.OK {
			t.Fatalf("stat %s: %+v", path, l)
		}
		return l.Detail
	}
	mustHave := func(got string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("%q lacks %q", got, w)
			}
		}
	}

	mustHave(stat("/work"), "uid=10001 gid=10001", "perm=0755")
	probe(0, "initdir", "/work", "20000:30000", "2750")
	mustHave(stat("/work"), "uid=20000 gid=30000", "perm=2750", "mode=drwxr-s---")
	probe(0, "chown", "/work", "5:6")
	mustHave(stat("/work"), "uid=5 gid=6")
	probe(0, "chmod", "/work", "0700")
	mustHave(stat("/work"), "perm=0700")
	probe(0, "write", "/work")
	if l, ok := probeout.Find(probe(1, "unlink", "/work"), "unlink"); !ok || l.OK {
		t.Errorf("unlink of a directory should be DENIED: %+v", l)
	}
	probe(1, "initdir", "/no-such-dir", "1:1", "0755")
	if l, ok := probeout.Find(probe(0, "alloc", "4194304"), "alloc"); !ok || l.Detail != "4194304" {
		t.Errorf("alloc: %+v", l)
	}
	if l, ok := probeout.Find(probe(0, "fork", "3"), "fork-started"); !ok || l.Detail != "3" {
		t.Errorf("fork: %+v", l)
	}
	if l, ok := probeout.Find(probe(0, "setuid", "12345"), "setuid"); !ok || !l.OK {
		t.Errorf("setuid as root: %+v", l)
	}
	probe(1, "unshare-user") // denied by the engine's default seccomp profile
	probe(0, "unlink", "/fixture")
}
