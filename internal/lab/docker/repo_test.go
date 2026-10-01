package docker

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Checks on repo files outside the package: the Makefile's help text and the
// software catalog. No daemon. Tests run in the package directory.
const repoRoot = "../../.."

// TestSpec_H4_MakeHelpListsCleanHarness: `make help` prints the header
// comment, so a target missing from it is undocumented.
func TestSpec_H4_MakeHelpListsCleanHarness(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make is not installed")
	}
	out, err := exec.Command("make", "-C", repoRoot, "help").CombinedOutput()
	if err != nil {
		t.Fatalf("make help: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "make clean-harness") {
		t.Errorf("make help does not list clean-harness:\n%s", out)
	}
}

// TestSpec_H4_CatalogRowsUnchanged: the catalog rows for the harness stay
// `no` in the Configured column until the Mac gate (plan, D4) passes.
func TestSpec_H4_CatalogRowsUnchanged(t *testing.T) {
	data, err := os.ReadFile(repoRoot + "/software/software.md")
	if err != nil {
		t.Fatal(err)
	}
	// Columns: Software | Layer | Use case | Configured | Setup.
	const configured = 4
	seen := map[string]string{"Docker Engine API": "", "Docker isolation harness": "", "cgroups v2": ""}
	for _, line := range strings.Split(string(data), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < configured+2 {
			continue
		}
		name := strings.TrimSpace(cells[1])
		if _, watched := seen[name]; watched {
			seen[name] = strings.TrimSpace(cells[configured])
		}
	}
	for name, got := range seen {
		if got != "no" {
			t.Errorf("software.md row %q: Configured = %q, want \"no\" until the Mac gate", name, got)
		}
	}
}
