package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
)

// optString is a string flag that remembers whether it was given. Edit commands
// write only the fields the user actually passed.
type optString struct {
	val string
	set bool
}

func (s *optString) String() string { return s.val }

func (s *optString) Set(v string) error {
	s.val, s.set = v, true
	return nil
}

// multiString collects a repeatable flag, e.g. --invariant A --invariant B.
type multiString struct {
	vals []string
	set  bool
}

func (m *multiString) String() string { return strings.Join(m.vals, ", ") }

func (m *multiString) Set(v string) error {
	m.vals = append(m.vals, v)
	m.set = true
	return nil
}

// optInt is the same idea for integer flags.
type optInt struct {
	val int64
	set bool
}

// optPositiveInt is an optional integer flag whose value must be greater than
// zero. It is used for curriculum positions, where zero and negative values
// have no useful meaning and should be rejected before touching the database.
type optPositiveInt struct {
	val int
	set bool
}

func (i *optPositiveInt) String() string {
	if !i.set {
		return ""
	}
	return strconv.Itoa(i.val)
}

func (i *optPositiveInt) Set(v string) error {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return fmt.Errorf("%q is not a positive integer", v)
	}
	i.val, i.set = n, true
	return nil
}

func (i *optInt) String() string {
	if !i.set {
		return ""
	}
	return fmt.Sprint(i.val)
}

func (i *optInt) Set(v string) error {
	n, err := parseID(v)
	if err != nil {
		return err
	}
	i.val, i.set = n, true
	return nil
}

func parseID(v string) (int64, error) {
	var n int64
	if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err != nil {
		return 0, fmt.Errorf("%q is not an id", v)
	}
	return n, nil
}

func newTabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// field prints one "Label: value" line, indenting any wrapped body text.
func field(w io.Writer, label, val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	if strings.Contains(val, "\n") {
		fmt.Fprintf(w, "%s:\n", label)
		for _, line := range strings.Split(strings.TrimRight(val, "\n"), "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
		return
	}
	fmt.Fprintf(w, "%-12s %s\n", label+":", val)
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func curriculumOrderText(order *int) string {
	if order == nil {
		return "-"
	}
	return strconv.Itoa(*order)
}

// usageFor prints a subcommand's flags after a short synopsis.
func usageFor(fs *flag.FlagSet, synopsis string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "usage: %s\n\n", synopsis)
		fs.PrintDefaults()
	}
}
