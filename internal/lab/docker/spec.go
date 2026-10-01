package docker

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// FixtureBinary is where the fixture binary sits in every fixture image. An
// exec does not apply the image entrypoint, so an Exec command starts with it:
// h.Exec(ctx, docker.FixtureBinary, "pids").
const FixtureBinary = fixtureBinary

// MountType is the kind of a Mount. There is no bind type: named volumes and
// tmpfs avoid Docker Desktop file sharing, and a container can never be handed
// a host path such as the Docker socket.
type MountType string

const (
	MountVolume MountType = "volume" // a named volume; Source is its name
	MountTmpfs  MountType = "tmpfs"  // a fresh tmpfs; Source must be empty
)

// Mount is one volume or tmpfs mount with an explicit target.
type Mount struct {
	Type MountType
	// Source is the volume name for MountVolume (anonymous volumes are not
	// allowed: they would carry no lesson label) and empty for MountTmpfs.
	Source string
	// Target is the absolute, clean path inside the container.
	Target   string
	ReadOnly bool
	// TmpfsSize caps a tmpfs in bytes; 0 is the engine default (half of RAM).
	TmpfsSize int64
}

// Spec describes one fixture container. Every isolation knob is a field a
// lesson flips on its own; the zero value of each is Docker's default, so
// Spec{Lesson, Role, Image} is the default container a lesson compares
// against, and Restricted is the hardened baseline. There is no Privileged
// field.
type Spec struct {
	Lesson, Role string // labels and the container name prefix
	Image        string
	Cmd, Env     []string
	User         string   // numeric "uid" or "uid:gid"; "" keeps the image's USER
	Groups       []string // supplementary numeric groups (GroupAdd)
	Mounts       []Mount  // volume or tmpfs, explicit target, ReadOnly flag
	Network      string   // "none", "bridge" or a network name; "" = engine default
	ReadOnlyRoot bool
	CapDrop      []string
	CapAdd       []string
	// NoNewPrivileges sets no-new-privileges:true.
	NoNewPrivileges bool
	Seccomp         string // "" builtin, "unconfined", or the profile JSON inline
	PIDMode         string // "" private, "container:<id or name>" shared
	Memory          int64  // bytes; swap is pinned to the same value
	NanoCPUs        int64
	PidsLimit       int64  // 0 = engine default (no limit)
	Runtime         string // OCI runtime handler; "" = engine default
}

// Restricted baseline values (plan, "Verified facts").
const (
	RestrictedUser      = DefaultFixtureUser // 10001:10001, non-root and numeric
	RestrictedMemory    = 64 << 20           // 64 MiB, swap pinned to it
	RestrictedNanoCPUs  = 500_000_000        // 0.5 CPU
	RestrictedPidsLimit = 64
	RestrictedTmpSize   = 16 << 20 // the /tmp tmpfs
	// RestrictedRuntime is runc by name, not "": the baseline then means the
	// same runtime on every engine even where an operator changed the
	// default, and a gVisor variant is one field (Runtime: "runsc"). runc is
	// registered on Docker Engine and on Docker Desktop.
	RestrictedRuntime = "runc"
)

// Restricted is the verified restricted baseline: user 10001:10001, no
// network, read-only root, every capability dropped, no-new-privileges,
// memory (swap pinned), CPU and PID limits, runc, and a tmpfs at /tmp.
// Private PID, IPC and cgroup namespaces come from Translate, which always
// sets IPC and cgroup to private and leaves PID private unless PIDMode says
// otherwise.
//
// It has no /work volume: a named volume outlives the container, so the
// lesson creates one (CreateVolume, which labels it for Sweep) and appends a
// Mount, and the image needs an OwnedDir there for a non-root user to write.
//
// Restricted never fails. Lesson and role are copied as given; Translate
// rejects names that are not lower-case letters, digits and single hyphens.
func Restricted(lesson, role, image string, cmd ...string) Spec {
	return Spec{
		Lesson:          lesson,
		Role:            role,
		Image:           image,
		Cmd:             slices.Clone(cmd),
		User:            RestrictedUser,
		Mounts:          []Mount{{Type: MountTmpfs, Target: "/tmp", TmpfsSize: RestrictedTmpSize}},
		Network:         "none",
		ReadOnlyRoot:    true,
		CapDrop:         []string{"ALL"},
		NoNewPrivileges: true,
		Memory:          RestrictedMemory,
		NanoCPUs:        RestrictedNanoCPUs,
		PidsLimit:       RestrictedPidsLimit,
		Runtime:         RestrictedRuntime,
	}
}

// restricted-complete is enforced where the preset is made: the package
// refuses to load if Restricted no longer translates to a restricted
// container, because that is a bug here and no lesson should measure on it.
func init() {
	req, err := Translate(Restricted("check", "check", "check"), "", "check")
	if err != nil {
		panic("docker: Restricted does not translate: " + err.Error())
	}
	if bad := restrictedComplete(req.Config, req.HostConfig); len(bad) > 0 {
		panic("docker: Restricted is incomplete: " + strings.Join(bad, "; "))
	}
}

// Request is what a Spec translates to: the container name and the two
// engine configs, ready for ContainerCreate.
type Request struct {
	Name       string
	Config     container.Config
	HostConfig container.HostConfig
}

// Translation errors that callers and tests match with errors.Is. Every
// other rejection wraps ErrInvalidSpec.
var (
	// ErrDockerSocket: a mount source names the Docker socket.
	ErrDockerSocket = errors.New("mount source is the Docker socket")
	// ErrSeccompPath: Seccomp is neither "", "unconfined" nor a JSON object.
	// The engine reads the profile inline only; a file path would fail at
	// start, after create, so Translate refuses it up front.
	ErrSeccompPath = errors.New(`seccomp must be "", "unconfined" or the profile JSON inline, not a path`)
	// ErrInvalidSpec: any other field the engine would reject or mis-read.
	ErrInvalidSpec = errors.New("invalid spec")
)

// Fixed Docker socket paths, rejected as mount sources on every engine
// besides the resolved one Translate is given.
var fixedSocketPaths = []string{"/var/run/docker.sock", "/run/docker.sock"}

var (
	// volumeNameRE is the engine's own rule for local volume names.
	volumeNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)
	// networkNameRE accepts what a container or network name can be.
	networkNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	suffixRE      = regexp.MustCompile(`^[a-z0-9]{1,32}$`)
	capRE         = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	numericIDRE   = regexp.MustCompile(`^[0-9]+$`)
)

// NameSuffix returns the short random part of a container or volume name, so
// parallel runs do not collide. Translate takes it as an input to stay
// deterministic.
func NameSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms
	return hex.EncodeToString(b)
}

// ContainerName is trinkets-<lesson>-<role>-<suffix>.
func ContainerName(lesson, role, suffix string) string {
	return "trinkets-" + lesson + "-" + role + "-" + suffix
}

// SocketPath is the filesystem path of a unix:// daemon host (as Host() or
// cli.DaemonHost() return it), or "" for any other kind of endpoint. The
// client's ParseHostURL keeps a unix socket path in URL.Host, not URL.Path.
func SocketPath(host string) string {
	u, err := client.ParseHostURL(host)
	if err != nil || u.Scheme != "unix" {
		return ""
	}
	return u.Host
}

// socketPaths is the exact set of mount sources no-docker-socket forbids:
// the fixed paths and the resolved socket, each cleaned.
func socketPaths(socket string) []string {
	out := slices.Clone(fixedSocketPaths)
	if socket != "" {
		out = append(out, path.Clean(socket))
	}
	return out
}

// isDockerSocket reports whether a mount source, after path.Clean, is one of
// the socket paths. The match is exact: "0docker.sock" is a fine volume name.
func isDockerSocket(source, socket string) bool {
	return source != "" && slices.Contains(socketPaths(socket), path.Clean(source))
}

// isJSONObject reports whether s is one valid-UTF-8 JSON object.
func isJSONObject(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	var obj map[string]json.RawMessage
	return json.Unmarshal([]byte(s), &obj) == nil && obj != nil
}

// normalizeCaps puts capabilities in the form the client sends and the
// engine records: upper case, CAP_ prefix except for ALL, sorted, no
// duplicates. Translating to that form keeps intent and inspect comparable.
func normalizeCaps(caps []string) []string {
	if caps == nil {
		return nil
	}
	out := make([]string, len(caps))
	for i, c := range caps {
		c = strings.ToUpper(c)
		if c != "ALL" && !strings.HasPrefix(c, "CAP_") {
			c = "CAP_" + c
		}
		out[i] = c
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func validNumericID(s string) bool {
	if !numericIDRE.MatchString(s) {
		return false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return err == nil && validID(n)
}

func validUser(u string) bool {
	uid, gid, hasGID := strings.Cut(u, ":")
	return validNumericID(uid) && (!hasGID || validNumericID(gid))
}

// Translate is the only path from a Spec to engine config. It is pure: socket
// is the resolved daemon socket path (SocketPath of cli.DaemonHost(), "" if
// the endpoint is not a unix socket) and suffix the random name part
// (NameSuffix), so equal inputs give equal output. It rejects, never tidies:
// every problem found is reported (joined), and on error the Request is the
// zero value.
//
// What it always sets: the harness labels, private IPC and cgroup namespaces
// (the engine defaults on cgroup v2, written down so they are part of the
// intent CheckObserved compares), MemorySwap equal to Memory, and harness
// labels on every volume mount, so a volume the engine creates on first use
// is still swept. Runtime is passed through untouched.
func Translate(s Spec, socket, suffix string) (Request, error) {
	var errs []error
	bad := func(format string, a ...any) {
		errs = append(errs, fmt.Errorf("%w: "+format, append([]any{ErrInvalidSpec}, a...)...))
	}

	if !lessonRE.MatchString(s.Lesson) {
		bad("lesson %q must match %s", s.Lesson, lessonRE)
	}
	if !lessonRE.MatchString(s.Role) {
		bad("role %q must match %s", s.Role, lessonRE)
	}
	if !suffixRE.MatchString(suffix) {
		bad("name suffix %q must match %s", suffix, suffixRE)
	}
	if s.Image == "" {
		bad("image is empty")
	}
	if s.User != "" && !validUser(s.User) {
		bad("user %q must be a numeric uid or uid:gid (fixture images have no passwd file)", s.User)
	}
	for _, g := range s.Groups {
		if !validNumericID(g) {
			bad("group %q must be a numeric gid", g)
		}
	}

	var mounts []mount.Mount
	targets := map[string]bool{}
	for i, m := range s.Mounts {
		if isDockerSocket(m.Source, socket) {
			errs = append(errs, fmt.Errorf("mount %d: %w: %q", i, ErrDockerSocket, m.Source))
		}
		switch m.Type {
		case MountVolume:
			if !volumeNameRE.MatchString(m.Source) {
				bad("mount %d: volume name %q must match %s (no paths, no anonymous volumes)", i, m.Source, volumeNameRE)
			}
			if m.TmpfsSize != 0 {
				bad("mount %d: TmpfsSize on a volume", i)
			}
		case MountTmpfs:
			if m.Source != "" {
				bad("mount %d: a tmpfs takes no source, got %q", i, m.Source)
			}
			if m.TmpfsSize < 0 {
				bad("mount %d: negative TmpfsSize", i)
			}
		default:
			bad("mount %d: type %q must be %q or %q", i, m.Type, MountVolume, MountTmpfs)
		}
		switch t := m.Target; {
		case t == "" || t[0] != '/' || t == "/":
			bad("mount %d: target %q must be an absolute path other than /", i, t)
		case path.Clean(t) != t:
			bad("mount %d: target %q is not clean", i, t)
		case strings.ContainsRune(t, 0) || !utf8.ValidString(t):
			bad("mount %d: target %q has NUL or invalid UTF-8", i, t)
		case targets[t]:
			bad("mount %d: target %q is used twice", i, t)
		}
		targets[m.Target] = true
		mm := mount.Mount{Type: mount.Type(m.Type), Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly}
		if m.Type == MountVolume {
			mm.VolumeOptions = &mount.VolumeOptions{Labels: Labels(s.Lesson)}
		}
		if m.Type == MountTmpfs && m.TmpfsSize > 0 {
			mm.TmpfsOptions = &mount.TmpfsOptions{SizeBytes: m.TmpfsSize}
		}
		mounts = append(mounts, mm)
	}

	switch n := s.Network; {
	case n == "host":
		bad(`network "host" shares the host's network namespace`)
	case n != "" && !networkNameRE.MatchString(n):
		bad("network %q must be none, bridge or a network name", n)
	}
	if s.PIDMode != "" {
		id, ok := strings.CutPrefix(s.PIDMode, "container:")
		if !ok || !networkNameRE.MatchString(id) {
			bad(`PID mode %q must be "" or "container:<id or name>"`, s.PIDMode)
		}
	}
	for _, c := range slices.Concat(s.CapDrop, s.CapAdd) {
		if !capRE.MatchString(c) {
			bad("capability %q must be letters, digits and underscores", c)
		}
	}

	var securityOpt []string
	if s.NoNewPrivileges {
		securityOpt = append(securityOpt, "no-new-privileges:true")
	}
	switch {
	case s.Seccomp == "":
	case s.Seccomp == "unconfined":
		securityOpt = append(securityOpt, "seccomp=unconfined")
	case isJSONObject(s.Seccomp):
		var compact bytes.Buffer
		_ = json.Compact(&compact, []byte(s.Seccomp)) // valid JSON always compacts
		securityOpt = append(securityOpt, "seccomp="+compact.String())
	default:
		errs = append(errs, fmt.Errorf("%w (got %.40q)", ErrSeccompPath, s.Seccomp))
	}

	if s.Memory < 0 || s.NanoCPUs < 0 || s.PidsLimit < 0 {
		bad("Memory %d, NanoCPUs %d and PidsLimit %d must not be negative", s.Memory, s.NanoCPUs, s.PidsLimit)
	}

	if len(errs) > 0 {
		return Request{}, fmt.Errorf("docker: spec %s/%s: %w", s.Lesson, s.Role, errors.Join(errs...))
	}

	res := container.Resources{Memory: s.Memory, NanoCPUs: s.NanoCPUs}
	if s.Memory > 0 {
		res.MemorySwap = s.Memory
	}
	if s.PidsLimit > 0 {
		pids := s.PidsLimit
		res.PidsLimit = &pids
	}
	return Request{
		Name: ContainerName(s.Lesson, s.Role, suffix),
		Config: container.Config{
			Image:  s.Image,
			Cmd:    slices.Clone(s.Cmd),
			Env:    slices.Clone(s.Env),
			User:   s.User,
			Labels: Labels(s.Lesson),
		},
		HostConfig: container.HostConfig{
			NetworkMode:    container.NetworkMode(s.Network),
			ReadonlyRootfs: s.ReadOnlyRoot,
			CapDrop:        normalizeCaps(s.CapDrop),
			CapAdd:         normalizeCaps(s.CapAdd),
			SecurityOpt:    securityOpt,
			PidMode:        container.PidMode(s.PIDMode),
			IpcMode:        container.IPCModePrivate,
			CgroupnsMode:   container.CgroupnsModePrivate,
			GroupAdd:       slices.Clone(s.Groups),
			Mounts:         mounts,
			Resources:      res,
			Runtime:        s.Runtime,
		},
	}, nil
}
