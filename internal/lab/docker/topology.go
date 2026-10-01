package docker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/client"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// SocketDir is where both SocketVolume mounts put the shared socket
// directory inside the broker and the workers.
const SocketDir = "/sock"

// PeerSocket is where VerifyPeerIdentity's peer-echo broker listens.
const PeerSocket = SocketDir + "/peer.sock"

// ProbePackage is the import path of the harness's own fixture binary (peer-echo,
// dial, initdir and the rest). go build resolves it from anywhere inside the
// module, which is where lessons and tests run.
const ProbePackage = "github.com/nickstrad/systems-trinkets/internal/lab/docker/probe"

// socketDirMode is what the init step gives the socket directory: the broker
// (owner) creates and removes entries, the socket group may only look names up
// (enough to connect), others get nothing, and the setgid bit makes new
// entries take the socket group.
const socketDirMode = "2750"

// maxEngineID is the largest uid or gid the engine starts a container with:
// a larger User or GroupAdd passes create and fails at start with "uids and
// gids must be in range 0-2147483647" (validID allows up to 2^32-2).
const maxEngineID = 1<<31 - 1

func validEngineID(n int) bool { return n >= 0 && n <= maxEngineID }

// lineTimeout bounds WaitLine when the caller's context has no sooner deadline.
const lineTimeout = 30 * time.Second

// SocketVolume creates a labelled named volume and prepares it so a broker
// running as brokerUID:socketGID can bind a Unix socket in it: a one-shot
// init container (root, CapDrop ALL, CapAdd CHOWN and FOWNER, no network,
// read-only root) sets the directory to brokerUID:socketGID mode 2750. The init
// container's group is socketGID, not 0: chmod clears the setgid bit when the
// caller is neither in the file's group nor holds CAP_FSETID, so as 0:0 the
// directory ended up 0750.
// broker is the read-write Mount for the broker; worker is the Mount for
// workers, who reach the socket through supplementary group socketGID.
// Both target SocketDir. Sweep removes the volume.
//
// The worker mount is read-only. connect(2) on a socket does not write the
// filesystem, so it still works there (TestSpec_H5_WorkersReportedWithOwnUIDAndPrimaryGID
// connects through it), while unlinking, replacing, chown and chmod fail with
// EROFS before the directory permissions are even asked.
//
// The init container runs the probe's initdir, so SocketVolume builds the probe
// fixture (a few seconds) and removes that image again before it returns.
func SocketVolume(ctx context.Context, cli *client.Client, lesson string, brokerUID, socketGID int) (broker, worker Mount, err error) {
	image, removeImage, err := buildProbe(ctx, cli, lesson)
	if err != nil {
		return Mount{}, Mount{}, err
	}
	defer func() { err = errors.Join(err, removeImage()) }()
	return socketVolume(ctx, cli, lesson, image, brokerUID, socketGID)
}

// socketVolume is SocketVolume with the probe image given, so a caller that
// already has one does not build it again.
func socketVolume(ctx context.Context, cli *client.Client, lesson, image string, brokerUID, socketGID int) (broker, worker Mount, err error) {
	if !validEngineID(brokerUID) || !validEngineID(socketGID) {
		return Mount{}, Mount{}, fmt.Errorf("docker: socket volume owner %d:%d is out of range", brokerUID, socketGID)
	}
	name, err := CreateVolume(ctx, cli, lesson, "sock")
	if err != nil {
		return Mount{}, Mount{}, err
	}
	prep := Restricted(lesson, "sockinit", image, "initdir", SocketDir, owner(brokerUID, socketGID), socketDirMode)
	prep.User = owner(0, socketGID) // in the group, so chmod keeps the setgid bit
	prep.CapAdd = []string{"CHOWN", "FOWNER"}
	prep.Mounts = append(prep.Mounts, Mount{Type: MountVolume, Source: name, Target: SocketDir})
	res, err := Run(ctx, cli, prep)
	if err == nil && res.ExitCode != 0 {
		err = fmt.Errorf("exit %d: %s%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if err != nil {
		return Mount{}, Mount{}, errors.Join(fmt.Errorf("docker: init socket volume %s: %w", name, err), removeVolume(ctx, cli, name))
	}
	return Mount{Type: MountVolume, Source: name, Target: SocketDir},
		Mount{Type: MountVolume, Source: name, Target: SocketDir, ReadOnly: true}, nil
}

func owner(uid, gid int) string { return strconv.Itoa(uid) + ":" + strconv.Itoa(gid) }

// BrokerSpec is the broker of a socket topology: Restricted running as
// brokerUID:socketGID with the broker's SocketVolume mount, so the socket it
// creates belongs to it and the socket group.
func BrokerSpec(lesson, image string, brokerUID, socketGID int, sock Mount, cmd ...string) Spec {
	s := Restricted(lesson, "broker", image, cmd...)
	s.User = owner(brokerUID, socketGID)
	s.Mounts = append(s.Mounts, sock)
	return s
}

// WorkerSpec is a worker of a socket topology: Restricted running as uid:gid
// with socketGID as its one supplementary group and the worker's SocketVolume
// mount. With every capability dropped and no-new-privileges set it has no
// SETUID or SETGID, so the identity the broker reads is the one assigned here.
func WorkerSpec(lesson, role, image string, uid, gid, socketGID int, sock Mount, cmd ...string) Spec {
	s := Restricted(lesson, role, image, cmd...)
	s.User = owner(uid, gid)
	s.Groups = []string{strconv.Itoa(socketGID)}
	s.Mounts = append(s.Mounts, sock)
	return s
}

// WaitLine polls a fixture container's output until it prints the probe line
// called name and returns that line, OK or DENIED. It gives up when ctx ends
// or after 30 s.
func (c *Container) WaitLine(ctx context.Context, name string) (probeout.Line, error) {
	ctx, cancel := context.WithTimeout(ctx, lineTimeout)
	defer cancel()
	for {
		stdout, _, err := c.Logs(ctx)
		if err != nil {
			return probeout.Line{}, err
		}
		if l, ok := probeout.Find(probeout.Parse(stdout), name); ok {
			return l, nil
		}
		select {
		case <-ctx.Done():
			return probeout.Line{}, fmt.Errorf("docker: %s printed no %q line (stdout %q): %w", c.Name, name, stdout, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// PeerIdentities are the numeric identities VerifyPeerIdentity assigns. Each
// worker runs as uid:uid, so its primary group equals its UID. All five
// numbers must differ and be non-zero: a worker whose primary group were the
// socket group would be in it without being given it.
type PeerIdentities struct {
	BrokerUID, SocketGID int
	Workers              [2]int // UIDs of two workers in the socket group
	Outsider             int    // UID of a worker without the socket group
}

// DefaultPeerIdentities are the identities of the scratch spike.
var DefaultPeerIdentities = PeerIdentities{BrokerUID: 20000, SocketGID: 30000, Workers: [2]int{20001, 20002}, Outsider: 20003}

func (p PeerIdentities) validate() error {
	ids := []int{p.BrokerUID, p.SocketGID, p.Workers[0], p.Workers[1], p.Outsider}
	seen := map[int]bool{}
	for _, id := range ids {
		if id == 0 || !validEngineID(id) || seen[id] {
			return fmt.Errorf("docker: peer identities %+v must be distinct, non-zero and at most %d", p, maxEngineID)
		}
		seen[id] = true
	}
	return nil
}

// PeerDial is one worker's dial to the peer-echo broker: the identity the
// launcher assigned and the probe lines the worker printed (a "dial" line and,
// once connected, a "reply" line holding the broker's SO_PEERCRED reading).
type PeerDial struct {
	Role     string
	UID, GID int
	InGroup  bool // the worker had the socket group
	Stdout   string
}

// VerifyPeerIdentity checks that a broker on this engine can trust the
// kernel's peer credentials. It prepares a socket volume, starts the probe's
// peer-echo as the broker, dials it from the two workers and the outsider, and
// checks peer-uid-is-assigned-uid and socket-group-gates-connect. A violation
// is an error naming the invariant: a user-namespace remap or Docker Desktop's
// Enhanced Container Isolation would make the UIDs differ, and a lesson must
// not measure on top of that. It builds the probe fixture and removes
// everything it created before it returns; the dials are returned either way.
func VerifyPeerIdentity(ctx context.Context, cli *client.Client, lesson string, ids PeerIdentities) (dials []PeerDial, err error) {
	if err := ids.validate(); err != nil {
		return nil, err
	}
	image, removeImage, err := buildProbe(ctx, cli, lesson)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, removeImage()) }()
	return verifyPeerIdentity(ctx, cli, lesson, image, ids)
}

// verifyPeerIdentity is VerifyPeerIdentity with the probe image given.
func verifyPeerIdentity(ctx context.Context, cli *client.Client, lesson, image string, ids PeerIdentities) (dials []PeerDial, err error) {
	if err := ids.validate(); err != nil {
		return nil, err
	}
	brokerMount, workerMount, err := socketVolume(ctx, cli, lesson, image, ids.BrokerUID, ids.SocketGID)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, removeVolume(ctx, cli, brokerMount.Source)) }()
	b, err := Start(ctx, cli, BrokerSpec(lesson, image, ids.BrokerUID, ids.SocketGID, brokerMount, "peer-echo", PeerSocket))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, b.Remove(ctx)) }() // before the volume, which it holds
	if l, err := b.WaitLine(ctx, "listen"); err != nil {
		return nil, err
	} else if !l.OK {
		return nil, fmt.Errorf("docker: broker %s cannot listen: %s", b.Name, l.Detail)
	}

	dial := func(role string, uid int, inGroup bool) error {
		s := WorkerSpec(lesson, role, image, uid, uid, ids.SocketGID, workerMount, "dial", "unix", PeerSocket, "peer\n")
		if !inGroup {
			s.Groups = nil
		}
		res, err := Run(ctx, cli, s)
		if err != nil {
			return err
		}
		dials = append(dials, PeerDial{Role: role, UID: uid, GID: uid, InGroup: inGroup, Stdout: res.Stdout})
		return nil
	}
	for i, uid := range ids.Workers {
		if err := dial("worker"+strconv.Itoa(i+1), uid, true); err != nil {
			return dials, err
		}
	}
	if err := dial("outsider", ids.Outsider, false); err != nil {
		return dials, err
	}
	if bad := checkPeerIdentity(dials); len(bad) > 0 {
		return dials, fmt.Errorf("docker: peer identity cannot be trusted on this engine: %s", strings.Join(bad, "; "))
	}
	return dials, nil
}

// buildProbe builds the probe fixture for lesson under a tag of its own and
// returns the tag and a function that removes the image again (on a detached
// context, like every cleanup here).
func buildProbe(ctx context.Context, cli *client.Client, lesson string) (string, func() error, error) {
	tag := "trinkets-" + lesson + "-probe-" + NameSuffix() + ":dev"
	remove := func() error {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()
		if _, err := cli.ImageRemove(ctx, tag, client.ImageRemoveOptions{Force: true, PruneChildren: true}); err != nil {
			if _, ierr := cli.ImageInspect(ctx, tag); ierr != nil {
				return nil // never built, or already gone
			}
			return fmt.Errorf("docker: remove image %s: %w", tag, err)
		}
		return nil
	}
	if err := BuildFixture(ctx, cli, FixtureImage{Lesson: lesson, Tag: tag, Package: ProbePackage}); err != nil {
		return "", nil, errors.Join(err, remove())
	}
	return tag, remove, nil
}

// removeVolume force-removes a volume on a detached context, so it still runs
// after ctx ended.
func removeVolume(ctx context.Context, cli *client.Client, name string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if _, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("docker: remove volume %s: %w", name, err)
	}
	return nil
}
