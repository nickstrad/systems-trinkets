package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func testDBPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "trinkets.db")
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func insertTestPattern(t *testing.T, db *sql.DB, id int64, slug, family string, curriculumOrder any) {
	t.Helper()
	mustExec(t, db, `INSERT INTO patterns (id, slug, name, family, curriculum_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		id, slug, slug, family, curriculumOrder)
}

func assertCurriculumIndex(t *testing.T, db *sql.DB) {
	t.Helper()
	var unique, partial int
	if err := db.QueryRow("SELECT [unique], partial FROM pragma_index_list(?) WHERE name = ?", "patterns", curriculumOrderIndex).Scan(&unique, &partial); err != nil {
		t.Fatalf("curriculum index: %v", err)
	}
	if unique != 1 || partial != 1 {
		t.Fatalf("curriculum index = unique=%d partial=%d, want unique partial", unique, partial)
	}
	var column string
	if err := db.QueryRow("SELECT name FROM pragma_index_info(?) WHERE seqno = 0", curriculumOrderIndex).Scan(&column); err != nil {
		t.Fatalf("curriculum index column: %v", err)
	}
	if column != "curriculum_order" {
		t.Fatalf("curriculum index column = %q, want curriculum_order", column)
	}
	var definition string
	if err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?", curriculumOrderIndex).Scan(&definition); err != nil {
		t.Fatalf("curriculum index definition: %v", err)
	}
	if definition != curriculumOrderIndexDDL {
		t.Fatalf("curriculum index definition = %q, want %q", definition, curriculumOrderIndexDDL)
	}
}

func assertNoForeignKeyViolations(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatalf("foreign key violation after migration")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenDBFreshCurriculumOrderConstraintsAndJSON(t *testing.T) {
	db, err := openDB(testDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	has, err := hasColumn(db, "patterns", "curriculum_order")
	if err != nil || !has {
		t.Fatalf("curriculum_order column = %t, %v", has, err)
	}
	assertCurriculumIndex(t, db)

	insertTestPattern(t, db, 1, "first-custom", "custom", nil)
	insertTestPattern(t, db, 2, "second-custom", "custom", nil)
	insertTestPattern(t, db, 3, "course", "curriculum", 3)

	for _, tc := range []struct {
		name  string
		order any
	}{
		{"duplicate", 3},
		{"zero", 0},
		{"negative", -1},
		{"fraction", 1.5},
		{"text", "not-a-number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.Exec(`INSERT INTO patterns (slug, name, curriculum_order, created_at, updated_at)
				VALUES (?, ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, tc.name, tc.name, tc.order)
			if err == nil {
				t.Fatalf("curriculum order %v was accepted", tc.order)
			}
		})
	}

	p, err := getPattern(db, "course")
	if err != nil {
		t.Fatal(err)
	}
	if p.CurriculumOrder == nil || *p.CurriculumOrder != 3 {
		t.Fatalf("course curriculum order = %v, want 3", p.CurriculumOrder)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(b, &encoded); err != nil {
		t.Fatal(err)
	}
	if got := string(encoded["curriculum_order"]); got != "3" {
		t.Fatalf("JSON curriculum_order = %s, want 3", got)
	}

	custom, err := getPattern(db, "first-custom")
	if err != nil {
		t.Fatal(err)
	}
	b, err = json.Marshal(custom)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &encoded); err != nil {
		t.Fatal(err)
	}
	if got := string(encoded["curriculum_order"]); got != "null" {
		t.Fatalf("JSON curriculum_order = %s, want null", got)
	}
}

func TestOpenDBMigratesCurrentLegacySchemaAndPreservesRelationships(t *testing.T) {
	path := testDBPath(t)
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, legacy, "PRAGMA foreign_keys = ON")
	mustExec(t, legacy, `CREATE TABLE patterns (
		id INTEGER PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
		family TEXT NOT NULL DEFAULT '', explanation TEXT NOT NULL DEFAULT '',
		use_cases TEXT NOT NULL DEFAULT '', invariants TEXT NOT NULL DEFAULT '[]' CHECK (json_type(invariants) = 'array'),
		readings TEXT NOT NULL DEFAULT '[]' CHECK (json_type(readings) = 'array'), notes TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`)
	mustExec(t, legacy, `CREATE TABLE engines (
		id INTEGER PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
		notes TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`)
	mustExec(t, legacy, `CREATE TABLE approaches (
		id INTEGER PRIMARY KEY, pattern_id INTEGER NOT NULL REFERENCES patterns (id) ON DELETE CASCADE,
		engine_id INTEGER NOT NULL REFERENCES engines (id) ON DELETE CASCADE, title TEXT NOT NULL,
		primitives TEXT NOT NULL DEFAULT '', writeup TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
		UNIQUE (pattern_id, engine_id, title), UNIQUE (id, pattern_id, engine_id)
	)`)
	mustExec(t, legacy, `CREATE TABLE attempts (
		id INTEGER PRIMARY KEY, pattern_id INTEGER NOT NULL REFERENCES patterns (id) ON DELETE CASCADE,
		engine_id INTEGER NOT NULL REFERENCES engines (id) ON DELETE CASCADE, approach_id INTEGER,
		title TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','in_progress','done','abandoned')),
		lessons TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
		FOREIGN KEY (approach_id, pattern_id, engine_id) REFERENCES approaches (id, pattern_id, engine_id)
	)`)
	mustExec(t, legacy, "INSERT INTO patterns VALUES (41, 'legacy', 'Legacy', 'state', 'preserved explanation', 'preserved use case', '[\"must hold\"]', '[]', 'preserved notes', '2020-01-01', '2021-01-01')")
	mustExec(t, legacy, "INSERT INTO engines VALUES (51, 'sqlite', 'SQLite', '', '2020-01-01', '2021-01-01')")
	mustExec(t, legacy, "INSERT INTO approaches VALUES (61, 41, 51, 'Approach', 'BEGIN', 'preserved writeup', '2020-01-01', '2021-01-01')")
	mustExec(t, legacy, "INSERT INTO attempts VALUES (71, 41, 51, 61, 'Attempt', 'done', 'lesson', '2020-01-01', '2021-01-01')")
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := getPattern(db, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 41 || p.CurriculumOrder != nil || p.Notes != "preserved notes" || p.CreatedAt != "2020-01-01" {
		t.Fatalf("legacy pattern changed: %#v", p)
	}
	a, err := getApproach(db, 61)
	if err != nil || a.PatternID != 41 || a.Writeup != "preserved writeup" {
		t.Fatalf("legacy approach = %#v, %v", a, err)
	}
	attempt, err := getAttempt(db, 71)
	if err != nil || attempt.PatternID != 41 || attempt.ApproachID == nil || *attempt.ApproachID != 61 {
		t.Fatalf("legacy attempt = %#v, %v", attempt, err)
	}
	assertCurriculumIndex(t, db)
	assertNoForeignKeyViolations(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = openDB(path)
	if err != nil {
		t.Fatalf("repeated migration: %v", err)
	}
	defer db.Close()
	p, err = getPattern(db, "legacy")
	if err != nil || p.ID != 41 || p.CurriculumOrder != nil {
		t.Fatalf("repeated migration changed legacy pattern: %#v, %v", p, err)
	}
}

func TestOpenDBMigratesEarlierInvariantAndReadingsSchema(t *testing.T) {
	path := testDBPath(t)
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, legacy, `CREATE TABLE patterns (
		id INTEGER PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
		family TEXT NOT NULL DEFAULT '', invariant TEXT NOT NULL DEFAULT '', readings TEXT NOT NULL DEFAULT '',
		notes TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`)
	mustExec(t, legacy, "INSERT INTO patterns VALUES (7, 'old', 'Old', 'legacy', 'first\nsecond', 'Docs - https://example.test/docs', 'keep me', '2020-01-01', '2021-01-01')")
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, err := getPattern(db, "old")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 7 || p.CurriculumOrder != nil || !reflect.DeepEqual(p.Invariants, []string{"first", "second"}) {
		t.Fatalf("earlier migration pattern = %#v", p)
	}
	if len(p.Readings) != 1 || p.Readings[0].URL != "https://example.test/docs" || p.Notes != "keep me" {
		t.Fatalf("earlier migration readings/notes = %#v", p)
	}
	has, err := hasColumn(db, "patterns", "invariant")
	if err != nil || has {
		t.Fatalf("legacy invariant column remains = %t, %v", has, err)
	}
	assertCurriculumIndex(t, db)
}

func TestCurriculumOrderMigrationRollsBackOnIncompatibleExistingIndex(t *testing.T) {
	path := testDBPath(t)
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, legacy, `CREATE TABLE patterns (
		id INTEGER PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
		family TEXT NOT NULL DEFAULT '', explanation TEXT NOT NULL DEFAULT '', use_cases TEXT NOT NULL DEFAULT '',
		invariants TEXT NOT NULL DEFAULT '[]' CHECK (json_type(invariants) = 'array'),
		readings TEXT NOT NULL DEFAULT '[]' CHECK (json_type(readings) = 'array'), notes TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`)
	mustExec(t, legacy, "CREATE INDEX "+curriculumOrderIndex+" ON patterns (slug)")
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := openDB(path); err == nil {
		t.Fatal("openDB accepted an incompatible curriculum order index")
	}

	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	has, err := hasColumn(check, "patterns", "curriculum_order")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("failed migration left curriculum_order column behind")
	}
}

func TestCurriculumOrderMigrationRejectsMalformedIndexDefinition(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index string
	}{
		{
			name:  "multiple columns",
			index: "CREATE UNIQUE INDEX " + curriculumOrderIndex + " ON patterns (curriculum_order, slug) WHERE curriculum_order IS NOT NULL",
		},
		{
			name:  "weaker predicate",
			index: "CREATE UNIQUE INDEX " + curriculumOrderIndex + " ON patterns (curriculum_order) WHERE curriculum_order > 10",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := testDBPath(t)
			legacy, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			mustExec(t, legacy, `CREATE TABLE patterns (
				id INTEGER PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
				family TEXT NOT NULL DEFAULT '', explanation TEXT NOT NULL DEFAULT '', use_cases TEXT NOT NULL DEFAULT '',
				invariants TEXT NOT NULL DEFAULT '[]' CHECK (json_type(invariants) = 'array'),
				readings TEXT NOT NULL DEFAULT '[]' CHECK (json_type(readings) = 'array'), notes TEXT NOT NULL DEFAULT '',
				curriculum_order INTEGER CHECK (curriculum_order IS NULL OR (typeof(curriculum_order) = 'integer' AND curriculum_order > 0)),
				created_at TEXT NOT NULL, updated_at TEXT NOT NULL
			)`)
			mustExec(t, legacy, tc.index)
			if err := legacy.Close(); err != nil {
				t.Fatal(err)
			}

			if _, err := openDB(path); err == nil {
				t.Fatalf("openDB accepted malformed index %q", tc.index)
			}

			check, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer check.Close()
			var definition string
			if err := check.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?", curriculumOrderIndex).Scan(&definition); err != nil {
				t.Fatal(err)
			}
			if definition != tc.index {
				t.Fatalf("malformed index changed to %q, want %q", definition, tc.index)
			}
		})
	}
}

func TestCurriculumOrderMigrationRejectsIncompatibleExistingColumn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		column string
	}{
		{
			name:   "wrong type",
			column: "curriculum_order TEXT CHECK (curriculum_order IS NULL OR (typeof(curriculum_order) = 'integer' AND curriculum_order > 0))",
		},
		{
			name:   "missing check",
			column: "curriculum_order INTEGER",
		},
		{
			name:   "check text in comment and unrelated string",
			column: `curriculum_order INTEGER /* CHECK (curriculum_order IS NULL OR (typeof(curriculum_order) = 'integer' AND curriculum_order > 0)) */`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := testDBPath(t)
			legacy, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			mustExec(t, legacy, `CREATE TABLE patterns (
				id INTEGER PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
				family TEXT NOT NULL DEFAULT '', explanation TEXT NOT NULL DEFAULT '', use_cases TEXT NOT NULL DEFAULT '',
				invariants TEXT NOT NULL DEFAULT '[]' CHECK (json_type(invariants) = 'array'),
				readings TEXT NOT NULL DEFAULT '[]' CHECK (json_type(readings) = 'array'), notes TEXT NOT NULL DEFAULT 'CHECK (curriculum_order IS NULL OR (typeof(curriculum_order) = ''integer'' AND curriculum_order > 0))',
				`+tc.column+`,
				created_at TEXT NOT NULL, updated_at TEXT NOT NULL
			)`)
			if err := legacy.Close(); err != nil {
				t.Fatal(err)
			}

			if _, err := openDB(path); err == nil {
				t.Fatalf("openDB accepted incompatible existing column %q", tc.column)
			}

			check, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer check.Close()
			var indexCount int
			if err := check.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?", curriculumOrderIndex).Scan(&indexCount); err != nil {
				t.Fatal(err)
			}
			if indexCount != 0 {
				t.Fatal("failed migration created the curriculum order index")
			}
		})
	}
}

func TestCurriculumOrderMigrationAcceptsCanonicalExistingColumn(t *testing.T) {
	path := testDBPath(t)
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, legacy, `CREATE TABLE patterns (
		id INTEGER PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
		family TEXT NOT NULL DEFAULT '', explanation TEXT NOT NULL DEFAULT '', use_cases TEXT NOT NULL DEFAULT '',
		invariants TEXT NOT NULL DEFAULT '[]' CHECK (json_type(invariants) = 'array'),
		readings TEXT NOT NULL DEFAULT '[]' CHECK (json_type(readings) = 'array'), notes TEXT NOT NULL DEFAULT '',
		curriculum_order INTEGER CHECK (curriculum_order IS NULL OR (typeof(curriculum_order) = 'integer' AND curriculum_order > 0)),
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`)
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := openDB(path)
	if err != nil {
		t.Fatalf("openDB rejected canonical existing column: %v", err)
	}
	defer db.Close()
	assertCurriculumIndex(t, db)
	for _, order := range []any{0, -1, 1.5, "not-positive"} {
		if _, err := db.Exec(`INSERT INTO patterns (slug, name, curriculum_order, created_at, updated_at)
			VALUES (?, ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, fmt.Sprintf("invalid-%v", order), "Invalid", order); err == nil {
			t.Fatalf("canonical existing column accepted %v", order)
		}
	}
}

func TestPatternAndMatrixUseCurriculumOrderThenCustomFamilySlug(t *testing.T) {
	db, err := openDB(testDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	insertTestPattern(t, db, 1, "custom-beta", "beta", nil)
	insertTestPattern(t, db, 2, "course-four", "alpha", 4)
	insertTestPattern(t, db, 3, "custom-alpha", "alpha", nil)
	insertTestPattern(t, db, 4, "course-two", "zeta", 2)
	insertTestPattern(t, db, 5, "custom-zeta", "zeta", nil)

	want := []string{"course-two", "course-four", "custom-alpha", "custom-beta", "custom-zeta"}
	patterns, err := listPatterns(db, "")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(patterns))
	for i, p := range patterns {
		got[i] = p.Slug
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pattern order = %v, want %v", got, want)
	}

	rows, _, err := buildMatrix(db, "")
	if err != nil {
		t.Fatal(err)
	}
	got = make([]string, len(rows))
	for i, row := range rows {
		got[i] = row.Pattern
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matrix order = %v, want %v", got, want)
	}
	if rows[0].CurriculumOrder == nil || *rows[0].CurriculumOrder != 2 || rows[2].CurriculumOrder != nil {
		t.Fatalf("matrix curriculum order values = %#v, %#v", rows[0].CurriculumOrder, rows[2].CurriculumOrder)
	}

	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" {
		t.Fatal("matrix JSON was empty")
	}
}
