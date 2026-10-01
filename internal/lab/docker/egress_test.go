package docker

import (
	"strings"
	"testing"
)

// The two H6 invariants are pure over what the probe reports; the gated
// tests feed them real interface lists and counts.
func TestInvariantWorkerHasNoRoute(t *testing.T) {
	for _, tc := range []struct {
		ifaces []string
		bad    bool
	}{
		{[]string{"lo"}, false},
		{nil, true},                    // no lo at all is not the network-none shape
		{[]string{"lo", "eth0"}, true}, // the broker's list
		{[]string{"eth0"}, true},
		{[]string{"lo", "lo"}, true},
	} {
		got := workerHasNoRoute(tc.ifaces)
		if (len(got) > 0) != tc.bad {
			t.Errorf("workerHasNoRoute(%q) = %q, want violation %v", tc.ifaces, got, tc.bad)
		}
		for _, v := range got {
			if !strings.HasPrefix(v, InvWorkerHasNoRoute+": ") {
				t.Errorf("violation %q does not name the invariant", v)
			}
		}
	}
}

func TestInvariantBlockedFixtureUntouched(t *testing.T) {
	if got := blockedFixtureUntouched(0); got != nil {
		t.Errorf("0 requests: %q", got)
	}
	// One request: one violation, worded with the invariant name and count.
	want := "blocked-fixture-untouched: the blocked fixture counted 1 requests, want 0"
	if got := blockedFixtureUntouched(1); len(got) != 1 || got[0] != want {
		t.Errorf("1 request: %q, want [%q]", got, want)
	}
}
