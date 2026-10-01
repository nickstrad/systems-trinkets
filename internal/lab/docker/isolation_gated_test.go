package docker

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/moby/moby/client"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// H4 isolation tests (TRINKETS_DOCKER=1). They run the probe fixture under the
// default container and under Restricted and assert what the plan's "Verified
// facts" table says, one subtest per row. Lesson labels start with h4-.

// workVolume creates a labelled volume and removes it when the test ends
// (before sweptLesson's no-leak check).
func workVolume(t *testing.T, ctx context.Context, cli *client.Client, lesson, role string) string {
	t.Helper()
	vol, err := CreateVolume(ctx, cli, lesson, role)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = cli.VolumeRemove(context.Background(), vol, client.VolumeRemoveOptions{Force: true})
	})
	return vol
}

// runLines runs a one-shot container and parses its probe output.
func runLines(t *testing.T, ctx context.Context, cli *client.Client, spec Spec) (Result, []probeout.Line) {
	t.Helper()
	res, err := Run(ctx, cli, spec)
	if err != nil {
		t.Fatalf("%s %q: %v", spec.Role, spec.Cmd, err)
	}
	lines := probeout.Parse(res.Stdout)
	for _, l := range probeout.Malformed(lines) {
		t.Errorf("%s %q: malformed probe line %q: %v", spec.Role, spec.Cmd, l.Raw, l.Err)
	}
	return res, lines
}

// want is what one probe line should say in one container. re matches the
// detail ("" accepts any); fn replaces ok and re for a row whose answer
// depends on the machine.
type want struct {
	ok bool
	re string
	fn func(probeout.Line) string // returns a problem, or ""
}

func (w want) check(l probeout.Line) string {
	if w.fn != nil {
		return w.fn(l)
	}
	if l.OK != w.ok {
		return "OK=" + strconv.FormatBool(l.OK) + " want " + strconv.FormatBool(w.ok) + " (" + l.Detail + ")"
	}
	if w.re != "" && !regexp.MustCompile(w.re).MatchString(l.Detail) {
		return "detail " + strconv.Quote(l.Detail) + " does not match " + w.re
	}
	return ""
}

func wantOK(re string) want     { return want{ok: true, re: re} }
func wantDenied(re string) want { return want{ok: false, re: re} }

// TestSpec_H4_GatedIsolationTable runs the probe in the default container
// (User "0:0") and in Restricted and asserts the plan's isolation table, one
// subtest per row, so a regression names the probe that changed.
//
// The default container's pids.max is the engine's own number (9483 on the
// Linux host where the table was made), so that cell only has to be "max or a
// number"; the dial cell only has to differ from "network is unreachable",
// since an offline machine still has a route. Restricted cells are exact.
func TestSpec_H4_GatedIsolationTable(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h4-table")

	// Each mode gets its own volume at /work: the default run's root chown
	// would otherwise change who owns the restricted run's volume.
	modes := []struct {
		name string
		spec func(vol string, cmd ...string) Spec
	}{
		{"default", func(vol string, cmd ...string) Spec {
			return Spec{Lesson: lesson, Role: "default", Image: img, Cmd: cmd, User: "0:0",
				Mounts: []Mount{{Type: MountVolume, Source: vol, Target: "/work"}}}
		}},
		{"restricted", func(vol string, cmd ...string) Spec {
			s := Restricted(lesson, "restricted", img, cmd...)
			s.Mounts = append(s.Mounts, Mount{Type: MountVolume, Source: vol, Target: "/work"})
			return s
		}},
	}
	got := map[string]map[string]probeout.Line{} // mode -> row -> line
	for _, m := range modes {
		vol := workVolume(t, ctx, cli, lesson, m.name+"-work")
		rows := map[string]probeout.Line{}
		got[m.name] = rows

		res, lines := runLines(t, ctx, cli, m.spec(vol, "battery"))
		if res.ExitCode != 0 {
			t.Errorf("%s battery exit %d, want 0 (it is a survey): %s", m.name, res.ExitCode, res.Stderr)
		}
		for _, l := range lines {
			rows[l.Name] = l
		}
		// Checks that take arguments, one container each. The setuid in the
		// battery runs last, so it cannot disturb the rows before it.
		for _, c := range []struct {
			row, name string
			cmd       []string
		}{
			{"write-work", "write", []string{"write", "/work"}},
			{"write-tmp", "write", []string{"write", "/tmp"}},
			{"chown", "chown", []string{"chown", "/work", "0:0"}},
			{"dial", "dial", []string{"dial", "tcp", "1.1.1.1:53"}},
		} {
			_, lines := runLines(t, ctx, cli, m.spec(vol, c.cmd...))
			l, found := probeout.Find(lines, c.name)
			if !found {
				t.Errorf("%s %s: no %q line in %q", m.name, c.row, c.name, probeout.Format(lines))
			}
			rows[c.row] = l
		}
	}

	notUnreachable := func(l probeout.Line) string {
		if strings.Contains(l.Detail, "network is unreachable") {
			return "default container has no route: " + l.Detail
		}
		return ""
	}
	table := []struct {
		row                string
		defaultC, restrict want
	}{
		{"identity", wantOK(`^uid=0 gid=0 `), wantOK(`^uid=10001 gid=10001 `)},
		{"cap-eff", wantOK(`^00000000a80425fb$`), wantOK(`^0000000000000000$`)},
		{"no-new-privs", wantOK(`^0$`), wantOK(`^1$`)},
		{"seccomp", wantOK(`^2$`), wantOK(`^2$`)},
		// PID 1 is the probe itself because these are one-shot runs.
		{"pids", wantOK(`^1$`), wantOK(`^1$`)},
		{"interfaces", wantOK(`^lo eth0$`), wantOK(`^lo$`)},
		{"write-root", wantOK(``), wantDenied(`read-only file system`)},
		{"write-work", wantOK(``), wantOK(``)},
		// A scratch image has no /tmp; Restricted mounts a tmpfs there.
		{"write-tmp", wantDenied(`no such file or directory`), wantOK(``)},
		{"setuid", wantOK(``), wantDenied(`operation not permitted`)},
		{"chown", wantOK(``), wantDenied(`operation not permitted`)},
		{"dial", want{fn: notUnreachable}, wantDenied(`network is unreachable`)},
		{"mount", wantDenied(``), wantDenied(``)},
		{"unshare-user", wantDenied(``), wantDenied(``)},
		{"keyctl", wantDenied(``), wantDenied(``)},
		{"memory.max", wantOK(`^max$`), wantOK(`^67108864$`)},
		{"pids.max", wantOK(`^(max|\d+)$`), wantOK(`^64$`)},
		{"cpu.max", wantOK(`^max 100000$`), wantOK(`^50000 100000$`)},
	}
	for _, tc := range table {
		t.Run(tc.row, func(t *testing.T) {
			for _, c := range []struct {
				mode string
				w    want
			}{{"default", tc.defaultC}, {"restricted", tc.restrict}} {
				l, found := got[c.mode][tc.row]
				if !found {
					t.Errorf("%s: probe printed no %q line", c.mode, tc.row)
					continue
				}
				if problem := c.w.check(l); problem != "" {
					t.Errorf("%s: %s", c.mode, problem)
				}
			}
		})
	}
}

// TestSpec_H4_GatedReadOnlyVolumeMount: the same volume rejects a write when
// mounted ReadOnly and accepts it when mounted read-write.
func TestSpec_H4_GatedReadOnlyVolumeMount(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h4-rovol")
	vol := workVolume(t, ctx, cli, lesson, "work")

	for _, tc := range []struct {
		name     string
		readOnly bool
		exit     int
		line     want
	}{
		{"read-only", true, 1, wantDenied(`read-only file system`)},
		{"read-write", false, 0, wantOK(``)},
	} {
		s := Restricted(lesson, tc.name, img, "write", "/work")
		s.Mounts = append(s.Mounts, Mount{Type: MountVolume, Source: vol, Target: "/work", ReadOnly: tc.readOnly})
		res, lines := runLines(t, ctx, cli, s)
		l, found := probeout.Find(lines, "write")
		if !found {
			t.Fatalf("%s: no write line in %q", tc.name, res.Stdout)
		}
		if res.ExitCode != tc.exit {
			t.Errorf("%s: exit %d, want %d", tc.name, res.ExitCode, tc.exit)
		}
		if problem := tc.line.check(l); problem != "" {
			t.Errorf("%s: %s", tc.name, problem)
		}
	}
}

// TestSpec_H4_GatedPIDNamespaceSharing: containers with private PID
// namespaces cannot see each other's processes; a container started with
// PIDMode "container:<id>" shares the namespace and both sides see all.
func TestSpec_H4_GatedPIDNamespaceSharing(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h4-pidns")

	start := func(role, pidMode string) *Container {
		t.Helper()
		s := Restricted(lesson, role, img, "sleep")
		s.PIDMode = pidMode
		c, err := Start(ctx, cli, s)
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		removeOnCleanup(t, c) // last started is removed first, so the joiner goes before its host
		return c
	}
	// visible counts the PIDs an exec'd probe sees: the container's own init
	// and the probe itself make two, anything more belongs to a namespace-mate.
	visible := func(c *Container) int {
		t.Helper()
		res, err := c.Exec(ctx, FixtureBinary, "pids")
		if err != nil {
			t.Fatal(err)
		}
		l, found := probeout.Find(probeout.Parse(res.Stdout), "pids")
		if !found {
			t.Fatalf("%s: no pids line in %q", c.Name, res.Stdout)
		}
		return len(strings.Fields(l.Detail))
	}

	a, b := start("a", ""), start("b", "")
	if na, nb := visible(a), visible(b); na != 2 || nb != 2 {
		t.Errorf("private PID namespaces: a sees %d processes, b sees %d, want 2 each (init and the probe)", na, nb)
	}
	j := start("joined", "container:"+a.ID)
	if na, nj := visible(a), visible(j); na != 3 || nj != 3 {
		t.Errorf("container:<a> pair: a sees %d, joined sees %d, want 3 each (a's init, joined's init, the probe)", na, nj)
	}
	if nb := visible(b); nb != 2 {
		t.Errorf("b still private: sees %d processes, want 2", nb)
	}
}

// TestSpec_H4_GatedOOMKilledAtMemoryLimit: touching more memory than the
// limit ends the container with OOMKilled true and exit 137 (128+SIGKILL); an
// allocation under the limit does not.
func TestSpec_H4_GatedOOMKilledAtMemoryLimit(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h4-oom")

	under, err := Run(ctx, cli, Restricted(lesson, "under", img, "alloc", strconv.Itoa(16<<20)))
	if err != nil {
		t.Fatal(err)
	}
	if under.ExitCode != 0 || under.OOMKilled {
		t.Errorf("16 MiB under a 64 MiB limit: exit %d OOMKilled %v", under.ExitCode, under.OOMKilled)
	}
	over, err := Run(ctx, cli, Restricted(lesson, "over", img, "alloc", strconv.Itoa(2*RestrictedMemory)))
	if err != nil {
		t.Fatal(err)
	}
	if !over.OOMKilled || over.ExitCode != 137 {
		t.Errorf("128 MiB over a 64 MiB limit: exit %d OOMKilled %v, want 137 and true (stdout %q)", over.ExitCode, over.OOMKilled, over.Stdout)
	}
}

// TestSpec_H4_GatedForkLoopStopsAtPidsLimit: PidsLimit counts threads, and
// every Go child has several, so the loop stops well below PidsLimit
// children. It must stop with EAGAIN, the pids cgroup's answer, not ENOMEM or
// anything else; a control with a higher limit forks without trouble.
func TestSpec_H4_GatedForkLoopStopsAtPidsLimit(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h4-fork")

	res, lines := runLines(t, ctx, cli, Restricted(lesson, "fork", img, "fork", "200"))
	if res.ExitCode != 1 {
		t.Errorf("fork 200 under PidsLimit %d: exit %d, want 1: %q", RestrictedPidsLimit, res.ExitCode, probeout.Format(lines))
	}
	started, found := probeout.Find(lines, "fork-started")
	n, err := strconv.Atoi(started.Detail)
	if !found || !started.OK || err != nil || n < 1 || n >= RestrictedPidsLimit {
		t.Errorf("fork-started = %+v, want a count in 1..%d", started, RestrictedPidsLimit-1)
	}
	if problem := wantDenied(`resource temporarily unavailable`).check(lineOf(lines, "fork")); problem != "" {
		t.Errorf("fork line: %s", problem)
	}
	t.Logf("%d children started under PidsLimit %d", n, RestrictedPidsLimit)

	// Control: the same spec with room for the children runs them all.
	ctl := Restricted(lesson, "control", img, "fork", "20")
	ctl.PidsLimit = 512
	ctl.Memory = 256 << 20 // twenty sleeping copies of the probe must not hit the 64 MiB limit instead
	res, lines = runLines(t, ctx, cli, ctl)
	if started, _ := probeout.Find(lines, "fork-started"); res.ExitCode != 0 || started.Detail != "20" || res.OOMKilled {
		t.Errorf("fork 20 under PidsLimit 512: exit %d OOMKilled %v lines %q, want 0 and 20 children", res.ExitCode, res.OOMKilled, probeout.Format(lines))
	}
}

// lineOf returns the named line; a missing one is a zero Line, which fails any
// want that needs a particular answer.
func lineOf(lines []probeout.Line, name string) probeout.Line {
	l, _ := probeout.Find(lines, name)
	return l
}

// TestSpec_H4_GatedCgroupFilesReadableInside: the container reads its own
// limits from /sys/fs/cgroup, so a lesson can report usage and limits from
// inside. The values are the Spec constants, not copies of them.
func TestSpec_H4_GatedCgroupFilesReadableInside(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h4-cgroup")

	res, lines := runLines(t, ctx, cli, Restricted(lesson, "cgroup", img, "cgroup"))
	if res.ExitCode != 0 {
		t.Fatalf("cgroup exit %d: %q %s", res.ExitCode, probeout.Format(lines), res.Stderr)
	}
	period := 100_000
	wants := map[string]string{
		"memory.max": strconv.Itoa(RestrictedMemory),
		"pids.max":   strconv.Itoa(RestrictedPidsLimit),
		"cpu.max":    strconv.Itoa(RestrictedNanoCPUs*period/1_000_000_000) + " " + strconv.Itoa(period),
	}
	for name, v := range wants {
		if l, found := probeout.Find(lines, name); !found || !l.OK || l.Detail != v {
			t.Errorf("%s = %+v, want OK %q", name, l, v)
		}
	}
}
