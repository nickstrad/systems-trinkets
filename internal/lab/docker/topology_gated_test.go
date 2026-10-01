package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/moby/moby/client"
	"pgregory.net/rapid"

	"github.com/nickstrad/systems-trinkets/internal/lab/docker/probeout"
)

// Gated peer-identity tests (H5). Lesson labels start with h5-; brokers and
// workers use the package's shared probe image, and SocketVolume and
// VerifyPeerIdentity, which build their own, are checked for leftover images.

// removeVolumeOnCleanup removes a volume when the test ends. Register it
// before starting the containers that mount it: cleanups run last-registered
// first, so the containers go before their volume.
func removeVolumeOnCleanup(t *testing.T, cli *client.Client, name string) {
	t.Cleanup(func() {
		if err := removeVolume(context.Background(), cli, name); err != nil {
			t.Errorf("%v", err)
		}
	})
}

// mustNoImages fails when an image still carries the lesson's label.
func mustNoImages(t *testing.T, ctx context.Context, cli *client.Client, lesson string) {
	t.Helper()
	res, err := cli.ImageList(ctx, client.ImageListOptions{All: true, Filters: lessonFilter(lesson)})
	if err != nil {
		t.Fatal(err)
	}
	for _, img := range res.Items {
		t.Errorf("image %v still carries %s=%s", img.RepoTags, LabelLesson, lesson)
	}
}

// startPeerBroker prepares a socket volume for the default identities and
// starts peer-echo on it as the broker. Everything goes when the test ends.
func startPeerBroker(t *testing.T, ctx context.Context, cli *client.Client, lesson, img string) (b *Container, brokerMount, workerMount Mount) {
	t.Helper()
	ids := DefaultPeerIdentities
	brokerMount, workerMount, err := socketVolume(ctx, cli, lesson, img, ids.BrokerUID, ids.SocketGID)
	if err != nil {
		t.Fatal(err)
	}
	removeVolumeOnCleanup(t, cli, brokerMount.Source)
	b, err = Start(ctx, cli, BrokerSpec(lesson, img, ids.BrokerUID, ids.SocketGID, brokerMount, "peer-echo", PeerSocket))
	if err != nil {
		t.Fatal(err)
	}
	removeOnCleanup(t, b)
	if l, err := b.WaitLine(ctx, "listen"); err != nil || !l.OK {
		t.Fatalf("broker listen: %+v %v", l, err)
	}
	return b, brokerMount, workerMount
}

// dialAs runs a one-shot worker as uid:uid, with or without the socket group,
// that dials the broker and reads its reply.
func dialAs(t *testing.T, ctx context.Context, cli *client.Client, lesson, img, role string, uid int, inGroup bool, sock Mount) PeerDial {
	t.Helper()
	s := WorkerSpec(lesson, role, img, uid, uid, DefaultPeerIdentities.SocketGID, sock, "dial", "unix", PeerSocket, "peer\n")
	if !inGroup {
		s.Groups = nil
	}
	res, err := Run(ctx, cli, s)
	if err != nil {
		t.Fatal(err)
	}
	return PeerDial{Role: role, UID: uid, GID: uid, InGroup: inGroup, Stdout: res.Stdout}
}

// stat runs the probe's stat in a running container and returns its detail.
func stat(t *testing.T, ctx context.Context, c *Container, path string) string {
	t.Helper()
	res, err := c.Exec(ctx, FixtureBinary, "stat", path)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := probeout.Find(probeout.Parse(res.Stdout), "stat")
	if !ok || !l.OK {
		t.Fatalf("stat %s: %+v", path, res)
	}
	return l.Detail
}

func TestSpec_H5_BindFailsWithoutInitAndSocketIsSrwxrwxWithIt(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h5-init")
	ids := DefaultPeerIdentities

	// A fresh volume: the image has no /sock, so the volume is root's 0755
	// and the broker (20000:30000) cannot create its socket there.
	raw, err := CreateVolume(ctx, cli, lesson, "raw")
	if err != nil {
		t.Fatal(err)
	}
	removeVolumeOnCleanup(t, cli, raw)
	res, err := Run(ctx, cli, BrokerSpec(lesson, img, ids.BrokerUID, ids.SocketGID,
		Mount{Type: MountVolume, Source: raw, Target: SocketDir}, "peer-echo", "--count", "1", PeerSocket))
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := probeout.Find(probeout.Parse(res.Stdout), "listen"); res.ExitCode != 1 || !ok || l.OK || !strings.Contains(l.Detail, "permission denied") {
		t.Errorf("broker on a fresh volume: %+v, want listen DENIED with permission denied", res)
	} else {
		t.Logf("without the init step: listen: DENIED (%s)", l.Detail)
	}

	// With the init step (SocketVolume builds and removes its own probe image).
	brokerMount, workerMount, err := SocketVolume(ctx, cli, lesson, ids.BrokerUID, ids.SocketGID)
	if err != nil {
		t.Fatal(err)
	}
	removeVolumeOnCleanup(t, cli, brokerMount.Source)
	mustNoImages(t, ctx, cli, lesson)
	if want := (Mount{Type: MountVolume, Source: brokerMount.Source, Target: SocketDir}); brokerMount != want {
		t.Errorf("broker mount %+v, want %+v", brokerMount, want)
	}
	if want := (Mount{Type: MountVolume, Source: brokerMount.Source, Target: SocketDir, ReadOnly: true}); workerMount != want {
		t.Errorf("worker mount %+v, want %+v", workerMount, want)
	}
	b, err := Start(ctx, cli, BrokerSpec(lesson, img, ids.BrokerUID, ids.SocketGID, brokerMount, "peer-echo", PeerSocket))
	if err != nil {
		t.Fatal(err)
	}
	removeOnCleanup(t, b)
	if l, err := b.WaitLine(ctx, "listen"); err != nil || !l.OK {
		t.Fatalf("broker listen after the init step: %+v %v", l, err)
	}
	// umask 007 on the broker gives the socket 0770; the directory is the
	// init step's 2750 owned by broker:socket group.
	if got, want := stat(t, ctx, b, PeerSocket), "/sock/peer.sock mode=srwxrwx--- perm=0770 uid=20000 gid=30000"; got != want {
		t.Errorf("socket: %q, want %q", got, want)
	}
	if got, want := stat(t, ctx, b, SocketDir), "/sock mode=drwxr-s--- perm=2750 uid=20000 gid=30000"; got != want {
		t.Errorf("socket directory: %q, want %q", got, want)
	}
}

// TestSpec_H5_WorkersReportedWithOwnUIDAndPrimaryGID: SO_PEERCRED gives the
// broker each worker's UID and primary GID (the supplementary socket group
// does not appear). The PID is 0 because the kernel translates the peer's PID
// into the reader's PID namespace, and a worker in its own private PID
// namespace has no number in the broker's; a dial from inside the broker's
// own container gets a real PID. So identity rests on UID/GID, never PID.
// The workers connect through the read-only worker mount: connect(2) does not
// write the filesystem.
func TestSpec_H5_WorkersReportedWithOwnUIDAndPrimaryGID(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h5-peers")
	b, _, workerMount := startPeerBroker(t, ctx, cli, lesson, img)
	if !workerMount.ReadOnly {
		t.Fatal("the worker mount should be read-only")
	}

	for i, uid := range DefaultPeerIdentities.Workers {
		d := dialAs(t, ctx, cli, lesson, img, "worker"+strconv.Itoa(i+1), uid, true, workerMount)
		if bad := peerUIDIsAssignedUID(d); len(bad) > 0 {
			t.Error(bad)
		}
		l, _ := probeout.Find(probeout.Parse(d.Stdout), "reply")
		if want := fmt.Sprintf("uid=%d gid=%d pid=0", uid, uid); l.Detail != want {
			t.Errorf("%s: broker reported %q, want %q", d.Role, l.Detail, want)
		}
	}
	stdout, _, err := b.Logs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"peer: OK uid=20001 gid=20001 pid=0", "peer: OK uid=20002 gid=20002 pid=0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("broker output %q lacks %q", stdout, want)
		}
	}

	res, err := b.Exec(ctx, FixtureBinary, "dial", "unix", PeerSocket, "peer\n")
	if err != nil {
		t.Fatal(err)
	}
	l, _ := probeout.Find(probeout.Parse(res.Stdout), "reply")
	_, _, pid, err := parsePeerCred(l.Detail)
	if err != nil || pid == 0 {
		t.Errorf("a dial inside the broker's PID namespace: %q (%v), want a real PID", l.Detail, err)
	} else {
		t.Logf("same PID namespace: %s; other namespaces: pid=0", l.Detail)
	}
}

func TestSpec_H5_WorkerWithoutSocketGroupRefusedAtConnect(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h5-group")
	b, _, workerMount := startPeerBroker(t, ctx, cli, lesson, img)
	uid := DefaultPeerIdentities.Outsider

	out := dialAs(t, ctx, cli, lesson, img, "outsider", uid, false, workerMount)
	if bad := socketGroupGatesConnect(out); len(bad) > 0 {
		t.Error(bad)
	} else {
		t.Logf("without the group: %s", strings.TrimSpace(out.Stdout))
	}
	// The same UID with the group connects, so the group made the difference.
	in := dialAs(t, ctx, cli, lesson, img, "insider", uid, true, workerMount)
	if bad := peerUIDIsAssignedUID(in); len(bad) > 0 {
		t.Errorf("uid %d with the socket group: %v", uid, bad)
	}
	stdout, _, err := b.Logs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(stdout, "peer: OK"); n != 1 {
		t.Errorf("broker accepted %d connections, want 1 (the insider): %q", n, stdout)
	}
}

// TestSpec_H5_WorkerCannotTamperWithSocketOrIdentity runs each attack twice:
// through the read-only worker mount SocketVolume returns (refused with EROFS
// before any permission check), and through a read-write mount of the same
// volume, which shows the directory's 2750 and the socket's ownership hold on
// their own.
func TestSpec_H5_WorkerCannotTamperWithSocketOrIdentity(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h5-tamper")
	b, brokerMount, workerMount := startPeerBroker(t, ctx, cli, lesson, img)
	ids := DefaultPeerIdentities
	me, other := ids.Workers[0], ids.Workers[1]
	before := stat(t, ctx, b, PeerSocket)

	const (
		rofs  = "read-only file system"
		eacc  = "permission denied"
		eperm = "operation not permitted"
	)
	for _, m := range []struct {
		name  string
		mount Mount
		want  map[string]string // probe command -> expected error
	}{
		{"read-only worker mount", workerMount, map[string]string{"unlink": rofs, "replace": rofs, "chown": rofs, "chmod": rofs, "setuid": eperm}},
		{"read-write mount", brokerMount, map[string]string{"unlink": eacc, "replace": eacc, "chown": eperm, "chmod": eperm, "setuid": eperm}},
	} {
		t.Run(m.name, func(t *testing.T) {
			w, err := Start(ctx, cli, WorkerSpec(lesson, "tamper", img, me, me, ids.SocketGID, m.mount, "sleep"))
			if err != nil {
				t.Fatal(err)
			}
			removeOnCleanup(t, w)
			for _, cmd := range [][]string{
				{"unlink", PeerSocket},
				{"replace", PeerSocket},
				{"chown", PeerSocket, owner(me, me)},
				{"chmod", PeerSocket, "0777"},
				{"setuid", strconv.Itoa(other)},
			} {
				res, err := w.Exec(ctx, append([]string{FixtureBinary}, cmd...)...)
				if err != nil {
					t.Fatal(err)
				}
				l, ok := probeout.Find(probeout.Parse(res.Stdout), cmd[0])
				if res.ExitCode != 1 || !ok || l.OK || !strings.Contains(l.Detail, m.want[cmd[0]]) {
					t.Errorf("%v: %+v, want DENIED with %q", cmd, res, m.want[cmd[0]])
				} else {
					t.Logf("%s: DENIED (%s)", cmd[0], l.Detail)
				}
			}
		})
	}
	if after := stat(t, ctx, b, PeerSocket); after != before {
		t.Errorf("the socket changed: %q, then %q", before, after)
	}
	if d := dialAs(t, ctx, cli, lesson, img, "after", me, true, workerMount); len(peerUIDIsAssignedUID(d)) > 0 {
		t.Errorf("the broker no longer answers on its socket: %q", d.Stdout)
	}
}

// TestProp_PeerUID: for random distinct identities (two workers, an outsider,
// the broker and the socket group), peer-uid-is-assigned-uid holds. Each case
// starts five containers, so it is capped at 3 cases unless the caller sets
// -rapid.checks or RAPID_CHECKS.
func TestProp_PeerUID(t *testing.T) {
	ctx, cli := gatedClient(t)
	img := probeImage(t, ctx, cli)
	lesson := sweptLesson(t, cli, "h5-peeruid")
	capRapid(t, 3, 1)
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.SliceOfNDistinct(rapid.IntRange(1, maxID), 5, 5, rapid.ID[int]).Draw(t, "ids")
		ids := PeerIdentities{BrokerUID: n[0], SocketGID: n[1], Workers: [2]int{n[2], n[3]}, Outsider: n[4]}
		dials, err := verifyPeerIdentity(ctx, cli, lesson, img, ids)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range dials {
			if d.InGroup {
				if bad := peerUIDIsAssignedUID(d); len(bad) > 0 {
					t.Fatal(bad)
				}
			}
		}
	})
}
