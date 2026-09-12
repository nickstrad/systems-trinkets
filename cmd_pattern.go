package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
)

func cmdPattern(db *sql.DB, args []string) error {
	v, rest, err := verb(args, "pattern", "list|show|add|edit|rm")
	if err != nil {
		return err
	}
	switch v {
	case "list", "ls":
		return patternList(db, rest)
	case "show":
		return patternShow(db, rest)
	case "add", "new":
		return patternAdd(db, rest)
	case "edit", "update":
		return patternEdit(db, rest)
	case "rm", "delete":
		return patternRM(db, rest)
	default:
		return fmt.Errorf("unknown pattern verb %q", v)
	}
}

// patternFlags holds the editable columns. On add, slug and name are
// required; on edit, every flag is optional and only what is passed is written.
// The list flags (--invariant, --reading) repeat; passing one on edit replaces
// the whole list.
type patternFlags struct {
	text                 map[string]*optString
	invariants           multiString
	readings             multiString
	curriculumOrder      optPositiveInt
	clearCurriculumOrder bool
}

func newPatternFlags(fs *flag.FlagSet) *patternFlags {
	f := &patternFlags{text: map[string]*optString{}}
	for name, help := range map[string]string{
		"slug":        "short stable identifier, e.g. fifo-queue",
		"name":        "human name, e.g. FIFO work queue",
		"family":      "grouping, e.g. queueing, coordination, limiting",
		"explanation": "what the pattern is, in plain words (a few sentences)",
		"use_cases":   "the problems it solves and where it shows up",
		"notes":       "anything else worth keeping",
	} {
		o := &optString{}
		fs.Var(o, name, help)
		f.text[name] = o
	}
	fs.Var(&f.invariants, "invariant", "an invariant the system must preserve (repeat for several)")
	fs.Var(&f.readings, "reading", "a helpful link as 'Title - URL' (repeat for several)")
	fs.Var(&f.curriculumOrder, "curriculum-order", "positive position in the recommended curriculum")
	fs.BoolVar(&f.clearCurriculumOrder, "clear-curriculum-order", false, "remove the curriculum position")
	return f
}

func (f *patternFlags) validate() error {
	if f.curriculumOrder.set && f.clearCurriculumOrder {
		return fmt.Errorf("--curriculum-order and --clear-curriculum-order are mutually exclusive")
	}
	return nil
}

func (f *patternFlags) fields() (*fields, error) {
	out := &fields{}
	for _, col := range []string{"slug", "name", "family", "explanation", "use_cases", "notes"} {
		if f.text[col].set {
			out.set(col, f.text[col].val)
		}
	}
	if f.invariants.set {
		v, err := jsonText(f.invariants.vals)
		if err != nil {
			return nil, err
		}
		out.set("invariants", v)
	}
	if f.readings.set {
		rs := make([]Reading, 0, len(f.readings.vals))
		for _, s := range f.readings.vals {
			r, err := parseReading(s)
			if err != nil {
				return nil, err
			}
			rs = append(rs, r)
		}
		v, err := jsonText(rs)
		if err != nil {
			return nil, err
		}
		out.set("readings", v)
	}
	if f.curriculumOrder.set {
		out.set("curriculum_order", f.curriculumOrder.val)
	} else if f.clearCurriculumOrder {
		out.set("curriculum_order", nil)
	}
	return out, nil
}

func patternList(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("pattern list", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets pattern list [--family F] [--json]")
	family := fs.String("family", "", "only patterns in this family")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rows, err := listPatterns(db, *family)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(rows)
	}
	w := newTabWriter()
	fmt.Fprintln(w, "ORDER\tSLUG\tNAME\tFAMILY\tINVARIANTS")
	for _, p := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", curriculumOrderText(p.CurriculumOrder), p.Slug, p.Name, p.Family, truncate(strings.Join(p.Invariants, " "), 60))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d pattern(s)\n", len(rows))
	return nil
}

func patternShow(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("pattern show", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets pattern show <slug> [--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	slug, err := slugArg(fs, "pattern")
	if err != nil {
		return err
	}
	p, err := getPattern(db, slug)
	if err != nil {
		return err
	}
	approaches, err := listApproaches(db, approachFilter{pattern: slug})
	if err != nil {
		return err
	}
	attempts, err := listAttempts(db, attemptFilter{pattern: slug})
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(struct {
			Pattern    Pattern    `json:"pattern"`
			Approaches []Approach `json:"approaches"`
			Attempts   []Attempt  `json:"attempts"`
		}{p, approaches, attempts})
	}

	out := os.Stdout
	fmt.Fprintf(out, "%s  (%s)\n\n", p.Name, p.Slug)
	field(out, "Family", p.Family)
	field(out, "Order", curriculumOrderText(p.CurriculumOrder))
	field(out, "Explanation", p.Explanation)
	field(out, "Use cases", p.UseCases)
	field(out, "Invariants", strings.Join(p.Invariants, "\n"))
	field(out, "Notes", p.Notes)
	readings := make([]string, 0, len(p.Readings))
	for _, r := range p.Readings {
		readings = append(readings, r.String())
	}
	field(out, "Readings", strings.Join(readings, "\n"))

	fmt.Fprintf(out, "\nApproaches (%d)\n", len(approaches))
	w := newTabWriter()
	for _, a := range approaches {
		fmt.Fprintf(w, "  %d\t%s\t%s\t%s\n", a.ID, a.EngineSlug, a.Title, truncate(a.Primitives, 54))
	}
	w.Flush()

	fmt.Fprintf(out, "\nAttempts (%d)\n", len(attempts))
	w = newTabWriter()
	for _, t := range attempts {
		fmt.Fprintf(w, "  %d\t%s\t%s\t%s\n", t.ID, t.EngineSlug, t.Status, t.Title)
	}
	return w.Flush()
}

func patternAdd(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("pattern add", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets pattern add --slug S --name N [--family F] [--curriculum-order N | --clear-curriculum-order] [--explanation E] [--use_cases U] [--invariant I]... [--reading 'Title - URL']... [--notes N]")
	f := newPatternFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := f.validate(); err != nil {
		return err
	}
	if !f.text["slug"].set || !f.text["name"].set {
		return fmt.Errorf("--slug and --name are required")
	}
	fl, err := f.fields()
	if err != nil {
		return err
	}
	id, err := fl.insert(db, "patterns")
	if err != nil {
		return err
	}
	fmt.Printf("added pattern %s (id %d)\n", f.text["slug"].val, id)
	return nil
}

func patternEdit(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("pattern edit", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets pattern edit <slug> [--name N] [--family F] [--curriculum-order N | --clear-curriculum-order] [--invariant I]... [--reading R]... ...")
	f := newPatternFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := f.validate(); err != nil {
		return err
	}
	slug, err := slugArg(fs, "pattern")
	if err != nil {
		return err
	}
	p, err := getPattern(db, slug)
	if err != nil {
		return err
	}
	fl, err := f.fields()
	if err != nil {
		return err
	}
	if err := fl.update(db, "patterns", p.ID); err != nil {
		return err
	}
	fmt.Printf("updated pattern %s\n", slug)
	return nil
}

func patternRM(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("pattern rm", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets pattern rm <slug> [--force]")
	force := fs.Bool("force", false, "delete the pattern's approaches and attempts too")
	if err := fs.Parse(args); err != nil {
		return err
	}
	slug, err := slugArg(fs, "pattern")
	if err != nil {
		return err
	}
	p, err := getPattern(db, slug)
	if err != nil {
		return err
	}
	na, err := countRows(db, "SELECT count(*) FROM approaches WHERE pattern_id = ?", p.ID)
	if err != nil {
		return err
	}
	nt, err := countRows(db, "SELECT count(*) FROM attempts WHERE pattern_id = ?", p.ID)
	if err != nil {
		return err
	}
	if (na > 0 || nt > 0) && !*force {
		return fmt.Errorf("pattern %s has %d approach(es) and %d attempt(s); pass --force to delete them too", slug, na, nt)
	}
	if _, err := db.Exec("DELETE FROM patterns WHERE id = ?", p.ID); err != nil {
		return err
	}
	fmt.Printf("deleted pattern %s (%d approaches, %d attempts)\n", slug, na, nt)
	return nil
}
