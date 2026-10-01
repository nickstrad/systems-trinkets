package docker

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// peerDials is a passing set of dials as the default identities produce
// them: each worker's reply names its own uid and primary gid, PID 0 (the
// broker cannot see the worker's PID namespace), and the outsider is refused.
func peerDials() []PeerDial {
	return []PeerDial{
		{Role: "worker1", UID: 20001, GID: 20001, InGroup: true,
			Stdout: "dial: OK unix /sock/peer.sock\nreply: OK uid=20001 gid=20001 pid=0\n"},
		{Role: "worker2", UID: 20002, GID: 20002, InGroup: true,
			Stdout: "dial: OK unix /sock/peer.sock\nreply: OK uid=20002 gid=20002 pid=0\n"},
		{Role: "outsider", UID: 20003, GID: 20003,
			Stdout: "dial: DENIED (dial unix /sock/peer.sock: connect: permission denied)\n"},
	}
}

// TestSpec_H5_VerifyPeerIdentityPassesAndFailsOnFakedMismatch: the
// comparison VerifyPeerIdentity runs is a pure function, so its failure path
// is tested here with faked broker reports; the gated subtest runs the real
// thing on this engine.
func TestSpec_H5_VerifyPeerIdentityPassesAndFailsOnFakedMismatch(t *testing.T) {
	if bad := checkPeerIdentity(peerDials()); len(bad) > 0 {
		t.Fatalf("the passing set reports %q", bad)
	}
	for _, tc := range []struct {
		name string
		fake func(d []PeerDial) []PeerDial
		want string // a substring of the one violation expected
	}{
		{"user-namespace remap shifts the uid", func(d []PeerDial) []PeerDial {
			// userns-remap with a 100000 base would show 20001 as 120001
			d[0].Stdout = "dial: OK unix /sock/peer.sock\nreply: OK uid=120001 gid=120001 pid=0\n"
			return d
		}, "peer-uid-is-assigned-uid: worker1: the broker read uid 120001, the launcher assigned 20001"},
		{"both workers read as one uid", func(d []PeerDial) []PeerDial {
			d[1].Stdout = "dial: OK unix /sock/peer.sock\nreply: OK uid=20001 gid=20001 pid=0\n"
			return d
		}, "peer-uid-is-assigned-uid: worker2: the broker read uid 20001, the launcher assigned 20002"},
		{"no reply", func(d []PeerDial) []PeerDial {
			d[0].Stdout = "dial: OK unix /sock/peer.sock\nreply: DENIED (EOF)\n"
			return d
		}, "peer-uid-is-assigned-uid: worker1 (uid 20001) got no reply from the broker"},
		{"malformed reply", func(d []PeerDial) []PeerDial {
			d[0].Stdout = "dial: OK unix /sock/peer.sock\nreply: OK uid=20001 gid=20001\n"
			return d
		}, `peer-uid-is-assigned-uid: worker1: peer credentials "uid=20001 gid=20001" are not`},
		{"outsider connected", func(d []PeerDial) []PeerDial {
			d[2].Stdout = "dial: OK unix /sock/peer.sock\nreply: OK uid=20003 gid=20003 pid=0\n"
			return d
		}, "socket-group-gates-connect: outsider (uid 20003) connected without the socket group"},
		{"outsider refused for another reason", func(d []PeerDial) []PeerDial {
			d[2].Stdout = "dial: DENIED (dial unix /sock/peer.sock: connect: no such file or directory)\n"
			return d
		}, "socket-group-gates-connect: outsider (uid 20003) was refused for another reason"},
		{"outsider printed nothing", func(d []PeerDial) []PeerDial {
			d[2].Stdout = ""
			return d
		}, "socket-group-gates-connect: outsider (uid 20003) printed no dial line"},
		{"one worker only", func(d []PeerDial) []PeerDial {
			return slices.Delete(d, 1, 2)
		}, "got 1 UIDs and 1 outside"},
		{"two workers with one uid", func(d []PeerDial) []PeerDial {
			d[1].UID, d[1].Stdout = 20001, d[0].Stdout
			return d
		}, "got 1 UIDs and 1 outside"},
		{"no outsider", func(d []PeerDial) []PeerDial {
			return d[:2]
		}, "got 2 UIDs and 0 outside"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := checkPeerIdentity(tc.fake(peerDials()))
			if len(bad) != 1 || !strings.Contains(bad[0], tc.want) {
				t.Errorf("violations %q, want exactly one containing %q", bad, tc.want)
			}
		})
	}

	t.Run("passes on this engine", func(t *testing.T) {
		ctx, cli := gatedClient(t)
		lesson := sweptLesson(t, cli, "h5-verify")
		dials, err := VerifyPeerIdentity(ctx, cli, lesson, DefaultPeerIdentities)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range dials {
			t.Logf("%s uid=%d in-group=%v: %q", d.Role, d.UID, d.InGroup, d.Stdout)
		}
		mustNoImages(t, ctx, cli, lesson)
	})
}

func TestParsePeerCred(t *testing.T) {
	// The format is exact: what Sprintf gives back for three integers.
	if u, g, p, err := parsePeerCred("uid=20001 gid=30000 pid=0"); err != nil || u != 20001 || g != 30000 || p != 0 {
		t.Errorf("parsePeerCred = %d %d %d %v", u, g, p, err)
	}
	for _, s := range []string{
		"", "uid=1 gid=2", "uid=1 gid=2 pid=3 extra", "uid=+1 gid=2 pid=3", "uid=01 gid=2 pid=3",
		"uid=1  gid=2 pid=3", "gid=2 uid=1 pid=3", "uid=1 gid=2 pid=3\n",
	} {
		if _, _, _, err := parsePeerCred(s); err == nil {
			t.Errorf("parsePeerCred(%q) accepted it", s)
		}
	}
}

// peerCredRE is the oracle for FuzzParsePeerCred: the texts fmt's %d writes
// for an int (no sign on zero, no leading zeros, no plus), three times.
var peerCredRE = regexp.MustCompile(`^uid=(0|-?[1-9][0-9]*) gid=(0|-?[1-9][0-9]*) pid=(0|-?[1-9][0-9]*)$`)

// FuzzParsePeerCred: parsePeerCred accepts s exactly when s is what
// formatting three ints gives back ("uid=U gid=G pid=P"), and then returns
// those ints.
func FuzzParsePeerCred(f *testing.F) {
	for _, s := range []string{
		"uid=20001 gid=20001 pid=0", "uid=0 gid=0 pid=0", "uid=-1 gid=2 pid=3", "uid=+1 gid=2 pid=3",
		"uid=01 gid=2 pid=3", "uid=1 gid=2", "uid=1 gid=2 pid=3 ", "uid=1  gid=2 pid=3", "uid=-0 gid=0 pid=0",
		"uid=99999999999999999999 gid=0 pid=0", "uid=1 gid=2 pid=3\n", "",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		uid, gid, pid, err := parsePeerCred(s)
		m := peerCredRE.FindStringSubmatch(s)
		want := m != nil
		var n [3]int
		for i := range n {
			if want {
				v, aerr := strconv.Atoi(m[i+1])
				want, n[i] = aerr == nil, v // out of int range does not round-trip
			}
		}
		if got := err == nil; got != want {
			t.Fatalf("parsePeerCred(%q) accepted=%v, the format says %v (err %v)", s, got, want, err)
		}
		if err == nil && (uid != n[0] || gid != n[1] || pid != n[2] || fmt.Sprintf("uid=%d gid=%d pid=%d", uid, gid, pid) != s) {
			t.Fatalf("parsePeerCred(%q) = %d %d %d", s, uid, gid, pid)
		}
	})
}

func TestPeerIdentitiesMustBeDistinctAndNonZero(t *testing.T) {
	if err := DefaultPeerIdentities.validate(); err != nil {
		t.Fatal(err)
	}
	for _, fake := range []func(*PeerIdentities){
		func(p *PeerIdentities) { p.Outsider = p.SocketGID }, // its primary group would be the socket group
		func(p *PeerIdentities) { p.Workers[1] = p.Workers[0] },
		func(p *PeerIdentities) { p.Workers[0] = 0 },
		func(p *PeerIdentities) { p.BrokerUID = maxID + 1 },
		func(p *PeerIdentities) { p.SocketGID = -1 },
	} {
		p := DefaultPeerIdentities
		fake(&p)
		if err := p.validate(); err == nil {
			t.Errorf("%+v was accepted", p)
		}
	}
}
