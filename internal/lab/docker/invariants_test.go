package docker

import (
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// Each invariant is checked against hand-built engine configs that break it
// in one way, so a check that stops looking at a field fails here. The
// configs bypass Translate on purpose: they are what a bug in Translate, or
// a caller building configs by hand, would send.

func goodRequest() Request {
	req, err := Translate(Restricted("les", "r", "img"), "/var/run/docker.sock", "abc")
	if err != nil {
		panic(err)
	}
	return req
}

// wantViolations checks that got holds exactly n violations, each starting
// with the invariant's name.
func wantViolations(t *testing.T, got []string, name string, n int) {
	t.Helper()
	if len(got) != n {
		t.Errorf("got %d violations %q, want %d", len(got), got, n)
	}
	for _, v := range got {
		if !strings.HasPrefix(v, name+": ") {
			t.Errorf("violation %q does not start with %q", v, name)
		}
	}
}

func TestInvariantsHoldForTranslatedRequests(t *testing.T) {
	if bad := CheckConfig(goodRequest(), "/var/run/docker.sock"); len(bad) > 0 {
		t.Errorf("restricted: %v", bad)
	}
	req, err := Translate(Spec{Lesson: "les", Role: "r", Image: "img"}, "", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if bad := CheckConfig(req, ""); len(bad) > 0 {
		t.Errorf("default: %v", bad)
	}
}

func TestInvariantNoDockerSocket(t *testing.T) {
	const desktop = "/Users/me/.docker/run/docker.sock"
	tests := []struct {
		name   string
		hc     container.HostConfig
		socket string
		n      int
	}{
		{"Binds entry", container.HostConfig{Binds: []string{"/tmp:/tmp"}}, "", 1},
		{"bind mount", container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/etc", Target: "/etc"}}}, "", 1},
		{"bind of the socket counts twice", container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/var/run/docker.sock", Target: "/s"}}}, "", 2},
		{"volume named like the socket", container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: "//run/docker.sock/", Target: "/s"}}}, "", 1},
		{"resolved socket", container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: desktop, Target: "/s"}}}, desktop, 1},
		{"resolved socket not given", container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: desktop, Target: "/s"}}}, "", 0},
		{"near-miss name", container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: "0docker.sock", Target: "/s"}}}, "", 0},
		{"socket as target only", container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeTmpfs, Target: "/var/run/docker.sock"}}}, "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { wantViolations(t, noDockerSocket(tc.hc, tc.socket), InvNoDockerSocket, tc.n) })
	}
}

func TestInvariantNeverPrivileged(t *testing.T) {
	wantViolations(t, neverPrivileged(container.HostConfig{Privileged: true}), InvNeverPrivileged, 1)
	all := container.HostConfig{PidMode: "host", NetworkMode: "host", IpcMode: "host", UTSMode: "host", UsernsMode: "host", CgroupnsMode: "host"}
	wantViolations(t, neverPrivileged(all), InvNeverPrivileged, 6)
	ok := container.HostConfig{PidMode: "container:x", NetworkMode: "hostile", IpcMode: "private", UTSMode: "", CgroupnsMode: "private"}
	wantViolations(t, neverPrivileged(ok), InvNeverPrivileged, 0)
}

func TestInvariantLabelled(t *testing.T) {
	cfg := func(labels map[string]string) container.Config { return container.Config{Labels: labels} }
	wantViolations(t, labelled("trinkets-les-r-1", cfg(Labels("les"))), InvLabelled, 0)
	wantViolations(t, labelled("trinkets-les-r-1", cfg(nil)), InvLabelled, 2)
	wantViolations(t, labelled("trinkets-les-r-1", cfg(map[string]string{LabelHarness: "true", LabelLesson: "les"})), InvLabelled, 1)
	wantViolations(t, labelled("trinkets-other-r-1", cfg(Labels("les"))), InvLabelled, 1)
	wantViolations(t, labelled("trinkets-lesson-r-1", cfg(Labels("les"))), InvLabelled, 1) // prefix must end at the hyphen
}

func TestInvariantSwapPinned(t *testing.T) {
	hc := func(mem, swap int64) container.HostConfig {
		return container.HostConfig{Resources: container.Resources{Memory: mem, MemorySwap: swap}}
	}
	wantViolations(t, swapPinned(hc(64, 64)), InvSwapPinned, 0)
	wantViolations(t, swapPinned(hc(0, 0)), InvSwapPinned, 0)
	wantViolations(t, swapPinned(hc(64, 0)), InvSwapPinned, 1)  // the engine would double it
	wantViolations(t, swapPinned(hc(64, -1)), InvSwapPinned, 1) // unlimited swap
}

func TestInvariantSeccompInline(t *testing.T) {
	hc := func(opts ...string) container.HostConfig { return container.HostConfig{SecurityOpt: opts} }
	wantViolations(t, seccompInline(hc("seccomp=unconfined", `seccomp={"defaultAction":"SCMP_ACT_ALLOW"}`, "no-new-privileges:true", "apparmor=x")), InvSeccompInline, 0)
	wantViolations(t, seccompInline(hc("seccomp=/etc/profile.json")), InvSeccompInline, 1)
	wantViolations(t, seccompInline(hc("seccomp:/etc/profile.json")), InvSeccompInline, 1) // legacy separator
	wantViolations(t, seccompInline(hc("seccomp=[]", "seccomp=")), InvSeccompInline, 2)
}

func TestInvariantRestrictedComplete(t *testing.T) {
	good := goodRequest()
	if !IsRestricted(good.Config, good.HostConfig) {
		t.Fatalf("restricted preset: %v", restrictedComplete(good.Config, good.HostConfig))
	}
	tests := []struct {
		name    string
		breakIt func(*container.Config, *container.HostConfig)
	}{
		{"root user", func(c *container.Config, _ *container.HostConfig) { c.User = "0:0" }},
		{"root user with zeros", func(c *container.Config, _ *container.HostConfig) { c.User = "000:10001" }},
		{"image user", func(c *container.Config, _ *container.HostConfig) { c.User = "" }},
		{"named user", func(c *container.Config, _ *container.HostConfig) { c.User = "nobody" }},
		{"CapDrop without ALL", func(_ *container.Config, h *container.HostConfig) { h.CapDrop = []string{"CAP_NET_RAW"} }},
		{"CapAdd", func(_ *container.Config, h *container.HostConfig) { h.CapAdd = []string{"CAP_CHOWN"} }},
		{"no no-new-privileges", func(_ *container.Config, h *container.HostConfig) { h.SecurityOpt = nil }},
		{"no-new-privileges false", func(_ *container.Config, h *container.HostConfig) {
			h.SecurityOpt = []string{"no-new-privileges:false"}
		}},
		{"writable root", func(_ *container.Config, h *container.HostConfig) { h.ReadonlyRootfs = false }},
		{"bridge network", func(_ *container.Config, h *container.HostConfig) { h.NetworkMode = "bridge" }},
		{"default network", func(_ *container.Config, h *container.HostConfig) { h.NetworkMode = "" }},
		{"shared PID", func(_ *container.Config, h *container.HostConfig) { h.PidMode = "container:x" }},
		{"shareable IPC", func(_ *container.Config, h *container.HostConfig) { h.IpcMode = "shareable" }},
		{"engine-default cgroupns", func(_ *container.Config, h *container.HostConfig) { h.CgroupnsMode = "" }},
		{"no memory limit", func(_ *container.Config, h *container.HostConfig) { h.Memory = 0 }},
		{"no CPU limit", func(_ *container.Config, h *container.HostConfig) { h.NanoCPUs = 0 }},
		{"no PID limit", func(_ *container.Config, h *container.HostConfig) { h.PidsLimit = nil }},
		{"zero PID limit", func(_ *container.Config, h *container.HostConfig) { zero := int64(0); h.PidsLimit = &zero }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := goodRequest()
			tc.breakIt(&req.Config, &req.HostConfig)
			wantViolations(t, restrictedComplete(req.Config, req.HostConfig), InvRestrictedComplete, 1)
			if IsRestricted(req.Config, req.HostConfig) {
				t.Error("IsRestricted still true")
			}
		})
	}
}

func TestCheckConfigCombinesTheInvariants(t *testing.T) {
	req := goodRequest()
	req.HostConfig.Privileged = true
	req.HostConfig.Binds = []string{"/var/run/docker.sock:/var/run/docker.sock"}
	req.HostConfig.MemorySwap = 0
	req.HostConfig.SecurityOpt = append(req.HostConfig.SecurityOpt, "seccomp=profile.json")
	req.Name = "evil"
	bad := CheckConfig(req, "/var/run/docker.sock")
	for _, name := range []string{InvNoDockerSocket, InvNeverPrivileged, InvLabelled, InvSwapPinned, InvSeccompInline} {
		if !strings.Contains(strings.Join(bad, "\n"), name+": ") {
			t.Errorf("CheckConfig misses %s: %q", name, bad)
		}
	}
}

// observed builds the inspect response an engine would return for req,
// filling in what the engine adds for a zero intent.
func observed(req Request) container.InspectResponse {
	cfg, hc := req.Config, req.HostConfig
	cfg.Labels = map[string]string{"from.image": "x"}
	for k, v := range req.Config.Labels {
		cfg.Labels[k] = v
	}
	if hc.NetworkMode == "" {
		hc.NetworkMode = "bridge"
	}
	if hc.Runtime == "" {
		hc.Runtime = "runc"
	}
	return container.InspectResponse{Name: "/" + req.Name, Config: &cfg, HostConfig: &hc}
}

func TestCheckObserved(t *testing.T) {
	def, err := Translate(Spec{Lesson: "les", Role: "r", Image: "img"}, "", "abc")
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range []Request{goodRequest(), def} {
		if bad := CheckObserved(observed(req), req); len(bad) > 0 {
			t.Errorf("%s: %v", req.Name, bad)
		}
	}
	tests := []struct {
		name   string
		req    Request
		change func(*container.InspectResponse)
		n      int
	}{
		{"privileged", def, func(r *container.InspectResponse) { r.HostConfig.Privileged = true }, 1},
		{"host network for a default intent", def, func(r *container.InspectResponse) { r.HostConfig.NetworkMode = "host" }, 2},
		{"empty runtime", def, func(r *container.InspectResponse) { r.HostConfig.Runtime = "" }, 0},
		{"other runtime for runc", goodRequest(), func(r *container.InspectResponse) { r.HostConfig.Runtime = "runsc" }, 1},
		{"user", goodRequest(), func(r *container.InspectResponse) { r.Config.User = "0:0" }, 1},
		{"image user for an empty intent", def, func(r *container.InspectResponse) { r.Config.User = "10001:10001" }, 0},
		{"name", goodRequest(), func(r *container.InspectResponse) { r.Name = "/other" }, 1},
		{"label", goodRequest(), func(r *container.InspectResponse) { r.Config.Labels[LabelLesson] = "other" }, 1},
		{"caps in another spelling", goodRequest(), func(r *container.InspectResponse) { r.HostConfig.CapDrop = []string{"all"} }, 0},
		{"cap added", goodRequest(), func(r *container.InspectResponse) { r.HostConfig.CapAdd = []string{"CAP_CHOWN"} }, 1},
		{"read-only root dropped", goodRequest(), func(r *container.InspectResponse) { r.HostConfig.ReadonlyRootfs = false }, 1},
		{"security options", goodRequest(), func(r *container.InspectResponse) { r.HostConfig.SecurityOpt = nil }, 1},
		{"memory", goodRequest(), func(r *container.InspectResponse) { r.HostConfig.Memory = 1 }, 1},
		{"swap", goodRequest(), func(r *container.InspectResponse) { r.HostConfig.MemorySwap = -1 }, 1},
		{"pids", goodRequest(), func(r *container.InspectResponse) { r.HostConfig.PidsLimit = nil }, 1},
		{"tmpfs size dropped", goodRequest(), func(r *container.InspectResponse) {
			r.HostConfig.Mounts = []mount.Mount{{Type: mount.TypeTmpfs, Target: "/tmp"}}
		}, 1},
		{"extra bind", def, func(r *container.InspectResponse) { r.HostConfig.Binds = []string{"/:/host"} }, 1},
		{"cgroupns", def, func(r *container.InspectResponse) { r.HostConfig.CgroupnsMode = "host" }, 2},
		{"no config", def, func(r *container.InspectResponse) { r.HostConfig = nil }, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := observed(tc.req)
			tc.change(&got)
			wantViolations(t, CheckObserved(got, tc.req), InvInspectMatches, tc.n)
		})
	}
}

func TestNoLeakWording(t *testing.T) {
	got := noLeak("les", Leftovers{Containers: []string{"trinkets-les-r-1"}, Volumes: []string{"v1", "v2"}, Networks: []string{"n"}})
	wantViolations(t, got, InvNoLeak, 4)
	if got[0] != "no-leak: container trinkets-les-r-1 still carries trinkets.lesson=les" {
		t.Errorf("wording: %q", got[0])
	}
	wantViolations(t, noLeak("les", Leftovers{}), InvNoLeak, 0)
}
