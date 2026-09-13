package suitekit

import (
	"os"
	"path/filepath"
)

// RunsDir is where runs are written: <module>/artifacts/runs/<run_id>/.
func RunsDir() string { return Path("artifacts", "runs") }

// Path joins elem onto the module root.
func Path(elem ...string) string { return filepath.Join(append([]string{ModuleRoot()}, elem...)...) }

// ModuleRoot walks up from the working directory to the nearest go.mod.
// Falls back to the working directory.
func ModuleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}
