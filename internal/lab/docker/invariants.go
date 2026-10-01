package docker

import (
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// Named invariants. Each is a small function that returns the violations it
// finds, empty when it holds. The code that does the work calls them before
// acting, and the tests call the same functions, so the two cannot drift.

// InvContextNamesClean: every name in a fixture build context is relative and
// clean (path.Clean leaves it unchanged), has no "..", backslash, NUL or
// trailing slash, and is valid UTF-8. A name outside this set could write
// outside the context when the daemon unpacks it. Names are generated, never
// taken from a lesson's paths, so this holds by construction; writeTar checks
// it anyway.
const InvContextNamesClean = "context-names-clean"

func contextNamesClean(files []contextFile) []string {
	var bad []string
	seen := map[string]bool{}
	for _, f := range files {
		n := f.Name
		switch {
		case n == "" || n == ".":
			bad = append(bad, fmt.Sprintf("%s: empty name %q", InvContextNamesClean, n))
		case path.IsAbs(n), path.Clean(n) != n, n == ".." || strings.HasPrefix(n, "../"):
			bad = append(bad, fmt.Sprintf("%s: %q is not relative and clean", InvContextNamesClean, n))
		case strings.ContainsAny(n, "\\\x00"), !utf8.ValidString(n):
			bad = append(bad, fmt.Sprintf("%s: %q has a backslash, NUL or invalid UTF-8", InvContextNamesClean, n))
		case seen[n]:
			bad = append(bad, fmt.Sprintf("%s: %q appears twice", InvContextNamesClean, n))
		}
		seen[n] = true
	}
	return bad
}

// Config invariants (plan, "Testing approach"). CheckConfig runs them all
// before every ContainerCreate; a violation is an error and nothing is
// created. Each message starts with the invariant's name.
const (
	InvNoDockerSocket     = "no-docker-socket"
	InvNeverPrivileged    = "never-privileged"
	InvLabelled           = "labelled"
	InvSwapPinned         = "swap-pinned"
	InvSeccompInline      = "seccomp-inline"
	InvRestrictedComplete = "restricted-complete"
	InvInspectMatches     = "inspect-matches-intent"
	InvNoLeak             = "no-leak"
)

// noDockerSocket: no emitted mount is a bind mount, and no mount source,
// after path.Clean, is /var/run/docker.sock, /run/docker.sock or the resolved
// Host() socket path (socket, "" when the endpoint is not a unix socket).
func noDockerSocket(hc container.HostConfig, socket string) []string {
	var bad []string
	for _, b := range hc.Binds {
		bad = append(bad, fmt.Sprintf("%s: bind mount %q", InvNoDockerSocket, b))
	}
	for _, m := range hc.Mounts {
		if m.Type == mount.TypeBind {
			bad = append(bad, fmt.Sprintf("%s: bind mount of %q at %q", InvNoDockerSocket, m.Source, m.Target))
		}
		if isDockerSocket(m.Source, socket) {
			bad = append(bad, fmt.Sprintf("%s: mount source %q is the Docker socket", InvNoDockerSocket, m.Source))
		}
	}
	return bad
}

// neverPrivileged: Privileged is false and no namespace mode (PidMode,
// NetworkMode, IpcMode, UTSMode, UsernsMode, CgroupnsMode) is host.
func neverPrivileged(hc container.HostConfig) []string {
	var bad []string
	if hc.Privileged {
		bad = append(bad, InvNeverPrivileged+": Privileged is true")
	}
	for _, ns := range []struct{ name, mode string }{
		{"PidMode", string(hc.PidMode)},
		{"NetworkMode", string(hc.NetworkMode)},
		{"IpcMode", string(hc.IpcMode)},
		{"UTSMode", string(hc.UTSMode)},
		{"UsernsMode", string(hc.UsernsMode)},
		{"CgroupnsMode", string(hc.CgroupnsMode)},
	} {
		if ns.mode == "host" {
			bad = append(bad, fmt.Sprintf("%s: %s is host", InvNeverPrivileged, ns.name))
		}
	}
	return bad
}

// labelled: labels trinkets.harness=1 and a non-empty trinkets.lesson are
// present, and the name starts with trinkets-<lesson>-.
func labelled(name string, cfg container.Config) []string {
	var bad []string
	if cfg.Labels[LabelHarness] != "1" {
		bad = append(bad, fmt.Sprintf("%s: label %s is %q, want 1", InvLabelled, LabelHarness, cfg.Labels[LabelHarness]))
	}
	lesson := cfg.Labels[LabelLesson]
	if lesson == "" {
		bad = append(bad, fmt.Sprintf("%s: label %s is missing or empty", InvLabelled, LabelLesson))
	} else if !strings.HasPrefix(name, "trinkets-"+lesson+"-") {
		bad = append(bad, fmt.Sprintf("%s: name %q does not start with trinkets-%s-", InvLabelled, name, lesson))
	}
	return bad
}

// swapPinned: Memory > 0 implies MemorySwap == Memory, so a memory limit is
// not quietly doubled by swap.
func swapPinned(hc container.HostConfig) []string {
	if hc.Memory > 0 && hc.MemorySwap != hc.Memory {
		return []string{fmt.Sprintf("%s: Memory %d with MemorySwap %d", InvSwapPinned, hc.Memory, hc.MemorySwap)}
	}
	return nil
}

// securityOptKV splits a security option the way the engine does: on the
// first "=", or on the first ":" for the legacy spelling.
func securityOptKV(opt string) (key, value string) {
	if k, v, ok := strings.Cut(opt, "="); ok {
		return k, v
	}
	k, v, _ := strings.Cut(opt, ":")
	return k, v
}

// seccompInline: a seccomp= option is unconfined or a JSON object.
func seccompInline(hc container.HostConfig) []string {
	var bad []string
	for _, opt := range hc.SecurityOpt {
		if k, v := securityOptKV(opt); k == "seccomp" && v != "unconfined" && !isJSONObject(v) {
			bad = append(bad, fmt.Sprintf("%s: seccomp option %.60q is neither unconfined nor a JSON object", InvSeccompInline, v))
		}
	}
	return bad
}

// restrictedComplete: the IsRestricted conditions, as violations: non-root
// numeric user, CapDrop contains ALL, no CapAdd, no-new-privileges, read-only
// root, network none, private PID/IPC/cgroup namespaces, memory, CPU and PID
// limits set.
func restrictedComplete(cfg container.Config, hc container.HostConfig) []string {
	var bad []string
	add := func(format string, a ...any) {
		bad = append(bad, fmt.Sprintf(InvRestrictedComplete+": "+format, a...))
	}
	uid, _, _ := strings.Cut(cfg.User, ":")
	if !validUser(cfg.User) || strings.TrimLeft(uid, "0") == "" {
		add("user %q is not a numeric non-root uid", cfg.User)
	}
	if !slices.Contains(hc.CapDrop, "ALL") {
		add("CapDrop %v lacks ALL", hc.CapDrop)
	}
	if len(hc.CapAdd) > 0 {
		add("CapAdd %v is not empty", hc.CapAdd)
	}
	if !slices.ContainsFunc(hc.SecurityOpt, isNoNewPrivileges) {
		add("no-new-privileges is not set")
	}
	if !hc.ReadonlyRootfs {
		add("root filesystem is writable")
	}
	if hc.NetworkMode != "none" {
		add("network is %q, want none", hc.NetworkMode)
	}
	if hc.PidMode != "" {
		add("PID namespace %q is shared", hc.PidMode)
	}
	if hc.IpcMode != container.IPCModePrivate {
		add("IPC namespace %q is not private", hc.IpcMode)
	}
	if hc.CgroupnsMode != container.CgroupnsModePrivate {
		add("cgroup namespace %q is not private", hc.CgroupnsMode)
	}
	if hc.Memory <= 0 || hc.NanoCPUs <= 0 || hc.PidsLimit == nil || *hc.PidsLimit <= 0 {
		add("memory %d, CPU %d and PID limit %v must all be set", hc.Memory, hc.NanoCPUs, ptrValue(hc.PidsLimit))
	}
	return bad
}

func ptrValue(p *int64) any {
	if p == nil {
		return "unset"
	}
	return *p
}

// isNoNewPrivileges matches the spellings the engine accepts for it.
func isNoNewPrivileges(opt string) bool {
	k, v := securityOptKV(opt)
	return k == "no-new-privileges" && (v == "" || v == "true")
}

// IsRestricted reports whether a translated container meets the restricted
// baseline (restricted-complete). The user lives in Config, the rest in
// HostConfig, so it takes both.
func IsRestricted(cfg container.Config, hc container.HostConfig) bool {
	return len(restrictedComplete(cfg, hc)) == 0
}

// CheckConfig runs every config invariant that holds for all containers the
// harness creates (restricted-complete applies to the Restricted preset only
// and is checked by IsRestricted). socket is the resolved daemon socket path.
// An empty result means the request may be sent.
func CheckConfig(req Request, socket string) []string {
	return slices.Concat(
		noDockerSocket(req.HostConfig, socket),
		neverPrivileged(req.HostConfig),
		labelled(req.Name, req.Config),
		swapPinned(req.HostConfig),
		seccompInline(req.HostConfig),
	)
}

// CheckObserved is inspect-matches-intent: the security fields the engine
// recorded for a container equal the ones in the request. Fields the request
// leaves at zero must still read as zero, except three the engine fills in:
// User (the image's USER), NetworkMode (bridge) and Runtime (the default
// runtime).
// Even then the observed value may never be host, and Privileged must be
// false. Capabilities compare in normalized form, which is what both the
// client and Translate send.
func CheckObserved(got container.InspectResponse, want Request) []string {
	var bad []string
	add := func(format string, a ...any) {
		bad = append(bad, fmt.Sprintf(InvInspectMatches+": "+format, a...))
	}
	if got.Config == nil || got.HostConfig == nil {
		return []string{InvInspectMatches + ": inspect has no Config or HostConfig"}
	}
	gc, gh, wh := *got.Config, *got.HostConfig, want.HostConfig
	if name := strings.TrimPrefix(got.Name, "/"); name != want.Name {
		add("name %q, want %q", name, want.Name)
	}
	if want.Config.User != "" && gc.User != want.Config.User {
		add("user %q, want %q", gc.User, want.Config.User)
	}
	for k, v := range want.Config.Labels {
		if gc.Labels[k] != v {
			add("label %s=%q, want %q", k, gc.Labels[k], v)
		}
	}
	for _, v := range neverPrivileged(gh) {
		add("%s", v)
	}
	eq := func(field string, got, want any) {
		if !reflect.DeepEqual(got, want) {
			add("%s %v, want %v", field, got, want)
		}
	}
	eq("ReadonlyRootfs", gh.ReadonlyRootfs, wh.ReadonlyRootfs)
	eq("CapAdd", normalizeCaps(gh.CapAdd), normalizeCaps(wh.CapAdd))
	eq("CapDrop", normalizeCaps(gh.CapDrop), normalizeCaps(wh.CapDrop))
	eq("SecurityOpt", emptyNil(gh.SecurityOpt), emptyNil(wh.SecurityOpt))
	eq("GroupAdd", emptyNil(gh.GroupAdd), emptyNil(wh.GroupAdd))
	if wh.NetworkMode != "" || (gh.NetworkMode != "bridge" && gh.NetworkMode != "default") {
		eq("NetworkMode", gh.NetworkMode, wh.NetworkMode)
	}
	if wh.Runtime != "" || gh.Runtime == "" {
		eq("Runtime", gh.Runtime, wh.Runtime)
	}
	eq("PidMode", gh.PidMode, wh.PidMode)
	eq("IpcMode", gh.IpcMode, wh.IpcMode)
	eq("CgroupnsMode", gh.CgroupnsMode, wh.CgroupnsMode)
	eq("UTSMode", gh.UTSMode, wh.UTSMode)
	eq("UsernsMode", gh.UsernsMode, wh.UsernsMode)
	eq("Memory", gh.Memory, wh.Memory)
	eq("MemorySwap", gh.MemorySwap, wh.MemorySwap)
	eq("NanoCPUs", gh.NanoCPUs, wh.NanoCPUs)
	eq("PidsLimit", ptrValueOr0(gh.PidsLimit), ptrValueOr0(wh.PidsLimit))
	eq("Binds", emptyNil(gh.Binds), emptyNil(wh.Binds))
	eq("Mounts", mountKeys(gh.Mounts), mountKeys(wh.Mounts))
	return bad
}

func emptyNil(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func ptrValueOr0(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// mountKeys is the security-relevant part of each mount, in order.
func mountKeys(ms []mount.Mount) []string {
	var out []string
	for _, m := range ms {
		var size int64
		if m.TmpfsOptions != nil {
			size = m.TmpfsOptions.SizeBytes
		}
		out = append(out, fmt.Sprintf("%s %q -> %q ro=%t size=%d", m.Type, m.Source, m.Target, m.ReadOnly, size))
	}
	return out
}

// Peer-identity invariants (H5). VerifyPeerIdentity checks both on every run
// and fails when either is violated.
const (
	InvPeerUIDIsAssignedUID    = "peer-uid-is-assigned-uid"
	InvSocketGroupGatesConnect = "socket-group-gates-connect"
)

// parsePeerCred reads peer-echo's report, "uid=U gid=G pid=P". The match is
// exact: the text must be what formatting the three numbers gives back, so
// "uid=+5" or trailing text is malformed rather than read loosely.
func parsePeerCred(s string) (uid, gid, pid int, err error) {
	const format = "uid=%d gid=%d pid=%d"
	if _, err := fmt.Sscanf(s, format, &uid, &gid, &pid); err != nil || fmt.Sprintf(format, uid, gid, pid) != s {
		return 0, 0, 0, fmt.Errorf("peer credentials %q are not %q", s, format)
	}
	return uid, gid, pid, nil
}

// peerUIDIsAssignedUID: the UID the broker read from SO_PEERCRED equals the
// UID the launcher assigned. peer-echo sends what it read back to the worker
// as its reply, so the worker's "reply" line is the broker's report; a missing
// or malformed reply is a violation, not a pass.
func peerUIDIsAssignedUID(d PeerDial) []string {
	l, ok := probeout.Find(probeout.Parse(d.Stdout), "reply")
	if !ok || !l.OK {
		return []string{fmt.Sprintf("%s: %s (uid %d) got no reply from the broker: %q", InvPeerUIDIsAssignedUID, d.Role, d.UID, d.Stdout)}
	}
	uid, _, _, err := parsePeerCred(l.Detail)
	if err != nil {
		return []string{fmt.Sprintf("%s: %s: %v", InvPeerUIDIsAssignedUID, d.Role, err)}
	}
	if uid != d.UID {
		return []string{fmt.Sprintf("%s: %s: the broker read uid %d, the launcher assigned %d", InvPeerUIDIsAssignedUID, d.Role, uid, d.UID)}
	}
	return nil
}

// socketGroupGatesConnect: a worker without the socket group cannot connect.
// Only a refusal for permission counts: a dial that fails for another reason
// (the socket is missing, say) shows nothing about the group.
func socketGroupGatesConnect(d PeerDial) []string {
	l, ok := probeout.Find(probeout.Parse(d.Stdout), "dial")
	switch {
	case !ok:
		return []string{fmt.Sprintf("%s: %s (uid %d) printed no dial line: %q", InvSocketGroupGatesConnect, d.Role, d.UID, d.Stdout)}
	case l.OK:
		return []string{fmt.Sprintf("%s: %s (uid %d) connected without the socket group", InvSocketGroupGatesConnect, d.Role, d.UID)}
	case !strings.Contains(l.Detail, "permission denied"):
		return []string{fmt.Sprintf("%s: %s (uid %d) was refused for another reason: %s", InvSocketGroupGatesConnect, d.Role, d.UID, l.Detail)}
	}
	return nil
}

// checkPeerIdentity runs the two peer-identity invariants over a set of dials:
// peer-uid-is-assigned-uid for workers in the socket group,
// socket-group-gates-connect for the others. A set too small to show anything
// is a violation too: it needs two workers in the group with different UIDs
// and one outside it.
func checkPeerIdentity(dials []PeerDial) []string {
	var bad []string
	uids := map[int]bool{}
	outsiders := 0
	for _, d := range dials {
		if d.InGroup {
			uids[d.UID] = true
			bad = append(bad, peerUIDIsAssignedUID(d)...)
		} else {
			outsiders++
			bad = append(bad, socketGroupGatesConnect(d)...)
		}
	}
	if len(uids) < 2 || outsiders == 0 {
		bad = append(bad, fmt.Sprintf("peer identity: need two workers in the socket group with different UIDs and one outside it, got %d UIDs and %d outside", len(uids), outsiders))
	}
	return bad
}

// Egress topology invariants (H6). CheckWorkerHasNoRoute and
// CheckBlockedFixtureUntouched read the facts from the running containers and
// apply these.
const (
	InvWorkerHasNoRoute        = "worker-has-no-route"
	InvBlockedFixtureUntouched = "blocked-fixture-untouched"
)

// workerHasNoRoute: the worker's only network interface is lo, so no
// address, gateway or name it tries can be routed anywhere; the socket
// volume is its only way out.
func workerHasNoRoute(interfaces []string) []string {
	if len(interfaces) == 1 && interfaces[0] == "lo" {
		return nil
	}
	return []string{fmt.Sprintf("%s: interfaces %q, want only lo", InvWorkerHasNoRoute, interfaces)}
}

// blockedFixtureUntouched: the blocked fixture counted no request.
func blockedFixtureUntouched(requests int) []string {
	if requests == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%s: the blocked fixture counted %d requests, want 0", InvBlockedFixtureUntouched, requests)}
}

// noLeak words the no-leak violations for objects still carrying a lesson
// label after its handles were removed or Sweep ran.
func noLeak(lesson string, left Leftovers) []string {
	var bad []string
	for _, kind := range []struct {
		kind  string
		names []string
	}{{"container", left.Containers}, {"volume", left.Volumes}, {"network", left.Networks}} {
		for _, n := range kind.names {
			bad = append(bad, fmt.Sprintf("%s: %s %s still carries %s=%s", InvNoLeak, kind.kind, n, LabelLesson, lesson))
		}
	}
	return bad
}
