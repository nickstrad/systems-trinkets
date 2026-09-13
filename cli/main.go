// Command trinkets keeps notes on systems patterns: what a pattern is, how each
// storage engine can build it, and what happened when I actually tried.
//
//	trinkets pattern list
//	trinkets approach show 12
//	trinkets attempt add --pattern fifo-queue --engine postgres --title "skip locked worker"
package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usage = `trinkets - notes on systems patterns across storage engines

usage: trinkets [--db PATH] <command> [<args>]

commands:
  pattern    list|show|add|edit|rm    a system behavior to build
  engine     list|show|add|edit|rm    a storage engine to build it on
  approach   list|show|add|edit|rm    a writeup of pattern-on-engine
  attempt    list|show|add|edit|rm    a record of trying it, and lessons
  matrix                              the pattern x engine core map
  seed                                load the patterns and engines from docs/
  version

The database is ./trinkets.db unless --db or $TRINKETS_DB says otherwise;
it is created on first use. Run "trinkets <command> -h" for that command's
flags, and "trinkets <command> <verb> -h" for a verb's flags.
`

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "trinkets: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	global := flag.NewFlagSet("trinkets", flag.ContinueOnError)
	global.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	dbPath := global.String("db", defaultDBPath(), "path to the SQLite database")
	if err := global.Parse(args); err != nil {
		return err
	}
	rest := global.Args()
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("no command given")
	}

	cmd, cmdArgs := rest[0], rest[1:]
	if cmd == "help" {
		fmt.Print(usage)
		return nil
	}
	if cmd == "version" {
		fmt.Println("trinkets " + version)
		return nil
	}

	// Validate seed modes before opening the source: dry-run must not migrate it.
	if cmd == "seed" {
		opts, err := parseSeedOptions(cmdArgs)
		if err != nil {
			return err
		}
		if opts.dryRun {
			return dryRunSeed(*dbPath)
		}
	}

	db, err := openDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	switch cmd {
	case "pattern", "patterns":
		return cmdPattern(db, cmdArgs)
	case "engine", "engines":
		return cmdEngine(db, cmdArgs)
	case "approach", "approaches":
		return cmdApproach(db, cmdArgs)
	case "attempt", "attempts":
		return cmdAttempt(db, cmdArgs)
	case "matrix":
		return cmdMatrix(db, cmdArgs)
	case "seed":
		return cmdSeed(db, cmdArgs)
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func defaultDBPath() string {
	if p := os.Getenv("TRINKETS_DB"); p != "" {
		return p
	}
	return filepath.Join(".", "trinkets.db")
}

// verb pulls the sub-verb off a resource command's arguments.
func verb(args []string, resource string, verbs string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("usage: trinkets %s <%s> [flags]", resource, verbs)
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintf(os.Stderr, "usage: trinkets %s <%s> [flags]\n", resource, verbs)
		return "", nil, flag.ErrHelp
	}
	rest := args[1:]
	switch args[0] {
	case "show", "edit", "update", "rm", "delete":
		rest = hoistPositional(rest)
	}
	return args[0], rest, nil
}

// hoistPositional moves a leading id or slug to the end of the argument list.
// Go's flag package stops parsing at the first non-flag argument, so without
// this "edit fifo-queue --notes x" would treat the flags as positional junk.
func hoistPositional(args []string) []string {
	if len(args) < 2 || strings.HasPrefix(args[0], "-") {
		return args
	}
	out := make([]string, 0, len(args))
	return append(append(out, args[1:]...), args[0])
}

// idArg reads a positional id, e.g. "trinkets approach show 12".
func idArg(fs *flag.FlagSet, what string) (int64, error) {
	if fs.NArg() != 1 {
		return 0, fmt.Errorf("expected exactly one %s id", what)
	}
	return parseID(fs.Arg(0))
}

// slugArg reads a positional slug, e.g. "trinkets pattern show fifo-queue".
func slugArg(fs *flag.FlagSet, what string) (string, error) {
	if fs.NArg() != 1 {
		return "", fmt.Errorf("expected exactly one %s slug", what)
	}
	return fs.Arg(0), nil
}

// countRows answers the "how much would this delete?" questions.
func countRows(db *sql.DB, query string, args ...any) (int, error) {
	var n int
	err := db.QueryRow(query, args...).Scan(&n)
	return n, err
}
