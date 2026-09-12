package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
)

func cmdApproach(db *sql.DB, args []string) error {
	v, rest, err := verb(args, "approach", "list|show|add|edit|rm")
	if err != nil {
		return err
	}
	switch v {
	case "list", "ls":
		return approachList(db, rest)
	case "show":
		return approachShow(db, rest)
	case "add", "new":
		return approachAdd(db, rest)
	case "edit", "update":
		return approachEdit(db, rest)
	case "rm", "delete":
		return approachRM(db, rest)
	default:
		return fmt.Errorf("unknown approach verb %q", v)
	}
}

// approachBody groups the flags that describe an approach.
type approachBody struct {
	title      optString
	primitives optString
	writeup    optString
}

func (b *approachBody) register(fs *flag.FlagSet) {
	fs.Var(&b.title, "title", "short name for this approach, unique per pattern+engine")
	fs.Var(&b.primitives, "primitives", "the primitives doing the work, e.g. \"FOR UPDATE SKIP LOCKED + transaction\"")
	fs.Var(&b.writeup, "writeup", "how to build it")
}

func (b *approachBody) fields() *fields {
	f := &fields{}
	for _, kv := range []struct {
		col string
		opt optString
	}{
		{"title", b.title},
		{"primitives", b.primitives},
		{"writeup", b.writeup},
	} {
		if kv.opt.set {
			f.set(kv.col, kv.opt.val)
		}
	}
	return f
}

func approachList(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("approach list", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets approach list [--pattern P] [--engine E] [--json]")
	f := approachFilter{}
	fs.StringVar(&f.pattern, "pattern", "", "only approaches for this pattern slug")
	fs.StringVar(&f.engine, "engine", "", "only approaches for this engine slug")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rows, err := listApproaches(db, f)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(rows)
	}
	w := newTabWriter()
	fmt.Fprintln(w, "ID\tPATTERN\tENGINE\tTITLE\tPRIMITIVES")
	for _, a := range rows {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", a.ID, a.PatternSlug, a.EngineSlug, a.Title, truncate(a.Primitives, 52))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d approach(es)\n", len(rows))
	return nil
}

func approachShow(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("approach show", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets approach show <id> [--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := idArg(fs, "approach")
	if err != nil {
		return err
	}
	a, err := getApproach(db, id)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(a)
	}
	out := os.Stdout
	fmt.Fprintf(out, "#%d  %s\n%s on %s\n\n", a.ID, a.Title, a.PatternSlug, a.EngineSlug)
	field(out, "Primitives", a.Primitives)
	field(out, "Writeup", a.Writeup)
	field(out, "Updated", a.UpdatedAt)
	return nil
}

func approachAdd(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("approach add", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets approach add --pattern P --engine E --title T [--primitives ...] [--writeup ...]")
	pattern := fs.String("pattern", "", "pattern slug (required)")
	engine := fs.String("engine", "", "engine slug (required)")
	body := &approachBody{}
	body.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pattern == "" || *engine == "" || !body.title.set {
		return fmt.Errorf("--pattern, --engine and --title are required")
	}
	p, err := getPattern(db, *pattern)
	if err != nil {
		return err
	}
	e, err := getEngine(db, *engine)
	if err != nil {
		return err
	}
	f := body.fields()
	f.set("pattern_id", p.ID)
	f.set("engine_id", e.ID)
	id, err := f.insert(db, "approaches")
	if err != nil {
		return err
	}
	fmt.Printf("added approach %d: %s on %s - %s\n", id, p.Slug, e.Slug, body.title.val)
	return nil
}

func approachEdit(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("approach edit", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets approach edit <id> [--title T] [--primitives ...] [--writeup ...]")
	body := &approachBody{}
	body.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := idArg(fs, "approach")
	if err != nil {
		return err
	}
	if _, err := getApproach(db, id); err != nil {
		return err
	}
	if err := body.fields().update(db, "approaches", id); err != nil {
		return err
	}
	fmt.Printf("updated approach %d\n", id)
	return nil
}

func approachRM(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("approach rm", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets approach rm <id>")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := idArg(fs, "approach")
	if err != nil {
		return err
	}
	a, err := getApproach(db, id)
	if err != nil {
		return err
	}
	// Attempts outlive the approach they cite: unlink them first, then delete.
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE attempts SET approach_id = NULL, updated_at = ? WHERE approach_id = ?", now(), id)
	if err != nil {
		return err
	}
	unlinked, _ := res.RowsAffected()
	if _, err := tx.Exec("DELETE FROM approaches WHERE id = ?", id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("deleted approach %d (%s on %s); unlinked %d attempt(s)\n", id, a.PatternSlug, a.EngineSlug, unlinked)
	return nil
}
