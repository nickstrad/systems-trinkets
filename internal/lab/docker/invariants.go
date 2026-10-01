package docker

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

// Named invariants. Each is a small function that returns the violations it
// finds, empty when it holds. The code that does the work calls them before
// acting, and the tests call the same functions, so the two cannot drift.

// InvContextNamesClean: every name in a fixture build context is relative and
// clean (path.Clean leaves it unchanged), has no "..", backslash, NUL or
// trailing slash, and is valid UTF-8. A name outside this set could write
// outside the context when the daemon unpacks it. Names are generated, never
// taken from a lesson's paths, so this holds by construction; writeTar checks
// it anyway.
const InvContextNamesClean = "context-names-clean"

func contextNamesClean(files []contextFile) []string {
	var bad []string
	seen := map[string]bool{}
	for _, f := range files {
		n := f.Name
		switch {
		case n == "" || n == ".":
			bad = append(bad, fmt.Sprintf("%s: empty name %q", InvContextNamesClean, n))
		case path.IsAbs(n), path.Clean(n) != n, n == ".." || strings.HasPrefix(n, "../"):
			bad = append(bad, fmt.Sprintf("%s: %q is not relative and clean", InvContextNamesClean, n))
		case strings.ContainsAny(n, "\\\x00"), !utf8.ValidString(n):
			bad = append(bad, fmt.Sprintf("%s: %q has a backslash, NUL or invalid UTF-8", InvContextNamesClean, n))
		case seen[n]:
			bad = append(bad, fmt.Sprintf("%s: %q appears twice", InvContextNamesClean, n))
		}
		seen[n] = true
	}
	return bad
}
