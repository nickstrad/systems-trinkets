package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"modernc.org/sqlite"
)

var curriculumRenames = [][2]string{
	{"dedup", "inbox"}, {"expiring-uniqueness", "expiring-reservation"},
	{"retry-queue", "retry-lifecycle"}, {"fixed-window-rate-limiter", "rate-limiter"},
	{"pubsub", "notification-vs-delivery"}, {"materialized-view", "incremental-projection"},
	{"ttl-cache", "cache-consistency"},
}
var curriculumRetirements = []string{"session-store", "distributed-coordination", "lock", "secondary-index", "time-ordered-data", "leaderboard", "dead-letter-queue", "sliding-window-rate-limiter", "token-bucket"}

type catalogAction struct {
	Action             string  `json:"action"`
	ID                 int64   `json:"id"`
	OldSlug            string  `json:"old_slug,omitempty"`
	Slug               string  `json:"slug"`
	RemovedApproachIDs []int64 `json:"removed_approach_ids,omitempty"`
}
type reconcileReport struct {
	Actions                     []catalogAction `json:"actions"`
	Conflicts                   []string        `json:"conflicts"`
	PreservedAuthoredApproaches []int64         `json:"preserved_authored_approaches"`
	DryRun                      bool            `json:"dry_run"`
}

func printReconcileReport(r *reconcileReport) error { return printJSON(r) }

// planCatalog reads only through the transaction, including all preservation
// checks. Apply rebuilds this report while holding the SQLite writer lock.
func planCatalog(tx *sql.Tx) (*reconcileReport, error) {
	r := &reconcileReport{Actions: []catalogAction{}, Conflicts: []string{}, PreservedAuthoredApproaches: []int64{}}
	type existing struct {
		id    int64
		notes string
	}
	existingRows := map[string]existing{}
	rows, err := tx.Query(`SELECT slug,id,notes FROM patterns ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var slug string
		var p existing
		if err := rows.Scan(&slug, &p.id, &p.notes); err != nil {
			rows.Close()
			return nil, err
		}
		existingRows[slug] = p
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	allowed := map[string]bool{}
	renames := map[string]string{}
	affected := map[string]bool{}
	for _, p := range seedPatterns {
		allowed[p.slug] = true
	}
	for _, pair := range curriculumRenames {
		allowed[pair[0]] = true
		renames[pair[1]] = pair[0]
		affected[pair[0]] = true
	}
	for _, slug := range curriculumRetirements {
		allowed[slug] = true
		affected[slug] = true
	}
	// Sort every report collection for repeatable preview output.
	slugs := make([]string, 0, len(existingRows))
	for slug := range existingRows {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		p := existingRows[slug]
		if !allowed[slug] {
			r.Conflicts = append(r.Conflicts, fmt.Sprintf("unknown pattern %s (id %d)", slug, p.id))
		}
		if affected[slug] && p.notes != "" {
			r.Conflicts = append(r.Conflicts, fmt.Sprintf("pattern %s (id %d) has authored notes", slug, p.id))
		}
	}
	for _, p := range seedPatterns {
		current, exists := existingRows[p.slug]
		old := renames[p.slug]
		source, hasSource := existingRows[old]
		if hasSource && exists {
			r.Conflicts = append(r.Conflicts, fmt.Sprintf("rename collision: %s (id %d) -> %s (id %d)", old, source.id, p.slug, current.id))
		}
		a := catalogAction{Action: "insert", Slug: p.slug}
		if exists {
			a.Action = "refresh"
			a.ID = current.id
		} else if hasSource {
			a.Action = "rename+refresh"
			a.ID = source.id
			a.OldSlug = old
		}
		r.Actions = append(r.Actions, a)
	}
	for _, slug := range curriculumRetirements {
		if p, ok := existingRows[slug]; ok {
			r.Actions = append(r.Actions, catalogAction{Action: "remove", ID: p.id, OldSlug: slug, Slug: slug})
		}
	}
	rows, err = tx.Query(`SELECT slug,id FROM engines ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var slug string
		var id int64
		if err := rows.Scan(&slug, &id); err != nil {
			rows.Close()
			return nil, err
		}
		if slug != "valkey" && slug != "sqlite" && slug != "postgres" {
			r.Conflicts = append(r.Conflicts, fmt.Sprintf("unknown engine %s (id %d)", slug, id))
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = tx.Query(`SELECT a.id,p.slug,a.title,a.writeup FROM approaches a JOIN patterns p ON p.id=a.pattern_id ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var slug, title, writeup string
		if err := rows.Scan(&id, &slug, &title, &writeup); err != nil {
			rows.Close()
			return nil, err
		}
		if affected[slug] && (title != seedApproachTitle || writeup != "") {
			r.Conflicts = append(r.Conflicts, fmt.Sprintf("approach %d on %s has authored content or non-core title %q", id, slug, title))
		}
		if !affected[slug] && allowed[slug] && title != seedApproachTitle {
			r.PreservedAuthoredApproaches = append(r.PreservedAuthoredApproaches, id)
		}
		for i := range r.Actions {
			if r.Actions[i].Action == "remove" && r.Actions[i].Slug == slug {
				r.Actions[i].RemovedApproachIDs = append(r.Actions[i].RemovedApproachIDs, id)
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = tx.Query(`SELECT a.id,p.slug FROM attempts a JOIN patterns p ON p.id=a.pattern_id ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var slug string
		if err := rows.Scan(&id, &slug); err != nil {
			rows.Close()
			return nil, err
		}
		if affected[slug] {
			r.Conflicts = append(r.Conflicts, fmt.Sprintf("attempt %d references affected pattern %s", id, slug))
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	sort.Strings(r.Conflicts)
	return r, nil
}

// hook is a failure-injection seam for transaction rollback tests.
func reconcileCatalog(db *sql.DB, dryRun bool, hook func(string) error) (*reconcileReport, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if !dryRun {
		if _, err := tx.Exec(`UPDATE patterns SET id=id WHERE 0`); err != nil {
			return nil, err
		}
	}
	report, err := planCatalog(tx)
	if err != nil {
		return nil, err
	}
	report.DryRun = dryRun
	if len(report.Conflicts) > 0 {
		return report, fmt.Errorf("curriculum reconciliation blocked by %d conflict(s)", len(report.Conflicts))
	}
	if dryRun {
		return report, nil
	}
	for _, a := range report.Actions {
		if a.Action == "rename+refresh" {
			if _, err := tx.Exec(`UPDATE patterns SET slug=?,updated_at=? WHERE id=?`, a.Slug, now(), a.ID); err != nil {
				return report, err
			}
		}
	}
	if hook != nil {
		if err := hook("rename"); err != nil {
			return report, err
		}
	}
	// Retirements are still present until after target upserts; release their order.
	for _, slug := range curriculumRetirements {
		if _, err := tx.Exec(`UPDATE patterns SET curriculum_order=NULL WHERE slug=? AND curriculum_order IS NOT NULL`, slug); err != nil {
			return report, err
		}
	}
	if _, err := seedCatalog(tx, true); err != nil {
		return report, err
	}
	if hook != nil {
		if err := hook("upsert"); err != nil {
			return report, err
		}
	}
	for _, slug := range curriculumRetirements {
		if _, err := tx.Exec(`DELETE FROM patterns WHERE slug=?`, slug); err != nil {
			return report, err
		}
	}
	if hook != nil {
		if err := hook("delete"); err != nil {
			return report, err
		}
	}
	if err := verifyCatalog(tx); err != nil {
		return report, err
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

func verifyCatalog(tx *sql.Tx) error {
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM patterns`).Scan(&count); err != nil {
		return err
	}
	if count != len(seedPatterns) {
		return fmt.Errorf("expected %d patterns, got %d", len(seedPatterns), count)
	}
	if err := tx.QueryRow(`SELECT count(*) FROM engines`).Scan(&count); err != nil {
		return err
	}
	if count != 3 {
		return fmt.Errorf("expected three engines, got %d", count)
	}
	for _, p := range seedPatterns {
		inv, err := jsonText(p.invariants)
		if err != nil {
			return err
		}
		readings, err := jsonText(p.readings)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT count(*) FROM patterns WHERE slug=? AND name=? AND family=? AND explanation=? AND use_cases=? AND invariants=? AND readings=? AND curriculum_order=?`, p.slug, p.name, p.family, p.explanation, p.useCases, inv, readings, p.curriculumOrder).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("canonical content mismatch for %s", p.slug)
		}
		for _, e := range []struct{ slug, primitives string }{{"valkey", p.valkey}, {"sqlite", p.sqlite}, {"postgres", p.postgres}} {
			if err := tx.QueryRow(`SELECT count(*) FROM approaches a JOIN patterns p ON p.id=a.pattern_id JOIN engines e ON e.id=a.engine_id WHERE p.slug=? AND e.slug=? AND a.title=? AND a.primitives=?`, p.slug, e.slug, seedApproachTitle, e.primitives).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("canonical approach mismatch for %s/%s", p.slug, e.slug)
			}
		}
	}

	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("foreign key check failed")
	}
	return rows.Err()
}

// backupSQLite opens the source read-only, without running application migrations
// or changing journal mode. SQLite's backup API includes committed WAL pages.
func backupSQLite(source, destination string) error {
	absolute, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Set("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(raw any) error {
		b, ok := raw.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("SQLite driver does not support backup")
		}
		backup, err := b.NewBackup(destination)
		if err != nil {
			return err
		}
		for {
			more, stepErr := backup.Step(-1)
			if stepErr != nil || !more {
				return errors.Join(stepErr, backup.Finish())
			}
		}
	})
}

func dryRunSeed(source string) error {
	dir, err := os.MkdirTemp("", "trinkets-preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	target := filepath.Join(dir, "preview.db")
	if err := backupSQLite(source, target); err != nil {
		return fmt.Errorf("preview backup: %w", err)
	}
	db, err := openDB(target)
	if err != nil {
		return err
	}
	defer db.Close()
	report, err := reconcileCatalog(db, true, nil)
	if report != nil {
		err = errors.Join(err, printReconcileReport(report))
	}
	return err
}
