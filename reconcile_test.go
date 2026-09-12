package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func legacyCatalog(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/legacy-catalog.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(fixture)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return path, db
}

func catalogSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	snapshot := map[string][]string{}
	for _, table := range []string{"patterns", "engines", "approaches", "attempts"} {
		rows, err := db.Query(`SELECT * FROM ` + table + ` ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(vals)
			if err != nil {
				t.Fatal(err)
			}
			snapshot[table] = append(snapshot[table], string(b))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	b, _ := json.Marshal(snapshot)
	return string(b)
}

func execTestSQL(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}
func checkCatalog(t *testing.T, db *sql.DB) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := verifyCatalog(tx); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := tx.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil || result != "ok" {
		t.Fatalf("integrity %s: %v", result, err)
	}
}

func TestReconcileBaselineIdentityAndIdempotence(t *testing.T) {
	_, db := legacyCatalog(t)
	type identity struct {
		ID      int64
		Created string
	}
	before := map[string]identity{}
	approaches := map[int64]string{}
	rows, err := db.Query(`SELECT slug,id,created_at FROM patterns`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var slug string
		var id identity
		if err := rows.Scan(&slug, &id.ID, &id.Created); err != nil {
			t.Fatal(err)
		}
		before[slug] = id
	}
	rows.Close()
	rows, err = db.Query(`SELECT id,created_at FROM approaches`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		var created string
		if err := rows.Scan(&id, &created); err != nil {
			t.Fatal(err)
		}
		approaches[id] = created
	}
	rows.Close()
	preview, err := reconcileCatalog(db, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := reconcileCatalog(db, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(preview.Actions, report.Actions) {
		t.Fatal("preview and apply actions differ")
	}
	checkCatalog(t, db)
	retained := 0
	for _, p := range seedPatterns {
		old := p.slug
		for _, pair := range curriculumRenames {
			if pair[1] == p.slug {
				old = pair[0]
			}
		}
		if want, ok := before[old]; ok {
			var got identity
			if err := db.QueryRow(`SELECT id,created_at FROM patterns WHERE slug=?`, p.slug).Scan(&got.ID, &got.Created); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("%s identity changed: %+v != %+v", p.slug, got, want)
			}
			retained++
		}
	}
	if retained != 19 {
		t.Fatalf("retained %d", retained)
	}
	rows, err = db.Query(`SELECT id,created_at FROM approaches`)
	if err != nil {
		t.Fatal(err)
	}
	preserved := 0
	total := 0
	for rows.Next() {
		var id int64
		var created string
		if err := rows.Scan(&id, &created); err != nil {
			t.Fatal(err)
		}
		total++
		if want, ok := approaches[id]; ok {
			if created != want {
				t.Fatalf("approach %d timestamp changed", id)
			}
			preserved++
		}
	}
	rows.Close()
	if total != 87 || preserved != 57 {
		t.Fatalf("approaches total %d preserved %d", total, preserved)
	}
	first := catalogSnapshot(t, db)
	execTestSQL(t, db, `CREATE TEMP TRIGGER forbid_pattern_update BEFORE UPDATE ON patterns BEGIN SELECT RAISE(ABORT,'unexpected second-run update'); END`)
	execTestSQL(t, db, `CREATE TEMP TRIGGER forbid_engine_update BEFORE UPDATE ON engines BEGIN SELECT RAISE(ABORT,'unexpected second-run update'); END`)
	execTestSQL(t, db, `CREATE TEMP TRIGGER forbid_approach_update BEFORE UPDATE ON approaches BEGIN SELECT RAISE(ABORT,'unexpected second-run update'); END`)
	if _, err := reconcileCatalog(db, false, nil); err != nil {
		t.Fatal(err)
	}
	if after := catalogSnapshot(t, db); after != first {
		t.Fatal("second reconcile changed semantic rows")
	}
	for _, args := range [][]string{nil, {"--update"}} {
		if err := cmdSeed(db, args); err != nil {
			t.Fatal(err)
		}
		if catalogSnapshot(t, db) != first {
			t.Fatal("seed changed reconciled catalog")
		}
	}
}

func TestReconcileRollbackStages(t *testing.T) {
	for _, stage := range []string{"rename", "upsert", "delete"} {
		t.Run(stage, func(t *testing.T) {
			_, db := legacyCatalog(t)
			before := catalogSnapshot(t, db)
			_, err := reconcileCatalog(db, false, func(s string) error {
				if s == stage {
					return errors.New("injected " + s)
				}
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("wrong error %v", err)
			}
			if catalogSnapshot(t, db) != before {
				t.Fatal("failed reconciliation changed data")
			}
		})
	}
}

func TestReconcileConflicts(t *testing.T) {
	cases := []struct{ name, sql, match string }{
		{"collision", `INSERT INTO patterns(slug,name,created_at,updated_at) VALUES('inbox','custom','then','then')`, "rename collision"},
		{"unknown pattern", `INSERT INTO patterns(slug,name,curriculum_order,created_at,updated_at) VALUES('custom','Custom',1,'then','then')`, "unknown pattern custom"},
		{"unknown engine", `INSERT INTO engines(slug,name,created_at,updated_at) VALUES('custom','Custom','then','then')`, "unknown engine custom"},
		{"rename notes", `UPDATE patterns SET notes='keep me' WHERE slug='dedup'`, "authored notes"},
		{"remove notes", `UPDATE patterns SET notes='keep me' WHERE slug='lock'`, "authored notes"},
		{"rename writeup", `UPDATE approaches SET writeup='keep me' WHERE pattern_id=(SELECT id FROM patterns WHERE slug='dedup')`, "authored content"},
		{"remove writeup", `UPDATE approaches SET writeup='keep me' WHERE pattern_id=(SELECT id FROM patterns WHERE slug='lock')`, "authored content"},
		{"non-core", `INSERT INTO approaches(pattern_id,engine_id,title,created_at,updated_at) SELECT p.id,e.id,'custom','then','then' FROM patterns p,engines e WHERE p.slug='lock' AND e.slug='sqlite'`, "non-core title"},
		{"renamed attempt", `INSERT INTO attempts(pattern_id,engine_id,approach_id,title,created_at,updated_at) SELECT a.pattern_id,a.engine_id,a.id,'history','then','then' FROM approaches a JOIN patterns p ON p.id=a.pattern_id WHERE p.slug='dedup' LIMIT 1`, "attempt"},
		{"retired attempt", `INSERT INTO attempts(pattern_id,engine_id,approach_id,title,created_at,updated_at) SELECT a.pattern_id,a.engine_id,a.id,'history','then','then' FROM approaches a JOIN patterns p ON p.id=a.pattern_id WHERE p.slug='lock' LIMIT 1`, "attempt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, db := legacyCatalog(t)
			execTestSQL(t, db, tc.sql)
			before := catalogSnapshot(t, db)
			for _, dry := range []bool{true, false} {
				report, err := reconcileCatalog(db, dry, nil)
				if err == nil {
					t.Fatal("conflict accepted")
				}
				if report == nil || !strings.Contains(strings.Join(report.Conflicts, "\n"), tc.match) {
					t.Fatalf("missing precise conflict: %+v / %v", report, err)
				}
				if before != catalogSnapshot(t, db) {
					t.Fatal("conflict changed data")
				}
			}
		})
	}
}

func TestReconcileRetainedAuthoredHistory(t *testing.T) {
	_, db := legacyCatalog(t)
	execTestSQL(t, db, `UPDATE patterns SET notes='my notes' WHERE slug='counter'`)
	execTestSQL(t, db, `UPDATE approaches SET writeup='my implementation' WHERE pattern_id=(SELECT id FROM patterns WHERE slug='counter')`)
	execTestSQL(t, db, `INSERT INTO approaches(pattern_id,engine_id,title,primitives,writeup,created_at,updated_at) SELECT p.id,e.id,'authored','my primitives','my prose','original','original' FROM patterns p,engines e WHERE p.slug='counter' AND e.slug='sqlite'`)
	execTestSQL(t, db, `INSERT INTO attempts(pattern_id,engine_id,approach_id,title,lessons,created_at,updated_at) SELECT a.pattern_id,a.engine_id,a.id,'history','lessons','original','original' FROM approaches a JOIN patterns p ON p.id=a.pattern_id WHERE p.slug='counter'`)
	var history string
	snapshotHistory := func() string {
		rows, err := db.Query(`SELECT id,pattern_id,engine_id,approach_id,title,lessons,created_at,updated_at FROM attempts ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var result []string
		for rows.Next() {
			var id, p, e, a int64
			var title, lessons, c, u string
			if err := rows.Scan(&id, &p, &e, &a, &title, &lessons, &c, &u); err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal([]any{id, p, e, a, title, lessons, c, u})
			result = append(result, string(b))
		}
		return strings.Join(result, "\n")
	}
	history = snapshotHistory()
	r, err := reconcileCatalog(db, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.PreservedAuthoredApproaches) != 1 {
		t.Fatalf("authored report %+v", r)
	}
	if snapshotHistory() != history {
		t.Fatal("history changed")
	}
	var notes string
	if err := db.QueryRow(`SELECT notes FROM patterns WHERE slug='counter'`).Scan(&notes); err != nil || notes != "my notes" {
		t.Fatalf("notes %q %v", notes, err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM approaches WHERE writeup='my implementation'`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("writeups %d %v", count, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM approaches WHERE title='authored' AND primitives='my primitives' AND writeup='my prose' AND created_at='original' AND updated_at='original'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("authored approach lost %d %v", count, err)
	}
	checkCatalog(t, db)
}

func TestDryRunDoesNotMigrateSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/legacy-catalog.sql")
	if err != nil {
		t.Fatal(err)
	}
	execTestSQL(t, db, string(fixture))
	db.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--db", path, "seed", "--update", "--prune", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("dry-run modified source bytes")
	}
	db, err = sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('patterns') WHERE name='curriculum_order'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("dry-run migrated source %d %v", count, err)
	}
}

func TestSeedOptionValidationBeforeOpen(t *testing.T) {
	for _, args := range [][]string{{"--prune"}, {"--dry-run"}, {"--update", "--dry-run"}, {"unexpected"}, {"--prune=false", "--dry-run"}} {
		path := filepath.Join(t.TempDir(), "must-not-exist.db")
		if err := run(append([]string{"--db", path, "seed"}, args...)); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid mode created database: %v", err)
		}
	}
}

func TestBackupIncludesCommittedWAL(t *testing.T) {
	source := filepath.Join(t.TempDir(), "wal.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	execTestSQL(t, db, `PRAGMA journal_mode=WAL`)
	execTestSQL(t, db, `PRAGMA wal_autocheckpoint=0`)
	execTestSQL(t, db, `CREATE TABLE committed(value TEXT)`)
	execTestSQL(t, db, `INSERT INTO committed VALUES('in the WAL')`)
	if stat, err := os.Stat(source + "-wal"); err != nil || stat.Size() == 0 {
		t.Fatalf("no live WAL: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "backup.db")
	if err := backupSQLite(source, destination); err != nil {
		t.Fatal(err)
	}
	copy, err := sql.Open("sqlite", destination)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	var value string
	if err := copy.QueryRow(`SELECT value FROM committed`).Scan(&value); err != nil || value != "in the WAL" {
		t.Fatalf("backup omitted WAL: %q %v", value, err)
	}
}

func TestOrdinarySeedKeepsLegacyAndCustomContent(t *testing.T) {
	_, db := legacyCatalog(t)
	execTestSQL(t, db, `INSERT INTO patterns(slug,name,notes,created_at,updated_at) VALUES('custom','Custom','mine','then','then')`)
	for _, args := range [][]string{nil, {"--update"}} {
		if err := cmdSeed(db, args); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM patterns WHERE slug IN ('lock','dedup','custom')`).Scan(&count); err != nil || count != 3 {
			t.Fatalf("ordinary seed removed/renamed rows: %d %v", count, err)
		}
	}
}

func TestSeedOrderSwapAndOccupiedCustomOrder(t *testing.T) {
	_, db := legacyCatalog(t)
	if _, err := reconcileCatalog(db, false, nil); err != nil {
		t.Fatal(err)
	}
	execTestSQL(t, db, `UPDATE patterns SET curriculum_order=NULL WHERE slug='counter'`)
	execTestSQL(t, db, `UPDATE patterns SET curriculum_order=1 WHERE slug='optimistic-concurrency'`)
	execTestSQL(t, db, `UPDATE patterns SET curriculum_order=2 WHERE slug='counter'`)
	if err := cmdSeed(db, []string{"--update"}); err != nil {
		t.Fatal(err)
	}
	checkCatalog(t, db)
	execTestSQL(t, db, `UPDATE patterns SET curriculum_order=NULL WHERE slug='counter'`)
	execTestSQL(t, db, `INSERT INTO patterns(slug,name,curriculum_order,created_at,updated_at) VALUES('custom','Custom',1,'then','then')`)
	before := catalogSnapshot(t, db)
	if err := cmdSeed(db, []string{"--update"}); err == nil {
		t.Fatal("occupied custom order accepted")
	}
	if catalogSnapshot(t, db) != before {
		t.Fatal("failed order refresh changed rows")
	}
}

func TestApplyRechecksConflictsAfterPreview(t *testing.T) {
	_, db := legacyCatalog(t)
	if _, err := reconcileCatalog(db, true, nil); err != nil {
		t.Fatal(err)
	}
	execTestSQL(t, db, `UPDATE patterns SET notes='written after preview' WHERE slug='dedup'`)
	before := catalogSnapshot(t, db)
	report, err := reconcileCatalog(db, false, nil)
	if err == nil || report == nil || !strings.Contains(strings.Join(report.Conflicts, "\n"), "dedup") {
		t.Fatalf("apply trusted stale preview: %+v %v", report, err)
	}
	if catalogSnapshot(t, db) != before {
		t.Fatal("intervening author edit lost")
	}
}
