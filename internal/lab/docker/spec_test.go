package docker

import (
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"pgregory.net/rapid"
)

// Daemon-free tests for the launcher's pure half: Spec, Restricted and
// Translate. The gated half is in run_gated_test.go.

func int64p(n int64) *int64 { return &n }

// harnessLabels is Labels written out, so a change to Labels shows here.
func harnessLabels(lesson string) map[string]string {
	return map[string]string{"trinkets.harness": "1", "trinkets.lesson": lesson}
}

// defaultHost is what every translated HostConfig carries even for a zero
// Spec: private IPC and cgroup namespaces, the engine defaults on cgroup v2.
func defaultHost() container.HostConfig {
	return container.HostConfig{IpcMode: "private", CgroupnsMode: "private"}
}

func TestSpec_H3_TranslatesSpecToEngineConfig(t *testing.T) {
	restrictedHC := defaultHost()
	restrictedHC.NetworkMode = "none"
	restrictedHC.ReadonlyRootfs = true
	restrictedHC.CapDrop = []string{"ALL"}
	restrictedHC.SecurityOpt = []string{"no-new-privileges:true"}
	restrictedHC.Runtime = "runc"
	restrictedHC.Mounts = []mount.Mount{{Type: "tmpfs", Target: "/tmp", TmpfsOptions: &mount.TmpfsOptions{SizeBytes: 16 << 20}}}
	// 64 MiB = 67108864 bytes, swap pinned to it; 0.5 CPU = 5e8 nano-CPUs.
	restrictedHC.Resources = container.Resources{Memory: 67108864, MemorySwap: 67108864, NanoCPUs: 500000000, PidsLimit: int64p(64)}

	withHost := func(f func(*container.HostConfig)) container.HostConfig {
		hc := defaultHost()
		f(&hc)
		return hc
	}
	tests := []struct {
		name string
		spec Spec
		want Request
	}{
		{
			name: "zero spec is Docker's default container",
			spec: Spec{Lesson: "les", Role: "worker", Image: "img:dev"},
			want: Request{
				Name:       "trinkets-les-worker-abc123",
				Config:     container.Config{Image: "img:dev", Labels: harnessLabels("les")},
				HostConfig: defaultHost(),
			},
		},
		{
			name: "restricted preset",
			spec: Restricted("les", "worker", "img:dev", "sleep", "5"),
			want: Request{
				Name:       "trinkets-les-worker-abc123",
				Config:     container.Config{Image: "img:dev", Cmd: []string{"sleep", "5"}, User: "10001:10001", Labels: harnessLabels("les")},
				HostConfig: restrictedHC,
			},
		},
		{
			name: "command, env, user and groups pass through",
			spec: Spec{Lesson: "les", Role: "r", Image: "i", Cmd: []string{"id"}, Env: []string{"A=1", "B="}, User: "20001:20001", Groups: []string{"30000", "0"}},
			want: Request{
				Name:       "trinkets-les-r-abc123",
				Config:     container.Config{Image: "i", Cmd: []string{"id"}, Env: []string{"A=1", "B="}, User: "20001:20001", Labels: harnessLabels("les")},
				HostConfig: withHost(func(hc *container.HostConfig) { hc.GroupAdd = []string{"30000", "0"} }),
			},
		},
		{
			name: "capabilities normalized like the client: upper case, CAP_ prefix, sorted, deduplicated",
			spec: Spec{Lesson: "les", Role: "r", Image: "i", CapDrop: []string{"net_raw", "all", "CAP_NET_RAW"}, CapAdd: []string{"FOWNER", "chown"}},
			want: Request{
				Name:   "trinkets-les-r-abc123",
				Config: container.Config{Image: "i", Labels: harnessLabels("les")},
				HostConfig: withHost(func(hc *container.HostConfig) {
					hc.CapDrop = []string{"ALL", "CAP_NET_RAW"}     // "ALL" < "CAP_..." ('A' < 'C')
					hc.CapAdd = []string{"CAP_CHOWN", "CAP_FOWNER"} // sorted
				}),
			},
		},
		{
			name: "seccomp JSON is compacted after no-new-privileges",
			spec: Spec{Lesson: "les", Role: "r", Image: "i", NoNewPrivileges: true, Seccomp: " {\n  \"defaultAction\": \"SCMP_ACT_ERRNO\"\n} "},
			want: Request{
				Name:   "trinkets-les-r-abc123",
				Config: container.Config{Image: "i", Labels: harnessLabels("les")},
				HostConfig: withHost(func(hc *container.HostConfig) {
					hc.SecurityOpt = []string{"no-new-privileges:true", `seccomp={"defaultAction":"SCMP_ACT_ERRNO"}`}
				}),
			},
		},
		{
			name: "seccomp unconfined",
			spec: Spec{Lesson: "les", Role: "r", Image: "i", Seccomp: "unconfined"},
			want: Request{
				Name:       "trinkets-les-r-abc123",
				Config:     container.Config{Image: "i", Labels: harnessLabels("les")},
				HostConfig: withHost(func(hc *container.HostConfig) { hc.SecurityOpt = []string{"seccomp=unconfined"} }),
			},
		},
		{
			name: "limits: swap pinned to memory, PID limit set",
			spec: Spec{Lesson: "les", Role: "r", Image: "i", Memory: 6 << 20, NanoCPUs: 1, PidsLimit: 7},
			want: Request{
				Name:   "trinkets-les-r-abc123",
				Config: container.Config{Image: "i", Labels: harnessLabels("les")},
				HostConfig: withHost(func(hc *container.HostConfig) {
					hc.Resources = container.Resources{Memory: 6291456, MemorySwap: 6291456, NanoCPUs: 1, PidsLimit: int64p(7)}
				}),
			},
		},
		{
			name: "mounts: labelled volume, read-only flag, tmpfs size",
			spec: Spec{Lesson: "les", Role: "r", Image: "i", Mounts: []Mount{
				{Type: MountVolume, Source: "trinkets-les-sock-1", Target: "/work", ReadOnly: true},
				{Type: MountTmpfs, Target: "/scratch"},
				{Type: MountTmpfs, Target: "/tmp", TmpfsSize: 1024},
			}},
			want: Request{
				Name:   "trinkets-les-r-abc123",
				Config: container.Config{Image: "i", Labels: harnessLabels("les")},
				HostConfig: withHost(func(hc *container.HostConfig) {
					hc.Mounts = []mount.Mount{
						{Type: "volume", Source: "trinkets-les-sock-1", Target: "/work", ReadOnly: true,
							VolumeOptions: &mount.VolumeOptions{Labels: harnessLabels("les")}},
						{Type: "tmpfs", Target: "/scratch"},
						{Type: "tmpfs", Target: "/tmp", TmpfsOptions: &mount.TmpfsOptions{SizeBytes: 1024}},
					}
				}),
			},
		},
		{
			name: "network name and shared PID namespace",
			spec: Spec{Lesson: "les", Role: "r", Image: "i", Network: "trinkets-les-net", PIDMode: "container:trinkets-les-broker-1"},
			want: Request{
				Name:   "trinkets-les-r-abc123",
				Config: container.Config{Image: "i", Labels: harnessLabels("les")},
				HostConfig: withHost(func(hc *container.HostConfig) {
					hc.NetworkMode = "trinkets-les-net"
					hc.PidMode = "container:trinkets-les-broker-1"
				}),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Translate(tc.spec, "/var/run/docker.sock", "abc123")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Translate:\n got %+v\n%+v\nwant %+v\n%+v", got.Config, got.HostConfig, tc.want.Config, tc.want.HostConfig)
			}
			if bad := CheckConfig(got, "/var/run/docker.sock"); len(bad) > 0 {
				t.Errorf("CheckConfig: %v", bad)
			}
		})
	}
}

func TestSpec_H3_TranslateRejectsInvalidSpecs(t *testing.T) {
	ok := Spec{Lesson: "les", Role: "r", Image: "i"}
	with := func(f func(*Spec)) Spec { s := ok; f(&s); return s }
	vol := func(src, target string) Mount { return Mount{Type: MountVolume, Source: src, Target: target} }
	tests := []struct {
		name, suffix string
		spec         Spec
		want         string
	}{
		{"empty lesson", "abc", with(func(s *Spec) { s.Lesson = "" }), "lesson"},
		{"upper-case lesson", "abc", with(func(s *Spec) { s.Lesson = "Les" }), "lesson"},
		{"underscore role", "abc", with(func(s *Spec) { s.Role = "a_b" }), "role"},
		{"double hyphen role", "abc", with(func(s *Spec) { s.Role = "a--b" }), "role"},
		{"empty suffix", "", ok, "suffix"},
		{"upper-case suffix", "ABC", ok, "suffix"},
		{"empty image", "abc", with(func(s *Spec) { s.Image = "" }), "image"},
		{"user name", "abc", with(func(s *Spec) { s.User = "root" }), "user"},
		{"user gid name", "abc", with(func(s *Spec) { s.User = "1:wheel" }), "user"},
		{"user out of range", "abc", with(func(s *Spec) { s.User = "4294967295" }), "user"},
		{"group name", "abc", with(func(s *Spec) { s.Groups = []string{"docker"} }), "group"},
		{"bind type", "abc", with(func(s *Spec) { s.Mounts = []Mount{{Type: "bind", Source: "/etc", Target: "/etc"}} }), "type"},
		{"empty type", "abc", with(func(s *Spec) { s.Mounts = []Mount{{Source: "v1", Target: "/v"}} }), "type"},
		{"anonymous volume", "abc", with(func(s *Spec) { s.Mounts = []Mount{vol("", "/v")} }), "volume name"},
		{"volume source is a path", "abc", with(func(s *Spec) { s.Mounts = []Mount{vol("/etc", "/v")} }), "volume name"},
		{"one-character volume name", "abc", with(func(s *Spec) { s.Mounts = []Mount{vol("v", "/v")} }), "volume name"},
		{"tmpfs with a source", "abc", with(func(s *Spec) { s.Mounts = []Mount{{Type: MountTmpfs, Source: "x", Target: "/t"}} }), "no source"},
		{"tmpfs size on a volume", "abc", with(func(s *Spec) { s.Mounts = []Mount{{Type: MountVolume, Source: "v1", Target: "/t", TmpfsSize: 1}} }), "TmpfsSize"},
		{"negative tmpfs size", "abc", with(func(s *Spec) { s.Mounts = []Mount{{Type: MountTmpfs, Target: "/t", TmpfsSize: -1}} }), "negative"},
		{"relative target", "abc", with(func(s *Spec) { s.Mounts = []Mount{vol("v1", "work")} }), "absolute"},
		{"root target", "abc", with(func(s *Spec) { s.Mounts = []Mount{vol("v1", "/")} }), "absolute"},
		{"unclean target", "abc", with(func(s *Spec) { s.Mounts = []Mount{vol("v1", "/a/../work")} }), "not clean"},
		{"trailing slash target", "abc", with(func(s *Spec) { s.Mounts = []Mount{vol("v1", "/work/")} }), "not clean"},
		{"duplicate target", "abc", with(func(s *Spec) { s.Mounts = []Mount{vol("v1", "/w"), {Type: MountTmpfs, Target: "/w"}} }), "twice"},
		{"host network", "abc", with(func(s *Spec) { s.Network = "host" }), "host"},
		{"network with a space", "abc", with(func(s *Spec) { s.Network = "my net" }), "network"},
		{"network container mode", "abc", with(func(s *Spec) { s.Network = "container:x" }), "network"},
		{"host PID mode", "abc", with(func(s *Spec) { s.PIDMode = "host" }), "PID mode"},
		{"empty container PID mode", "abc", with(func(s *Spec) { s.PIDMode = "container:" }), "PID mode"},
		{"capability with a space", "abc", with(func(s *Spec) { s.CapAdd = []string{"NET RAW"} }), "capability"},
		{"empty capability", "abc", with(func(s *Spec) { s.CapDrop = []string{""} }), "capability"},
		{"negative memory", "abc", with(func(s *Spec) { s.Memory = -1 }), "negative"},
		{"negative CPUs", "abc", with(func(s *Spec) { s.NanoCPUs = -1 }), "negative"},
		{"negative PID limit", "abc", with(func(s *Spec) { s.PidsLimit = -1 }), "negative"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Translate(tc.spec, "", tc.suffix)
			if err == nil {
				t.Fatalf("Translate accepted it: %+v", got)
			}
			if !errors.Is(err, ErrInvalidSpec) || errors.Is(err, ErrDockerSocket) || errors.Is(err, ErrSeccompPath) {
				t.Errorf("error %v: want ErrInvalidSpec only", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
			if !reflect.DeepEqual(got, Request{}) {
				t.Errorf("a failed Translate returned a non-zero request: %+v", got)
			}
		})
	}
}

func TestSpec_H3_RejectsDockerSocketMount(t *testing.T) {
	const desktop = "/Users/me/.docker/run/docker.sock" // a Docker Desktop style resolved socket
	sockets := []struct {
		name, source, socket string
	}{
		{"/var/run/docker.sock", "/var/run/docker.sock", ""},
		{"/run/docker.sock", "/run/docker.sock", ""},
		{"resolved socket", desktop, desktop},
		{"resolved socket from Host()", desktop, SocketPath("unix://" + desktop)},
		{"dot-dot that cleans to /run/docker.sock", "/var/run/../run/docker.sock", ""},
		{"doubled leading slash", "//var/run/docker.sock", ""},
		{"doubled inner slash", "/run//docker.sock", ""},
		{"trailing slash", "/var/run/docker.sock/", ""},
		{"dot segment", "/var/./run/docker.sock", ""},
		{"climb above root", "/../run/docker.sock", ""},
		{"resolved socket, unclean", "/Users/me/.docker/x/../run//docker.sock/", desktop},
		{"fixed path while the resolved one differs", "/var/run/docker.sock", desktop},
	}
	for _, tc := range sockets {
		for _, typ := range []MountType{MountVolume, MountTmpfs, "bind"} {
			t.Run(tc.name+"/"+string(typ), func(t *testing.T) {
				s := Spec{Lesson: "les", Role: "r", Image: "i", Mounts: []Mount{{Type: typ, Source: tc.source, Target: "/var/run/docker.sock"}}}
				got, err := Translate(s, tc.socket, "abc")
				if !errors.Is(err, ErrDockerSocket) {
					t.Fatalf("Translate(%q, socket %q) = %v, want ErrDockerSocket", tc.source, tc.socket, err)
				}
				if !reflect.DeepEqual(got, Request{}) {
					t.Errorf("non-zero request on error: %+v", got)
				}
			})
		}
	}

	// Near misses are not the socket. An exact rule accepts the volume
	// names and rejects the paths only as bad volume names.
	nearMisses := []struct {
		source, socket string
		valid          bool
	}{
		{"0docker.sock", "", true},
		{"docker.sock", "", true},
		{"var.run.docker.sock", "", true},
		{"/var/run/docker.sock0", "", false},
		{"/var/run/docker.so", "", false},
		{"/var/run/docker.sock/..", "", false}, // cleans to /var/run
		{"/var/run/../docker.sock", "", false}, // cleans to /docker.sock
		{"var/run/docker.sock", "", false},     // relative
		{desktop, "", false},                   // only the resolved socket is special
		{"/var/run/docker.sock.d/x", "", false},
	}
	for _, tc := range nearMisses {
		s := Spec{Lesson: "les", Role: "r", Image: "i", Mounts: []Mount{{Type: MountVolume, Source: tc.source, Target: "/v"}}}
		_, err := Translate(s, tc.socket, "abc")
		if errors.Is(err, ErrDockerSocket) {
			t.Errorf("%q is not the socket but was rejected as one: %v", tc.source, err)
		}
		if (err == nil) != tc.valid {
			t.Errorf("Translate(volume %q) error = %v, want valid=%v", tc.source, err, tc.valid)
		}
	}
}

func TestSocketPath(t *testing.T) {
	for host, want := range map[string]string{
		"unix:///var/run/docker.sock":              "/var/run/docker.sock",
		"unix:///Users/me/.docker/run/docker.sock": "/Users/me/.docker/run/docker.sock",
		"tcp://127.0.0.1:2375":                     "",
		"npipe:////./pipe/docker_engine":           "",
		"ssh://me@host":                            "",
		"not a url":                                "",
		"":                                         "",
	} {
		if got := SocketPath(host); got != want {
			t.Errorf("SocketPath(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestSpec_H3_RejectsPathLikeSeccomp(t *testing.T) {
	for _, s := range []string{
		"/etc/docker/seccomp.json", "seccomp.json", "./profile.json", "~/profile.json", "builtin", "default",
		"Unconfined", "unconfined ", " unconfined", "[]", "null", "true", "7", `"{}"`, "{", "{} {}", `{"a":1}x`, "{\"a\":\"\xff\"}",
	} {
		_, err := Translate(Spec{Lesson: "les", Role: "r", Image: "i", Seccomp: s}, "", "abc")
		if !errors.Is(err, ErrSeccompPath) {
			t.Errorf("Seccomp %q: error %v, want ErrSeccompPath", s, err)
		}
	}
	for _, s := range []string{"", "unconfined", "{}", " {\n\t\"defaultAction\": \"SCMP_ACT_ALLOW\", \"syscalls\": [] }\n"} {
		if _, err := Translate(Spec{Lesson: "les", Role: "r", Image: "i", Seccomp: s}, "", "abc"); err != nil {
			t.Errorf("Seccomp %q: %v", s, err)
		}
	}
}

func TestSpec_H3_PassesRuntimeThroughUntouched(t *testing.T) {
	for _, rt := range []string{"", "runc", "runsc", "no-such-runtime", "io.containerd.runc.v2", "Odd Name/../x", " runc "} {
		got, err := Translate(Spec{Lesson: "les", Role: "r", Image: "i", Runtime: rt}, "", "abc")
		if err != nil {
			t.Fatalf("Runtime %q: %v", rt, err)
		}
		if got.HostConfig.Runtime != rt {
			t.Errorf("Runtime %q came out as %q", rt, got.HostConfig.Runtime)
		}
	}
	if got := Restricted("les", "r", "i").Runtime; got != "runc" {
		t.Errorf("Restricted runtime = %q, want runc", got)
	}
}

// ---- properties ----------------------------------------------------------

// The generators below build each field as valid or broken by construction
// and record which, so the oracle never calls the code under test.

// specCase is a generated Spec with what the oracle expects of it.
type specCase struct {
	spec    Spec
	socket  string
	suffix  string
	invalid bool // any field broken
	sock    bool // some mount source cleans to a socket path
	seccomp bool // Seccomp is neither "", "unconfined" nor a JSON object
}

var validNameGen = rapid.StringMatching(`[a-z0-9]{1,6}(-[a-z0-9]{1,6}){0,2}`)

// drawOr draws from good, or with probability 1/n from bad, and marks the
// case invalid when it took a bad value.
func drawOr[V any](t *rapid.T, c *specCase, label string, n int, good, bad *rapid.Generator[V]) V {
	if rapid.IntRange(0, n-1).Draw(t, label+" broken?") == 0 {
		c.invalid = true
		return bad.Draw(t, label+" (bad)")
	}
	return good.Draw(t, label)
}

// socketSpelling returns one of the socket paths spelled so that path.Clean
// still gives the socket: doubled slashes, a trailing slash, "." and
// "x/.." segments, a climb above the root.
func socketSpelling(t *rapid.T, sockets []string) string {
	p := rapid.SampledFrom(sockets).Draw(t, "socket")
	for i, n := 0, rapid.IntRange(0, 3).Draw(t, "spelling changes"); i < n; i++ {
		switch rapid.IntRange(0, 4).Draw(t, "spelling") {
		case 0:
			p = "/" + p
		case 1:
			p += "/"
		case 2, 3:
			// replace one "/" with "/./" or "/zz/../"
			var slashes []int
			for j := range len(p) {
				if p[j] == '/' {
					slashes = append(slashes, j)
				}
			}
			j := rapid.SampledFrom(slashes).Draw(t, "slash")
			p = p[:j] + rapid.SampledFrom([]string{"/./", "/zz/../"}).Draw(t, "segment") + p[j+1:]
		case 4:
			p = "/.." + p
		}
	}
	return p
}

// genSource draws a mount source and says whether it is a socket spelling
// and whether it is a valid volume name.
func genSource(t *rapid.T, socket string) (src string, sock, volumeName bool) {
	sockets := []string{"/var/run/docker.sock", "/run/docker.sock"}
	if socket != "" {
		sockets = append(sockets, socket)
	}
	switch rapid.IntRange(0, 5).Draw(t, "source kind") {
	case 0, 1:
		return rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9_.-]{1,10}`).Draw(t, "volume name"), false, true
	case 2:
		return rapid.SampledFrom([]string{"0docker.sock", "docker.sock", "run-docker.sock", "var.run.docker.sock"}).Draw(t, "near-miss name"), false, true
	case 3:
		return socketSpelling(t, sockets), true, false
	case 4:
		return rapid.SampledFrom([]string{
			"/var/run/docker.sock0", "/var/run/docker.so", "/var/run/docker.sock/..", "/var/run/../docker.sock",
			"/run/docker.sock.d", "/var/run/docker", "var/run/docker.sock", "/var/run/docker.sock/x", "..", "/", "a/b", "/x//y/",
			"/Users/someone/.docker/run/docker.sock",
		}).Draw(t, "near-miss path"), false, false
	}
	return "", false, false
}

func genMount(t *rapid.T, c *specCase) Mount {
	var m Mount
	switch rapid.IntRange(0, 6).Draw(t, "mount type") {
	case 0, 1, 2:
		m.Type = MountVolume
		var sock, name bool
		m.Source, sock, name = genSource(t, c.socket)
		c.sock = c.sock || sock
		c.invalid = c.invalid || !name
		if rapid.IntRange(0, 9).Draw(t, "volume size?") == 0 {
			m.TmpfsSize, c.invalid = 1, true
		}
	case 3, 4, 5:
		m.Type = MountTmpfs
		if rapid.IntRange(0, 3).Draw(t, "tmpfs source?") == 0 {
			var sock bool
			m.Source, sock, _ = genSource(t, c.socket)
			c.sock = c.sock || sock
			c.invalid = c.invalid || m.Source != ""
		}
		m.TmpfsSize = drawOr(t, c, "tmpfs size", 10, rapid.Int64Range(0, 1<<30), rapid.Int64Range(-5, -1))
	default:
		m.Type = rapid.SampledFrom([]MountType{"bind", "", "Volume", "npipe"}).Draw(t, "bad type")
		var sock bool
		m.Source, sock, _ = genSource(t, c.socket)
		c.sock = c.sock || sock
		c.invalid = true
	}
	m.Target = drawOr(t, c, "target", 8, rapid.StringMatching(`/[a-z]{1,3}(/[a-z]{1,3})?`),
		rapid.SampledFrom([]string{"", "/", "work", "./w", "/w/", "/a/../w", "/a//w", "/a/.", "/w\x00"}))
	m.ReadOnly = rapid.Bool().Draw(t, "read-only")
	return m
}

func genSeccomp(t *rapid.T, c *specCase) string {
	if rapid.IntRange(0, 2).Draw(t, "seccomp bad?") == 0 {
		c.invalid, c.seccomp = true, true
		return rapid.OneOf(
			rapid.SampledFrom([]string{"/etc/seccomp.json", "profile.json", "builtin", "Unconfined", "unconfined ", "[]", "null", "{", "{}{}", `"x"`, "{\"a\":\"\xff\"}"}),
			rapid.StringMatching(`[a-z/._-]{1,12}`).Filter(func(s string) bool { return s != "unconfined" }),
		).Draw(t, "bad seccomp")
	}
	return rapid.SampledFrom([]string{"", "unconfined", "{}", `{"defaultAction":"SCMP_ACT_ALLOW"}`, " {\n \"defaultAction\" : \"SCMP_ACT_ERRNO\", \"syscalls\": [ ] }\t"}).Draw(t, "seccomp")
}

func genSpecCase(t *rapid.T) specCase {
	c := specCase{socket: rapid.SampledFrom([]string{"", "/var/run/docker.sock", "/Users/me/.docker/run/docker.sock", "/run/user/1000/docker.sock"}).Draw(t, "socket")}
	c.suffix = drawOr(t, &c, "suffix", 12, rapid.StringMatching(`[a-z0-9]{1,8}`), rapid.SampledFrom([]string{"", "ABC", "a-b", "x y", "é"}))
	badName := rapid.SampledFrom([]string{"", "A", "a b", "a_b", "-a", "a-", "a--b", "a/b", "a:b", "é"})
	s := &c.spec
	s.Lesson = drawOr(t, &c, "lesson", 12, validNameGen, badName)
	s.Role = drawOr(t, &c, "role", 12, validNameGen, badName)
	s.Image = drawOr(t, &c, "image", 15, rapid.SampledFrom([]string{"img", "trinkets-x-probe:dev", "busybox@sha256:abc"}), rapid.Just(""))
	s.Cmd = rapid.SliceOfN(rapid.String(), 0, 3).Draw(t, "cmd")
	s.Env = rapid.SliceOfN(rapid.String(), 0, 2).Draw(t, "env")
	s.User = drawOr(t, &c, "user", 10,
		rapid.SampledFrom([]string{"", "0", "0:0", "10001:10001", "20000:30000", "4294967294:1"}),
		rapid.SampledFrom([]string{"root", "1:", ":1", "-1", "1:2:3", "4294967295", "1:x", " 1"}))
	s.Groups = rapid.SliceOfN(drawGen(&c, "group", 10, rapid.SampledFrom([]string{"0", "30000", "4294967294"}), rapid.SampledFrom([]string{"wheel", "", "-1", "1 "})), 0, 2).Draw(t, "groups")
	for i, n := 0, rapid.IntRange(0, 4).Draw(t, "mounts"); i < n; i++ {
		s.Mounts = append(s.Mounts, genMount(t, &c))
	}
	for i := range s.Mounts {
		for j := range i {
			if s.Mounts[i].Target == s.Mounts[j].Target {
				c.invalid = true // a target used twice
			}
		}
	}
	s.Network = drawOr(t, &c, "network", 10, rapid.SampledFrom([]string{"", "none", "bridge", "trinkets-x-net", "n_1.x"}),
		rapid.SampledFrom([]string{"host", "a b", "container:x", "-x", "/x", "é"}))
	s.ReadOnlyRoot = rapid.Bool().Draw(t, "read-only root")
	capGood := rapid.SampledFrom([]string{"ALL", "all", "NET_RAW", "cap_chown", "CAP_SYS_ADMIN"})
	capBad := rapid.SampledFrom([]string{"", "NET RAW", "CAP-X", "é"})
	s.CapDrop = rapid.SliceOfN(drawGen(&c, "cap drop", 12, capGood, capBad), 0, 3).Draw(t, "cap drop")
	s.CapAdd = rapid.SliceOfN(drawGen(&c, "cap add", 12, capGood, capBad), 0, 2).Draw(t, "cap add")
	s.NoNewPrivileges = rapid.Bool().Draw(t, "no new privileges")
	s.Seccomp = genSeccomp(t, &c)
	s.PIDMode = drawOr(t, &c, "pid mode", 10, rapid.SampledFrom([]string{"", "container:abc", "container:trinkets-x-y-1"}),
		rapid.SampledFrom([]string{"host", "container:", "private", "container:a b"}))
	limit := func(label string) int64 {
		return drawOr(t, &c, label, 15, rapid.Int64Range(0, 1<<40), rapid.Int64Range(-1<<40, -1))
	}
	s.Memory, s.NanoCPUs, s.PidsLimit = limit("memory"), limit("cpus"), limit("pids")
	s.Runtime = rapid.OneOf(rapid.SampledFrom([]string{"", "runc", "runsc", "no-such-runtime"}), rapid.String()).Draw(t, "runtime")
	return c
}

// drawGen is drawOr as a generator, for slices of possibly bad elements.
func drawGen[V any](c *specCase, label string, n int, good, bad *rapid.Generator[V]) *rapid.Generator[V] {
	return rapid.Custom(func(t *rapid.T) V { return drawOr(t, c, label, n, good, bad) })
}

// propTranslate: Translate succeeds exactly when every field was generated
// valid; it fails with ErrDockerSocket exactly when a mount source is a
// socket spelling and with ErrSeccompPath exactly when Seccomp is bad. A
// success passes CheckConfig and keeps Runtime; a failure returns the zero
// request; two calls agree.
func propTranslate(t *rapid.T) {
	c := genSpecCase(t)
	got, err := Translate(c.spec, c.socket, c.suffix)
	again, err2 := Translate(c.spec, c.socket, c.suffix)
	if !reflect.DeepEqual(got, again) || errString(err) != errString(err2) {
		t.Fatalf("two calls differ:\n%+v %v\n%+v %v", got, err, again, err2)
	}
	if wantErr := c.invalid || c.sock || c.seccomp; (err != nil) != wantErr {
		t.Fatalf("Translate error = %v, want an error: %v (invalid=%v sock=%v seccomp=%v)", err, wantErr, c.invalid, c.sock, c.seccomp)
	}
	if errors.Is(err, ErrDockerSocket) != c.sock {
		t.Fatalf("ErrDockerSocket = %v, want %v: %v", errors.Is(err, ErrDockerSocket), c.sock, err)
	}
	if errors.Is(err, ErrSeccompPath) != c.seccomp {
		t.Fatalf("ErrSeccompPath = %v, want %v: %v", errors.Is(err, ErrSeccompPath), c.seccomp, err)
	}
	if err != nil {
		if !reflect.DeepEqual(got, Request{}) {
			t.Fatalf("failed Translate returned %+v", got)
		}
		return
	}
	if bad := CheckConfig(got, c.socket); len(bad) > 0 {
		t.Fatalf("CheckConfig on a translated spec: %v", bad)
	}
	if got.HostConfig.Runtime != c.spec.Runtime {
		t.Fatalf("Runtime %q became %q", c.spec.Runtime, got.HostConfig.Runtime)
	}
	if want := "trinkets-" + c.spec.Lesson + "-" + c.spec.Role + "-" + c.suffix; got.Name != want {
		t.Fatalf("name %q, want %q", got.Name, want)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestProp_Translate(t *testing.T) { rapid.Check(t, propTranslate) }
func FuzzTranslate(f *testing.F)      { f.Fuzz(rapid.MakeFuzz(propTranslate)) }

// nameOracle is the lesson and role rule, written apart from lessonRE:
// lower-case letters and digits in groups joined by single hyphens.
var nameOracle = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// propRestrictedPreset: Restricted never fails and its isolation fields do
// not depend on its arguments. Invalid names are not tidied: they pass
// through Restricted unchanged and Translate rejects them with
// ErrInvalidSpec. With valid names and an image, the translation satisfies
// restricted-complete and CheckConfig.
func propRestrictedPreset(t *rapid.T) {
	lesson := rapid.OneOf(validNameGen, rapid.String()).Draw(t, "lesson")
	role := rapid.OneOf(validNameGen, rapid.String()).Draw(t, "role")
	image := rapid.OneOf(rapid.Just(""), rapid.String()).Draw(t, "image")
	cmd := rapid.SliceOfN(rapid.String(), 0, 4).Draw(t, "cmd")
	s := Restricted(lesson, role, image, cmd...)
	if s.Lesson != lesson || s.Role != role || s.Image != image || !slices.Equal(s.Cmd, cmd) {
		t.Fatalf("Restricted changed its arguments: %+v", s)
	}
	if len(cmd) > 0 {
		cmd[0] += "changed"
		if s.Cmd[0] == cmd[0] {
			t.Fatal("Restricted shares the caller's cmd slice")
		}
	}
	base := Restricted("a", "b", "c")
	s2 := s
	s2.Lesson, s2.Role, s2.Image, s2.Cmd = "a", "b", "c", nil
	if !reflect.DeepEqual(s2, base) {
		t.Fatalf("isolation fields depend on the arguments:\n%+v\n%+v", s2, base)
	}
	req, err := Translate(s, "/var/run/docker.sock", "abc")
	valid := nameOracle.MatchString(lesson) && nameOracle.MatchString(role) && image != ""
	if !valid {
		if err == nil || !errors.Is(err, ErrInvalidSpec) || errors.Is(err, ErrDockerSocket) || errors.Is(err, ErrSeccompPath) {
			t.Fatalf("invalid names or image: error %v, want ErrInvalidSpec only", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if bad := restrictedComplete(req.Config, req.HostConfig); len(bad) > 0 || !IsRestricted(req.Config, req.HostConfig) {
		t.Fatalf("Restricted is not restricted: %v", bad)
	}
	if bad := CheckConfig(req, "/var/run/docker.sock"); len(bad) > 0 {
		t.Fatalf("CheckConfig: %v", bad)
	}
}

func TestProp_RestrictedPreset(t *testing.T) { rapid.Check(t, propRestrictedPreset) }
func FuzzRestrictedPreset(f *testing.F)      { f.Fuzz(rapid.MakeFuzz(propRestrictedPreset)) }

// propTighteningNeverLoosens: any sequence of tightening steps applied to a
// restricted spec leaves it restricted. A step drops one more capability,
// removes a mount, makes a mount read-only, or lowers a limit to a smaller
// positive value (0 would mean "no limit", which is not lower).
func propTighteningNeverLoosens(t *rapid.T) {
	s := Restricted(validNameGen.Draw(t, "lesson"), validNameGen.Draw(t, "role"), "img", "sleep")
	if rapid.Bool().Draw(t, "with a volume") {
		s.Mounts = append(s.Mounts, Mount{Type: MountVolume, Source: "trinkets-x-work-1", Target: "/work"})
	}
	lower := func(label string, v int64) int64 { return rapid.Int64Range(1, v).Draw(t, label) }
	for i, n := 0, rapid.IntRange(1, 8).Draw(t, "steps"); i < n; i++ {
		switch rapid.IntRange(0, 5).Draw(t, "step") {
		case 0:
			s.CapDrop = append(s.CapDrop, rapid.SampledFrom([]string{"NET_RAW", "chown", "CAP_SETUID", "ALL"}).Draw(t, "drop"))
		case 1:
			if len(s.Mounts) > 0 {
				s.Mounts = slices.Delete(slices.Clone(s.Mounts), 0, 1)
			}
		case 2:
			if len(s.Mounts) > 0 {
				s.Mounts = slices.Clone(s.Mounts)
				s.Mounts[rapid.IntRange(0, len(s.Mounts)-1).Draw(t, "mount")].ReadOnly = true
			}
		case 3:
			s.Memory = lower("memory", s.Memory)
		case 4:
			s.NanoCPUs = lower("cpus", s.NanoCPUs)
		case 5:
			s.PidsLimit = lower("pids", s.PidsLimit)
		}
		req, err := Translate(s, "/var/run/docker.sock", "abc")
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if bad := restrictedComplete(req.Config, req.HostConfig); len(bad) > 0 {
			t.Fatalf("step %d loosened the spec: %v", i, bad)
		}
		if bad := CheckConfig(req, "/var/run/docker.sock"); len(bad) > 0 {
			t.Fatalf("step %d: CheckConfig: %v", i, bad)
		}
	}
}

func TestProp_TighteningNeverLoosens(t *testing.T) { rapid.Check(t, propTighteningNeverLoosens) }
func FuzzTighteningNeverLoosens(f *testing.F)      { f.Fuzz(rapid.MakeFuzz(propTighteningNeverLoosens)) }
