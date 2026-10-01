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
	// make reads the Makefile in a child process, which go test's cache cannot
	// see; reading it here makes a Makefile edit invalidate the cached result.
	if _, err := os.ReadFile(repoRoot + "/Makefile"); err != nil {
		t.Fatal(err)
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

// TestSpec_H4_IdeaReadinessUnchanged: the ideas the harness will unblock keep
// their readiness wording in docs/lessons/ideas.md until the Mac gate (plan,
// D4). Each idea's Software bullet says "Blocked: ..." or "Ready: ...".
func TestSpec_H4_IdeaReadinessUnchanged(t *testing.T) {
	data, err := os.ReadFile(repoRoot + "/docs/lessons/ideas.md")
	if err != nil {
		t.Fatal(err)
	}
	sections := map[string]string{} // slug -> entry text with whitespace collapsed
	var slug string
	for _, line := range strings.Split(string(data), "\n") {
		if s, ok := strings.CutPrefix(line, "### "); ok {
			slug = strings.TrimSpace(s)
			continue
		}
		sections[slug] += " " + strings.TrimSpace(line)
	}
	for slug, marker := range map[string]string{
		"worker-capabilities":          "Blocked: the Go Docker client",
		"focused-runtime-cell":         "Blocked: harness unconfigured",
		"noisy-neighbor-limits":        "Blocked: Go Docker client unconfigured",
		"container-namespace-boundary": "Ready: Compose expresses both variants",
		"exec-cancel-reap":             "Ready: one Compose service",
		"surrogate-credential-broker":  "Docker isolation harness (blocked)",
	} {
		if !strings.Contains(sections[slug], marker) {
			t.Errorf("idea %q no longer says %q", slug, marker)
		}
	}
}
