package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"strings"
)

const seedApproachTitle = "core map sketch"

type seedOptions struct{ update, prune, dryRun bool }

func parseSeedOptions(args []string) (seedOptions, error) {
	var o seedOptions
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets seed [--update [--prune [--dry-run]]]")
	fs.BoolVar(&o.update, "update", false, "refresh curriculum-owned fields, preserving authored prose")
	fs.BoolVar(&o.prune, "prune", false, "explicitly reconcile legacy identities to the curriculum (requires --update)")
	fs.BoolVar(&o.dryRun, "dry-run", false, "preview reconciliation without modifying the source (requires --update --prune)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() != 0 {
		return o, fmt.Errorf("seed takes no positional arguments")
	}
	if o.prune && !o.update {
		return o, fmt.Errorf("--prune requires --update")
	}
	if o.dryRun && !(o.update && o.prune) {
		return o, fmt.Errorf("--dry-run requires --update --prune")
	}
	return o, nil
}

func cmdSeed(db *sql.DB, args []string) error {
	o, err := parseSeedOptions(args)
	if err != nil {
		return err
	}
	if o.prune {
		report, err := reconcileCatalog(db, o.dryRun, nil)
		if report != nil {
			err = errors.Join(err, printReconcileReport(report))
		}
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	counts, err := seedCatalog(tx, o.update)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("changed %d engine(s), %d pattern(s), %d approach(es)\n", counts[0], counts[1], counts[2])
	return nil
}

// seedCatalog uses the caller's transaction; it never replaces authored fields.
// Conditional upserts leave timestamps intact when curriculum content is equal.
func seedCatalog(tx *sql.Tx, update bool) ([3]int, error) {
	var counts [3]int
	ts := now()
	engineSQL := `INSERT INTO engines (slug,name,notes,created_at,updated_at) VALUES (?,?,?,?,?) ON CONFLICT(slug) DO ` + seedConflict(update, "engines", []string{"name", "notes"})
	patternSQL := `INSERT INTO patterns (slug,name,family,explanation,use_cases,invariants,readings,curriculum_order,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(slug) DO ` + seedConflict(update, "patterns", []string{"name", "family", "explanation", "use_cases", "invariants", "readings", "curriculum_order"})
	approachSQL := `INSERT INTO approaches (pattern_id,engine_id,title,primitives,created_at,updated_at) SELECT p.id,e.id,?,?,?,? FROM patterns p,engines e WHERE p.slug=? AND e.slug=? ON CONFLICT(pattern_id,engine_id,title) DO ` + seedConflict(update, "approaches", []string{"primitives"})
	// Clear changed managed positions before assigning any target positions.
	if update {
		// Only clear positions when they differ from target: unchanged runs do no writes.
		for _, p := range seedPatterns {
			if _, err := tx.Exec(`UPDATE patterns SET curriculum_order=NULL WHERE slug=? AND curriculum_order IS NOT ?`, p.slug, p.curriculumOrder); err != nil {
				return counts, err
			}
		}
	}
	for _, e := range seedEngines {
		r, err := tx.Exec(engineSQL, e.slug, e.name, e.notes, ts, ts)
		if err != nil {
			return counts, fmt.Errorf("seed engine %s: %w", e.slug, err)
		}
		counts[0] += affected(r)
	}
	for _, p := range seedPatterns {
		inv, err := jsonText(p.invariants)
		if err != nil {
			return counts, err
		}
		reading, err := jsonText(p.readings)
		if err != nil {
			return counts, err
		}
		r, err := tx.Exec(patternSQL, p.slug, p.name, p.family, p.explanation, p.useCases, inv, reading, p.curriculumOrder, ts, ts)
		if err != nil {
			return counts, fmt.Errorf("seed pattern %s: %w", p.slug, err)
		}
		counts[1] += affected(r)
		for _, e := range []struct{ slug, primitives string }{{"valkey", p.valkey}, {"sqlite", p.sqlite}, {"postgres", p.postgres}} {
			r, err := tx.Exec(approachSQL, seedApproachTitle, e.primitives, ts, ts, p.slug, e.slug)
			if err != nil {
				return counts, fmt.Errorf("seed approach %s/%s: %w", p.slug, e.slug, err)
			}
			counts[2] += affected(r)
		}
	}
	return counts, nil
}

func seedConflict(update bool, table string, columns []string) string {
	if !update {
		return "NOTHING"
	}
	var sets, changes []string
	for _, c := range columns {
		sets = append(sets, c+"=excluded."+c)
		changes = append(changes, table+"."+c+" IS NOT excluded."+c)
	}
	return "UPDATE SET " + strings.Join(sets, ",") + ",updated_at=excluded.updated_at WHERE " + strings.Join(changes, " OR ")
}
func affected(res sql.Result) int {
	n, err := res.RowsAffected()
	if err != nil {
		return 0
	}
	return int(n)
}
