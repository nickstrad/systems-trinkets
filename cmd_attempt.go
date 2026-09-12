package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
)

func cmdAttempt(db *sql.DB, args []string) error {
	v, rest, err := verb(args, "attempt", "list|show|add|edit|rm")
	if err != nil {
		return err
	}
	switch v {
	case "list", "ls":
		return attemptList(db, rest)
	case "show":
		return attemptShow(db, rest)
	case "add", "new":
		return attemptAdd(db, rest)
	case "edit", "update":
		return attemptEdit(db, rest)
	case "rm", "delete":
		return attemptRM(db, rest)
	default:
		return fmt.Errorf("unknown attempt verb %q", v)
	}
}

// attemptBody groups the flags describing one run at building a pattern.
type attemptBody struct {
	title    optString
	status   optString
	lessons  optString
	approach optInt
}

func (b *attemptBody) register(fs *flag.FlagSet) {
	fs.Var(&b.title, "title", "what this attempt was")
	fs.Var(&b.status, "status", "planned|in_progress|done|abandoned")
	fs.Var(&b.lessons, "lessons", "what the attempt taught")
	fs.Var(&b.approach, "approach", "id of the approach being tried (must match the pattern and engine)")
}

func (b *attemptBody) fields() *fields {
	f := &fields{}
	for _, kv := range []struct {
		col string
		opt optString
	}{
		{"title", b.title},
		{"status", b.status},
		{"lessons", b.lessons},
	} {
		if kv.opt.set {
			f.set(kv.col, kv.opt.val)
		}
	}
	if b.approach.set {
		f.set("approach_id", b.approach.val)
	}
	return f
}

func attemptList(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("attempt list", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets attempt list [--pattern P] [--engine E] [--status S] [--json]")
	f := attemptFilter{}
	fs.StringVar(&f.pattern, "pattern", "", "only attempts at this pattern slug")
	fs.StringVar(&f.engine, "engine", "", "only attempts on this engine slug")
	fs.StringVar(&f.status, "status", "", "only attempts with this status")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rows, err := listAttempts(db, f)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(rows)
	}
	w := newTabWriter()
	fmt.Fprintln(w, "ID\tPATTERN\tENGINE\tSTATUS\tTITLE")
	for _, t := range rows {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", t.ID, t.PatternSlug, t.EngineSlug, t.Status, t.Title)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d attempt(s)\n", len(rows))
	return nil
}

func attemptShow(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("attempt show", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets attempt show <id> [--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := idArg(fs, "attempt")
	if err != nil {
		return err
	}
	t, err := getAttempt(db, id)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(t)
	}
	out := os.Stdout
	fmt.Fprintf(out, "#%d  %s\n%s on %s\n\n", t.ID, t.Title, t.PatternSlug, t.EngineSlug)
	field(out, "Status", t.Status)
	if t.ApproachID != nil {
		field(out, "Approach", fmt.Sprintf("%d - %s", *t.ApproachID, t.ApproachTitle))
	}
	field(out, "Lessons", t.Lessons)
	field(out, "Updated", t.UpdatedAt)
	return nil
}

func attemptAdd(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("attempt add", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets attempt add --pattern P --engine E --title T [--approach ID] [--status S] [--lessons ...]")
	pattern := fs.String("pattern", "", "pattern slug (required)")
	engine := fs.String("engine", "", "engine slug (required)")
	body := &attemptBody{}
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
	id, err := f.insert(db, "attempts")
	if err != nil {
		return annotateApproachFK(err, body.approach.set)
	}
	fmt.Printf("added attempt %d: %s on %s - %s\n", id, p.Slug, e.Slug, body.title.val)
	return nil
}

func attemptEdit(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("attempt edit", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets attempt edit <id> [--status S] [--lessons ...] [--title T] [--approach ID]")
	body := &attemptBody{}
	body.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := idArg(fs, "attempt")
	if err != nil {
		return err
	}
	if _, err := getAttempt(db, id); err != nil {
		return err
	}
	if err := body.fields().update(db, "attempts", id); err != nil {
		return annotateApproachFK(err, body.approach.set)
	}
	fmt.Printf("updated attempt %d\n", id)
	return nil
}

func attemptRM(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("attempt rm", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets attempt rm <id>")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := idArg(fs, "attempt")
	if err != nil {
		return err
	}
	if _, err := getAttempt(db, id); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM attempts WHERE id = ?", id); err != nil {
		return err
	}
	fmt.Printf("deleted attempt %d\n", id)
	return nil
}

// annotateApproachFK explains the composite foreign key, which is the one
// constraint here whose message ("FOREIGN KEY constraint failed") says nothing
// useful on its own.
func annotateApproachFK(err error, usedApproach bool) error {
	if err == nil || !usedApproach {
		return err
	}
	return fmt.Errorf("%w (the approach must exist and belong to the same pattern and engine as this attempt)", err)
}
