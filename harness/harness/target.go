package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Environment variables a suite's TestMain reads. cmd/harness sets them
// (TargetEnv for the target ones); TargetFromEnv and Main read them.
const (
	EnvTarget   = "HARNESS_TARGET"   // path to a targets/*.toml file; wins over EnvURL
	EnvURL      = "HARNESS_URL"      // ad-hoc target base URL
	EnvPattern  = "HARNESS_PATTERN"  // with EnvURL; default: current directory name
	EnvLanguage = "HARNESS_LANGUAGE" // with EnvURL; default "?"
	EnvEngine   = "HARNESS_ENGINE"   // with EnvURL; default "?"
	EnvLabel    = "HARNESS_LABEL"    // with EnvURL
	EnvRunID    = "HARNESS_RUN_ID"   // pre-assigned run id; default results.NewRunID()
	EnvResults  = "HARNESS_RESULTS"  // results directory; default RunsDir()/<run_id>
	EnvSUTRef   = "HARNESS_SUT_REF"  // free-text SUT version, recorded on the run
)

// LoadTarget reads a targets/<name>.toml file. Relative paths inside it (Cwd)
// are left as written; the sut package resolves them in phase 2.
func LoadTarget(path string) (Target, error) {
	var t Target
	md, err := toml.DecodeFile(path, &t)
	if err != nil {
		return t, fmt.Errorf("target %s: %w", path, err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		return t, fmt.Errorf("target %s: unknown keys %v", path, u)
	}
	if t.URL == "" {
		return t, fmt.Errorf("target %s: url is required", path)
	}
	if t.Pattern == "" {
		return t, fmt.Errorf("target %s: pattern is required", path)
	}
	return t, nil
}

// TargetFromEnv builds the Target for this process from EnvTarget (the file
// wins) or EnvURL plus the ad-hoc variables; with EnvURL the pattern defaults
// to the current directory name, since go test runs with cwd = the suite
// package. ok is false when neither variable is set, in which case suites skip.
func TargetFromEnv() (t Target, ok bool, err error) {
	if path := os.Getenv(EnvTarget); path != "" {
		t, err = LoadTarget(path)
		return t, err == nil, err
	}
	u := os.Getenv(EnvURL)
	if u == "" {
		return Target{}, false, nil
	}
	t = Target{
		URL:      strings.TrimRight(u, "/"),
		Pattern:  os.Getenv(EnvPattern),
		Language: envOr(EnvLanguage, "?"),
		Engine:   envOr(EnvEngine, "?"),
		Label:    os.Getenv(EnvLabel),
	}
	if t.Pattern == "" {
		wd, _ := os.Getwd()
		t.Pattern = filepath.Base(wd)
	}
	return t, true, nil
}

// TargetEnv is the inverse of TargetFromEnv for an ad-hoc target: the
// KEY=VALUE pairs that make a child go test see t. Empty fields are omitted
// so TargetFromEnv applies its defaults.
func TargetEnv(t Target) []string {
	env := []string{EnvURL + "=" + t.URL, EnvPattern + "=" + t.Pattern}
	for _, kv := range [][2]string{{EnvLanguage, t.Language}, {EnvEngine, t.Engine}, {EnvLabel, t.Label}} {
		if kv[1] != "" {
			env = append(env, kv[0]+"="+kv[1])
		}
	}
	return env
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
