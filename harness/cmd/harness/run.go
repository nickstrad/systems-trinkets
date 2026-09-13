package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"systems-trinkets/harness/harness"
	"systems-trinkets/harness/results"
)

// runFlags describes the target a run is pointed at.
type runFlags struct {
	allTargets bool
	target     string
	url        string
	language   string
	engine     string
	label      string
	sutRef     string
}

// cmdRun wraps `go test ./suites/<pattern>/` with the environment a suite's
// TestMain reads, then reports the run it produced. Its exit code is go test's.
func cmdRun(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		usageRun(os.Stderr)
		return 2
	}
	// Keep the plan's flag-first batch spelling as well as the normal
	// pattern-first CLI spelling. Everything after -- remains go test flags.
	if args[0] == "--all-targets" && len(args) > 1 {
		args = append([]string{args[1], args[0]}, args[2:]...)
	}
	pattern := args[0]
	if !slugRE.MatchString(pattern) {
		fmt.Fprintf(os.Stderr, "harness run: bad pattern slug %q\n", pattern)
		return 2
	}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.Usage = func() { usageRun(fs.Output()) }
	f := &runFlags{}
	fs.BoolVar(&f.allTargets, "all-targets", false, "run every matching target in filename order")
	fs.StringVar(&f.target, "target", "", "target file, e.g. targets/counter-go-valkey.toml")
	fs.StringVar(&f.url, "url", "", "SUT base URL, e.g. http://127.0.0.1:8080")
	fs.StringVar(&f.language, "language", "", "language of the SUT (with --url)")
	fs.StringVar(&f.engine, "engine", "", "backing engine of the SUT (with --url)")
	fs.StringVar(&f.label, "label", "", "free-text label shown in reports (with --url)")
	fs.StringVar(&f.sutRef, "sut-ref", "", "git ref or version of the SUT, recorded on the run")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	suiteDir := harness.Path("suites", pattern)
	if _, err := os.Stat(suiteDir); err != nil {
		fmt.Fprintf(os.Stderr, "harness run: no suite %s (%s); scaffold one with `harness new-suite %s`\n",
			pattern, suiteDir, pattern)
		return 2
	}

	targets, err := runTargets(f, pattern, harness.Path("targets"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness run:", err)
		return 2
	}
	return runSequence(targets, func(target runFlags) int {
		return runTarget(&target, pattern, fs.Args())
	})
}

// runTargets validates the entire batch before any process is started.
func runTargets(f *runFlags, pattern, dir string) ([]runFlags, error) {
	if !f.allTargets {
		if _, err := targetEnv(f, pattern); err != nil {
			return nil, err
		}
		return []runFlags{*f}, nil
	}
	if f.target != "" || f.url != "" {
		return nil, errors.New("--all-targets cannot be combined with --target or --url")
	}
	paths, err := filepath.Glob(filepath.Join(dir, pattern+"-*.toml"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no targets for pattern %q in %s", pattern, dir)
	}
	targets := make([]runFlags, 0, len(paths))
	for _, path := range paths {
		target := *f
		target.allTargets = false
		target.target = path
		if _, err := targetEnv(&target, pattern); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func runSequence(targets []runFlags, run func(runFlags) int) int {
	code := 0
	for _, target := range targets {
		if next := run(target); code == 0 && next != 0 {
			code = next
		}
	}
	return code
}

func runTarget(f *runFlags, pattern string, testArgs []string) int {
	env, err := targetEnv(f, pattern)
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness run:", err)
		return 2
	}
	runID := results.NewRunID()
	runsDir := harness.RunsDir()
	resultDir := filepath.Join(runsDir, runID)
	env = append(env,
		harness.EnvRunID+"="+runID,
		harness.EnvResults+"="+resultDir,
		harness.EnvSUTRef+"="+f.sutRef,
	)

	goArgs := append([]string{"test", "./suites/" + pattern + "/", "-count=1", "-v"}, testArgs...)
	cmd := exec.Command("go", goArgs...)
	cmd.Dir = harness.ModuleRoot()
	cmd.Env = runEnvironment(os.Environ(), env)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Fprintf(os.Stderr, "harness: run %s: go %s\n", runID, strings.Join(goArgs, " "))

	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else {
			fmt.Fprintln(os.Stderr, "harness run: go test:", err)
			return 1
		}
	}

	fmt.Fprintf(os.Stdout, "\nrun %s\nresults %s\n\n", runID, resultDir)
	// Keyed on runs.parquet, not on resultDir: Main creates the directory
	// early, before the health wait, to put sut.log in it, so a run that
	// failed fast still has a directory and would otherwise print an empty
	// summary table instead of saying nothing was recorded.
	if _, err := os.Stat(filepath.Join(resultDir, "runs.parquet")); err != nil {
		fmt.Fprintln(os.Stderr, "harness: no results recorded (the suite never opened a sink)")
		return code
	}
	if err := report(os.Stdout, runsDir, "summary", runID, pattern, "box"); err != nil {
		fmt.Fprintln(os.Stderr, "harness run: report:", err)
	}
	return code
}

// targetEnv turns --target or --url into the variables harness.TargetFromEnv
// reads. Exactly one of the two is required.
func targetEnv(f *runFlags, pattern string) ([]string, error) {
	switch {
	case f.target != "" && f.url != "":
		return nil, errors.New("pass either --target or --url, not both")
	case f.target != "":
		path, err := filepath.Abs(f.target)
		if err != nil {
			return nil, fmt.Errorf("resolve --target: %w", err)
		}
		t, err := harness.LoadTarget(path)
		if err != nil {
			return nil, err
		}
		if t.Pattern != pattern {
			return nil, fmt.Errorf("target %s is for pattern %q, not %q", f.target, t.Pattern, pattern)
		}
		return []string{harness.EnvTarget + "=" + path}, nil
	case f.url != "":
		return harness.TargetEnv(harness.Target{URL: f.url, Pattern: pattern, Language: f.language, Engine: f.engine, Label: f.label}), nil
	default:
		return nil, errors.New("need --target <file.toml> or --url <http://…>")
	}
}

// Prevent a shell's HARNESS_TARGET from overriding an explicit --url, or
// stale ad-hoc metadata from leaking into the selected run.
func runEnvironment(parent, selected []string) []string {
	keys := map[string]bool{}
	for _, key := range []string{harness.EnvTarget, harness.EnvURL, harness.EnvPattern,
		harness.EnvLanguage, harness.EnvEngine, harness.EnvLabel, harness.EnvRunID,
		harness.EnvResults, harness.EnvSUTRef} {
		keys[key] = true
	}
	env := make([]string, 0, len(parent)+len(selected))
	for _, entry := range parent {
		key, _, _ := strings.Cut(entry, "=")
		if !keys[key] {
			env = append(env, entry)
		}
	}
	return append(env, selected...)
}
