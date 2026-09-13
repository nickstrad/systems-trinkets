// Command harness drives the HTTP invariant suites and reports on their
// results: `run` wraps `go test` with the environment a suite's TestMain
// reads, `report` and `sql` query the Parquet files every run leaves behind,
// `new-suite` scaffolds a suite and `targets` lists the configured SUTs.
//
// See docs/architecture.md for the command surface and results tables.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	os.Exit(dispatch(os.Args[1:]))
}

// dispatch routes the first argument to a subcommand; everything after it is
// that subcommand's own argument list.
func dispatch(args []string) int {
	if len(args) == 0 {
		usage(os.Stdout)
		return 0
	}
	switch args[0] {
	case "run":
		return cmdRun(args[1:])
	case "report":
		return cmdReport(args[1:])
	case "sql":
		return cmdSQL(args[1:])
	case "new-suite":
		return cmdNewSuite(args[1:])
	case "targets":
		return cmdTargets(args[1:])
	case "help", "-h", "--help":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "harness: unknown command %q\n\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

// splitArgs separates positional arguments from flag arguments so that flags
// may be written on either side of them — the flag package otherwise stops at
// the first positional. Every flag in this CLI takes a value, so "-f value"
// consumes two arguments and "-f=value" one; "-" is a positional (stdin).
func splitArgs(args []string) (positional, flags []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) < 2 || a[0] != '-' {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		if isHelpFlag(a) || strings.Contains(a, "=") {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return positional, flags
}

func isHelpFlag(a string) bool {
	switch a {
	case "-h", "--h", "-help", "--help":
		return true
	}
	return false
}

func usage(w io.Writer) {
	fmt.Fprint(w, `harness — HTTP invariant harness for the systems patterns

usage: harness <command> [flags]

commands:
  run        run a suite against one or all targets and report
  report     print a canned query over recorded runs
  sql        run arbitrary DuckDB SQL over recorded runs
  new-suite  scaffold a suite with contract/invariant docs and separate test files
  targets    list targets/*.toml
  help       this message

`)
	usageRun(w)
	fmt.Fprintln(w)
	usageReport(w)
	fmt.Fprintln(w)
	usageSQL(w)
	fmt.Fprintln(w)
	usageNewSuite(w)
	fmt.Fprintln(w)
	usageTargets(w)
}

func usageRun(w io.Writer) {
	fmt.Fprint(w, `harness run <pattern> (--target targets/x.toml | --url http://host:port | --all-targets) [flags] [-- go test flags]

  --all-targets    run targets/<pattern>-*.toml sequentially in filename order
  --target FILE    target file; its pattern must match <pattern>
  --url URL        ad-hoc target, with --language, --engine, --label
  --sut-ref REF    git ref or version of the SUT, recorded on the run

  Runs `+"`go test ./suites/<pattern>/ -count=1 -v`"+` from the module root with
  HARNESS_RUN_ID pre-assigned, then prints the summary report for that run.
  A batch continues through failures and returns the first nonzero exit code.
  Each target gets its own run ID and summary; single runs return go test's code.

  e.g. harness run counter --url http://127.0.0.1:8080 --language go --engine memory -- -run TestContract
`)
}

func usageReport(w io.Writer) {
	fmt.Fprint(w, `harness report [--run last|<run_id>|<a>,<b>] [--query summary|latency|checks|compare|history]
               [--pattern slug] [--format box|md|json] [--runs-dir DIR]

  Runs queries/<query>.sql against views over <runs-dir>/*/<table>.parquet.
  --query compare needs --run <a>,<b>; --query history needs --pattern.
`)
}

func usageSQL(w io.Writer) {
	fmt.Fprint(w, `harness sql "<duckdb sql>" [--format box|md|json] [--runs-dir DIR]

  The views runs, tests, checks, samples, samples_measured (samples minus the
  /_reset + /healthz setup handshake) and metrics are pre-registered over every
  recorded run. Pass "-" to read the statement from stdin.
`)
}

func usageNewSuite(w io.Writer) {
	fmt.Fprint(w, `harness new-suite <pattern> [--dir DIR]

  Scaffolds CONTRACT.md, INVARIANTS.md, main_test.go, contract_test.go,
  and concurrency_test.go. Never overwrites.
`)
}

func usageTargets(w io.Writer) {
	fmt.Fprint(w, `harness targets [--dir DIR] [--format box|md|json]

  Lists targets/*.toml: file, pattern, language, engine, url, label.
`)
}
