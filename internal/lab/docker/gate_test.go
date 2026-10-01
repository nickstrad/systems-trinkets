package docker

import (
	"os"
	"testing"
)

// requireDocker skips a test that needs a daemon unless TRINKETS_DOCKER=1.
// make check-docker sets it; make test and make fuzz never do.
func requireDocker(t testing.TB) {
	t.Helper()
	if os.Getenv("TRINKETS_DOCKER") != "1" {
		t.Skip("needs a Docker daemon: set TRINKETS_DOCKER=1 (make check-docker)")
	}
}
