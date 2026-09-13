package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// schemaSQL is idempotent: running it against an existing database is a no-op.
//
//	patterns   - a system behavior to build (queue, lease, rate limiter, ...)
//	engines    - a storage engine to build it on (valkey, sqlite, postgres)
//	approaches - how to build one pattern on one engine; many per pair
//	attempts   - a record of actually trying it, and what it taught
const schemaSQL = `
CREATE TABLE IF NOT EXISTS patterns (
  id          INTEGER PRIMARY KEY,
  slug        TEXT NOT NULL UNIQUE,
  name        TEXT NOT NULL,
  family      TEXT NOT NULL DEFAULT '',
  explanation TEXT NOT NULL DEFAULT '',
  use_cases   TEXT NOT NULL DEFAULT '',
  -- JSON arrays, kept as text so json_each()/jsonb() work on them:
  --   invariants: ["...", "..."]
  --   readings:   [{"title": "...", "url": "..."}, ...]
  invariants  TEXT NOT NULL DEFAULT '[]' CHECK (json_type(invariants) = 'array'),
  readings    TEXT NOT NULL DEFAULT '[]' CHECK (json_type(readings) = 'array'),
  notes       TEXT NOT NULL DEFAULT '',
  curriculum_order INTEGER CHECK (curriculum_order IS NULL OR
                     (typeof(curriculum_order) = 'integer' AND curriculum_order > 0)),
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS patterns_family ON patterns (family);

CREATE TABLE IF NOT EXISTS engines (
  id         INTEGER PRIMARY KEY,
  slug       TEXT NOT NULL UNIQUE,
  name       TEXT NOT NULL,
  notes      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS approaches (
  id         INTEGER PRIMARY KEY,
  pattern_id INTEGER NOT NULL REFERENCES patterns (id) ON DELETE CASCADE,
  engine_id  INTEGER NOT NULL REFERENCES engines (id) ON DELETE CASCADE,
  title      TEXT NOT NULL,
  primitives TEXT NOT NULL DEFAULT '',
  writeup    TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  -- Several approaches per (pattern, engine); their titles distinguish them.
  UNIQUE (pattern_id, engine_id, title),
  -- Target for the composite foreign key on attempts below.
  UNIQUE (id, pattern_id, engine_id)
);

CREATE INDEX IF NOT EXISTS approaches_pattern ON approaches (pattern_id);
CREATE INDEX IF NOT EXISTS approaches_engine  ON approaches (engine_id);

CREATE TABLE IF NOT EXISTS attempts (
  id          INTEGER PRIMARY KEY,
  pattern_id  INTEGER NOT NULL REFERENCES patterns (id) ON DELETE CASCADE,
  engine_id   INTEGER NOT NULL REFERENCES engines (id) ON DELETE CASCADE,
  approach_id INTEGER,
  title       TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'planned'
              CHECK (status IN ('planned','in_progress','done','abandoned')),
  lessons     TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  -- An attempt may cite no approach, but if it cites one that approach must be
  -- for the same pattern and engine. SQLite skips the check when approach_id is
  -- NULL, so the optional link costs nothing.
  FOREIGN KEY (approach_id, pattern_id, engine_id)
    REFERENCES approaches (id, pattern_id, engine_id)
);

CREATE INDEX IF NOT EXISTS attempts_pattern ON attempts (pattern_id);
CREATE INDEX IF NOT EXISTS attempts_engine  ON attempts (engine_id);
CREATE INDEX IF NOT EXISTS attempts_status  ON attempts (status);
`

func openDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One writer, one process: the CLI is short-lived and this keeps the
	// pragmas above true of every connection it uses.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	for _, m := range migrations {
		if err := m(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	return db, nil
}

// migrations bring a database created by an older schemaSQL up to date. Each
// one is idempotent; CREATE TABLE IF NOT EXISTS never touches existing tables,
// so columns added later have to be added here too.
var migrations = []func(*sql.DB) error{
	func(db *sql.DB) error {
		return addColumn(db, "patterns", "explanation", "TEXT NOT NULL DEFAULT ''")
	},
	func(db *sql.DB) error {
		return addColumn(db, "patterns", "use_cases", "TEXT NOT NULL DEFAULT ''")
	},
	func(db *sql.DB) error {
		return addColumn(db, "patterns", "readings", "TEXT NOT NULL DEFAULT '[]' CHECK (json_type(readings) = 'array')")
	},
	// The single-text `invariant` column became a JSON array called
	// `invariants`; each old line becomes one element.
	func(db *sql.DB) error {
		has, err := hasColumn(db, "patterns", "invariant")
		if err != nil || !has {
			return err
		}
		if err := addColumn(db, "patterns", "invariants", "TEXT NOT NULL DEFAULT '[]' CHECK (json_type(invariants) = 'array')"); err != nil {
			return err
		}
		if err := rewriteColumn(db, "patterns", "invariant", "invariants", func(old string) (string, error) {
			return jsonText(splitLines(old))
		}); err != nil {
			return err
		}
		_, err = db.Exec("ALTER TABLE patterns DROP COLUMN invariant")
		return err
	},
	// readings briefly held "Title - URL" lines before becoming JSON.
	func(db *sql.DB) error {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM patterns WHERE NOT json_valid(readings)").Scan(&n); err != nil || n == 0 {
			return err
		}
		return rewriteColumn(db, "patterns", "readings", "readings", func(old string) (string, error) {
			if old == "" {
				return "[]", nil
			}
			var rs []Reading
			for _, l := range splitLines(old) {
				r, err := parseReading(l)
				if err != nil {
					return "", err
				}
				rs = append(rs, r)
			}
			return jsonText(rs)
		})
	},
	migrateCurriculumOrder,
}

const curriculumOrderColumn = "INTEGER CHECK (curriculum_order IS NULL OR (typeof(curriculum_order) = 'integer' AND curriculum_order > 0))"
const curriculumOrderIndex = "patterns_curriculum_order_unique"
const curriculumOrderIndexDDL = "CREATE UNIQUE INDEX patterns_curriculum_order_unique ON patterns (curriculum_order) WHERE curriculum_order IS NOT NULL"

// migrateCurriculumOrder keeps the column and its partial unique index in one
// transaction. The index belongs here rather than schemaSQL: legacy databases
// need the column before SQLite can create an index that references it.
func migrateCurriculumOrder(db *sql.DB) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var hasColumn bool
	if err = tx.QueryRow("SELECT count(*) > 0 FROM pragma_table_info(?) WHERE name = ?", "patterns", "curriculum_order").Scan(&hasColumn); err != nil {
		return err
	}
	if !hasColumn {
		if _, err = tx.Exec("ALTER TABLE patterns ADD COLUMN curriculum_order " + curriculumOrderColumn); err != nil {
			return err
		}
	} else if err = validateCurriculumOrderColumn(tx); err != nil {
		return err
	}

	var indexSQL string
	err = tx.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?", curriculumOrderIndex).Scan(&indexSQL)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(curriculumOrderIndexDDL)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if indexSQL != curriculumOrderIndexDDL {
		return fmt.Errorf("existing index %s is not the curriculum order unique partial index", curriculumOrderIndex)
	}

	err = tx.Commit()
	return err
}

// validateCurriculumOrderColumn fail-closes when an existing database already
// has the column. SQLite's table-info pragma reports the declared type but not
// its CHECK constraints, so inspect the column definition in the saved CREATE
// TABLE statement. Comparing the extracted definition as tokens keeps comments
// and unrelated string literals from satisfying the check by accident.
func validateCurriculumOrderColumn(tx *sql.Tx) error {
	var tableSQL string
	if err := tx.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", "patterns").Scan(&tableSQL); err != nil {
		return err
	}
	definition, err := tableColumnDefinition(tableSQL, "curriculum_order")
	if err != nil {
		return fmt.Errorf("read existing curriculum_order column: %w", err)
	}
	want, err := sqlTokens(curriculumOrderColumn)
	if err != nil {
		return fmt.Errorf("parse curriculum_order definition: %w", err)
	}
	if !sameTokens(definition, append([]string{"curriculum_order"}, want...)) {
		return errors.New("existing curriculum_order column is not nullable INTEGER with the required positive integer CHECK")
	}
	return nil
}

// tableColumnDefinition returns one top-level column definition from a CREATE
// TABLE statement. It deliberately tokenizes SQL instead of matching strings:
// SQLite preserves comments and arbitrary text in sqlite_master.
func tableColumnDefinition(tableSQL, column string) ([]string, error) {
	tokens, err := sqlTokens(tableSQL)
	if err != nil {
		return nil, err
	}
	open := -1
	for i, token := range tokens {
		if token == "(" {
			open = i
			break
		}
	}
	if open < 0 {
		return nil, errors.New("CREATE TABLE has no column list")
	}

	depth := 0
	start := open + 1
	for i := start; i < len(tokens); i++ {
		switch tokens[i] {
		case "(":
			depth++
		case ")":
			if depth == 0 {
				if definition := matchingColumnDefinition(tokens[start:i], column); definition != nil {
					return definition, nil
				}
				return nil, fmt.Errorf("column %q not found", column)
			}
			depth--
		case ",":
			if depth == 0 {
				if definition := matchingColumnDefinition(tokens[start:i], column); definition != nil {
					return definition, nil
				}
				start = i + 1
			}
		}
	}
	return nil, errors.New("unterminated CREATE TABLE column list")
}

func matchingColumnDefinition(definition []string, column string) []string {
	if len(definition) != 0 && definition[0] == column {
		return definition
	}
	return nil
}

// sqlTokens emits only the tokens relevant to SQL structure. It discards line
// and block comments and retains quoted strings as single tokens.
func sqlTokens(text string) ([]string, error) {
	var tokens []string
	for i := 0; i < len(text); {
		switch {
		case strings.ContainsRune(" \t\r\n\f", rune(text[i])):
			i++
		case i+1 < len(text) && text[i:i+2] == "--":
			i += 2
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case i+1 < len(text) && text[i:i+2] == "/*":
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return nil, errors.New("unterminated block comment")
			}
			i += end + 4
		case text[i] == '\'':
			start := i
			i++
			for i < len(text) {
				if text[i] != '\'' {
					i++
					continue
				}
				i++
				if i < len(text) && text[i] == '\'' {
					i++
					continue
				}
				break
			}
			if i > len(text) || text[i-1] != '\'' {
				return nil, errors.New("unterminated string")
			}
			tokens = append(tokens, text[start:i])
		case text[i] == '"' || text[i] == '`' || text[i] == '[':
			start, quote := i, text[i]
			close := quote
			if quote == '[' {
				close = ']'
			}
			i++
			for i < len(text) && text[i] != close {
				i++
			}
			if i == len(text) {
				return nil, errors.New("unterminated quoted identifier")
			}
			i++
			tokens = append(tokens, text[start:i])
		case isSQLIdentifierChar(text[i]):
			start := i
			for i < len(text) && isSQLIdentifierChar(text[i]) {
				i++
			}
			tokens = append(tokens, strings.ToLower(text[start:i]))
		default:
			tokens = append(tokens, string(text[i]))
			i++
		}
	}
	return tokens, nil
}

func isSQLIdentifierChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

func sameTokens(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	var n int
	err := db.QueryRow("SELECT count(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&n)
	return n > 0, err
}

func addColumn(db *sql.DB, table, column, decl string) error {
	has, err := hasColumn(db, table, column)
	if err != nil || has {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
}

// rewriteColumn copies every row's `from` column into `to` through conv.
func rewriteColumn(db *sql.DB, table, from, to string, conv func(string) (string, error)) error {
	rows, err := db.Query(fmt.Sprintf("SELECT id, %s FROM %s", from, table))
	if err != nil {
		return err
	}
	type pair struct {
		id  int64
		val string
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.val); err != nil {
			rows.Close()
			return err
		}
		pairs = append(pairs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range pairs {
		v, err := conv(p.val)
		if err != nil {
			return err
		}
		if _, err := db.Exec(fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", table, to), v, p.id); err != nil {
			return err
		}
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }
