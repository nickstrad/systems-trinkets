package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
)

func cmdEngine(db *sql.DB, args []string) error {
	v, rest, err := verb(args, "engine", "list|show|add|edit|rm")
	if err != nil {
		return err
	}
	switch v {
	case "list", "ls":
		return engineList(db, rest)
	case "show":
		return engineShow(db, rest)
	case "add", "new":
		return engineAdd(db, rest)
	case "edit", "update":
		return engineEdit(db, rest)
	case "rm", "delete":
		return engineRM(db, rest)
	default:
		return fmt.Errorf("unknown engine verb %q", v)
	}
}

func engineFlags(fs *flag.FlagSet) map[string]*optString {
	f := map[string]*optString{}
	for name, help := range map[string]string{
		"slug":  "short stable identifier, e.g. postgres",
		"name":  "human name, e.g. PostgreSQL",
		"notes": "which primitives this engine gives you, version quirks, ...",
	} {
		o := &optString{}
		fs.Var(o, name, help)
		f[name] = o
	}
	return f
}

func engineFields(f map[string]*optString) *fields {
	out := &fields{}
	for _, col := range []string{"slug", "name", "notes"} {
		if f[col].set {
			out.set(col, f[col].val)
		}
	}
	return out
}

func engineList(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("engine list", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets engine list [--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rows, err := listEngines(db)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(rows)
	}
	w := newTabWriter()
	fmt.Fprintln(w, "SLUG\tNAME\tNOTES")
	for _, e := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\n", e.Slug, e.Name, truncate(e.Notes, 70))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d engine(s)\n", len(rows))
	return nil
}

func engineShow(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("engine show", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets engine show <slug> [--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	slug, err := slugArg(fs, "engine")
	if err != nil {
		return err
	}
	e, err := getEngine(db, slug)
	if err != nil {
		return err
	}
	approaches, err := listApproaches(db, approachFilter{engine: slug})
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(struct {
			Engine     Engine     `json:"engine"`
			Approaches []Approach `json:"approaches"`
		}{e, approaches})
	}
	out := os.Stdout
	fmt.Fprintf(out, "%s  (%s)\n\n", e.Name, e.Slug)
	field(out, "Notes", e.Notes)
	fmt.Fprintf(out, "\nApproaches (%d)\n", len(approaches))
	w := newTabWriter()
	for _, a := range approaches {
		fmt.Fprintf(w, "  %d\t%s\t%s\t%s\n", a.ID, a.PatternSlug, a.Title, truncate(a.Primitives, 50))
	}
	return w.Flush()
}

func engineAdd(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("engine add", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets engine add --slug S --name N [--notes N]")
	f := engineFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !f["slug"].set || !f["name"].set {
		return fmt.Errorf("--slug and --name are required")
	}
	id, err := engineFields(f).insert(db, "engines")
	if err != nil {
		return err
	}
	fmt.Printf("added engine %s (id %d)\n", f["slug"].val, id)
	return nil
}

func engineEdit(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("engine edit", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets engine edit <slug> [--name N] [--notes N]")
	f := engineFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	slug, err := slugArg(fs, "engine")
	if err != nil {
		return err
	}
	e, err := getEngine(db, slug)
	if err != nil {
		return err
	}
	if err := engineFields(f).update(db, "engines", e.ID); err != nil {
		return err
	}
	fmt.Printf("updated engine %s\n", slug)
	return nil
}

func engineRM(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("engine rm", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets engine rm <slug> [--force]")
	force := fs.Bool("force", false, "delete the engine's approaches and attempts too")
	if err := fs.Parse(args); err != nil {
		return err
	}
	slug, err := slugArg(fs, "engine")
	if err != nil {
		return err
	}
	e, err := getEngine(db, slug)
	if err != nil {
		return err
	}
	na, err := countRows(db, "SELECT count(*) FROM approaches WHERE engine_id = ?", e.ID)
	if err != nil {
		return err
	}
	nt, err := countRows(db, "SELECT count(*) FROM attempts WHERE engine_id = ?", e.ID)
	if err != nil {
		return err
	}
	if (na > 0 || nt > 0) && !*force {
		return fmt.Errorf("engine %s has %d approach(es) and %d attempt(s); pass --force to delete them too", slug, na, nt)
	}
	if _, err := db.Exec("DELETE FROM engines WHERE id = ?", e.ID); err != nil {
		return err
	}
	fmt.Printf("deleted engine %s (%d approaches, %d attempts)\n", slug, na, nt)
	return nil
}
