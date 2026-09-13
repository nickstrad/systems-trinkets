package counter

import (
	"net/http"
	"os"
	reference "systems-trinkets/examples/counter"
	"systems-trinkets/harness/suitekit"
	"testing"
)

// TestMain runs against the configured target, or in-process against the
// reference memory counter when there is none (so `go test ./...` runs the
// suite rather than skipping it).
func TestMain(m *testing.M) {
	os.Exit(suitekit.Main(m, suitekit.InProcess(func() http.Handler {
		return reference.NewHandler(reference.NewMemory())
	})))
}
