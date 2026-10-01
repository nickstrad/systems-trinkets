package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
	"pgregory.net/rapid"
)

// setHost sets DOCKER_HOST for the test; "" unsets it entirely.
func setHost(t *testing.T, v string) {
	t.Helper()
	t.Setenv(client.EnvOverrideHost, v) // restores the original at cleanup
	if v == "" {
		os.Unsetenv(client.EnvOverrideHost)
	}
}

// stubContext replaces the docker CLI lookup and reports how often it ran.
func stubContext(t *testing.T, answer string) *int {
	t.Helper()
	calls := 0
	old := contextHost
	contextHost = func() string { calls++; return answer }
	t.Cleanup(func() { contextHost = old })
	return &calls
}

// requires returns every module path the main module requires, mapped to
// whether it is direct (no "// indirect" marker), as reported by go mod edit
// -json (run against the go.mod that go env GOMOD names, so the test works
// from any directory).
func requires(t *testing.T) map[string]bool {
	t.Helper()
	gomod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	path := strings.TrimSpace(string(gomod))
	if path == "" || path == os.DevNull {
		t.Fatal("not inside a module")
	}
	out, err := exec.Command("go", "mod", "edit", "-json", path).Output()
	if err != nil {
		t.Fatalf("go mod edit -json: %v", err)
	}
	var mod struct {
		Require []struct {
			Path     string
			Indirect bool
		}
	}
	if err := json.Unmarshal(out, &mod); err != nil {
		t.Fatal(err)
	}
	all := map[string]bool{}
	for _, r := range mod.Require {
		all[r.Path] = !r.Indirect
	}
	return all
}

func TestSpec_H1_GoModDirectRequirements(t *testing.T) {
	reqs := requires(t)
	for _, mod := range []string{"github.com/moby/moby/client", "github.com/moby/moby/api"} {
		if !reqs[mod] {
			t.Errorf("go.mod does not list %s as a direct requirement (run go mod tidy after the import exists)", mod)
		}
	}
	// Absent from every Require entry, direct or indirect.
	if _, present := reqs["github.com/docker/docker"]; present {
		t.Error("go.mod requires github.com/docker/docker; use github.com/moby/moby/client")
	}
}

func TestSpec_H1_HostPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		env        string // "" = DOCKER_HOST unset
		ctx        string // what the context lookup answers; "" = lookup failed
		want       string
		wantLookup bool
	}{
		{"env wins over context", "tcp://10.0.0.1:2375", "unix:///run/user/1000/docker.sock", "tcp://10.0.0.1:2375", false},
		{"env wins when context lookup fails", "unix:///tmp/x.sock", "", "unix:///tmp/x.sock", false},
		{"context used when env unset", "", "unix:///Users/me/.docker/run/docker.sock", "unix:///Users/me/.docker/run/docker.sock", true},
		{"default when env unset and lookup fails", "", "", client.DefaultDockerHost, true},
		{"default when context answer is unparseable", "", "<no value>", client.DefaultDockerHost, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setHost(t, tc.env)
			calls := stubContext(t, tc.ctx)
			if got := Host(); got != tc.want {
				t.Errorf("Host() = %q, want %q", got, tc.want)
			}
			if (*calls > 0) != tc.wantLookup {
				t.Errorf("context lookup calls = %d, want lookup=%v", *calls, tc.wantLookup)
			}
		})
	}
}

func TestSpec_H1_GatedPingReportsVersionAndArch(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cli := Connect(ctx)
	defer cli.Close()
	ping, err := cli.Ping(ctx, client.PingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	arch := EngineArch(ctx, cli)
	if arch != "amd64" && arch != "arm64" {
		t.Errorf("EngineArch = %q", arch)
	}
	t.Logf("host=%s api=%s os=%s engine arch=%s", Host(), ping.APIVersion, ping.OSType, arch)
}

// mustPanicWithError runs f and returns the error it panicked with, failing
// if it returned or did not finish within limit.
func mustPanicWithError(t *testing.T, limit time.Duration, f func()) error {
	t.Helper()
	got := make(chan any, 1)
	go func() {
		defer func() { got <- recover() }()
		f()
	}()
	select {
	case r := <-got:
		err, ok := r.(error)
		if !ok {
			t.Fatalf("Connect did not panic with an error, got %v (%T)", r, r)
		}
		return err
	case <-time.After(limit):
		t.Fatalf("Connect still running after %v: it hangs instead of failing fast", limit)
		return nil
	}
}

func TestSpec_H1_ConnectPanicsWhenDaemonUnreachable(t *testing.T) {
	t.Run("missing socket", func(t *testing.T) {
		sock := filepath.Join(t.TempDir(), "docker.sock")
		setHost(t, "unix://"+sock)
		err := mustPanicWithError(t, 10*time.Second, func() { Connect(context.Background()) })
		if !strings.Contains(err.Error(), sock) {
			t.Errorf("panic %q does not name the socket %s", err, sock)
		}
		if !client.IsErrConnectionFailed(err) {
			t.Errorf("panic %v is not a connection failure", err)
		}
		if !errors.Is(err, syscall.ENOENT) {
			t.Errorf("panic %v does not wrap ENOENT", err)
		}
	})
	t.Run("daemon accepts but never answers", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() { // hold connections open without replying
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				defer c.Close()
			}
		}()
		old := pingTimeout
		pingTimeout = 300 * time.Millisecond
		t.Cleanup(func() { pingTimeout = old })
		setHost(t, "tcp://"+ln.Addr().String())
		start := time.Now()
		err = mustPanicWithError(t, 10*time.Second, func() { Connect(context.Background()) })
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("panic %v is not a deadline error", err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("took %v to give up; pingTimeout is %v", d, pingTimeout)
		}
	})
}

func TestSpec_H1_ArchMapping(t *testing.T) {
	tests := []struct {
		in   string
		want string // "" = error
	}{
		{"x86_64", "amd64"},
		{"aarch64", "arm64"},
		{"", ""},
		{"amd64", ""},
		{"arm64", ""},
		{"armv7l", ""},
		{"riscv64", ""},
		{"X86_64", ""},
		{"x86_64 ", ""},
		{"aarch64\n", ""},
		{"x86_64_v2", ""},
	}
	for _, tc := range tests {
		got, err := archToGOARCH(tc.in)
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Errorf("archToGOARCH(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if got, err := archFromInfo(system.Info{Architecture: "aarch64"}); got != "arm64" || err != nil {
		t.Errorf("archFromInfo(aarch64) = %q, %v", got, err)
	}
}

// checkArch is the property both FuzzArch and TestProp_Arch assert: the
// mapping answers amd64 for exactly "x86_64", arm64 for exactly "aarch64",
// and an error with an empty result for everything else.
func checkArch(in string) error {
	got, err := archToGOARCH(in)
	switch {
	case in == "x86_64":
		if got != "amd64" || err != nil {
			return errors.New("x86_64 must map to amd64")
		}
	case in == "aarch64":
		if got != "arm64" || err != nil {
			return errors.New("aarch64 must map to arm64")
		}
	default:
		if err == nil || got != "" {
			return errors.New("unknown architecture must be an error with an empty result, got " + got)
		}
	}
	return nil
}

func propArch(t *rapid.T) {
	in := rapid.OneOf(
		rapid.SampledFrom([]string{"x86_64", "aarch64", "amd64", "arm64", "X86_64", "AARCH64", "x86_64\n", " aarch64", "i686", "armv7l", "s390x"}),
		rapid.String(),
		rapid.StringMatching(`(x86|aarch|arm|amd)[0-9_a-z]{0,6}`),
	).Draw(t, "kernel arch")
	if err := checkArch(in); err != nil {
		t.Fatalf("%q: %v", in, err)
	}
}

func TestProp_Arch(t *testing.T) { rapid.Check(t, propArch) }

func FuzzArch(f *testing.F) {
	for _, s := range []string{"x86_64", "aarch64", "", "amd64", "arm64", "X86_64", "x86_64\n", "aarch64\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if err := checkArch(in); err != nil {
			t.Fatalf("%q: %v", in, err)
		}
	})
}
