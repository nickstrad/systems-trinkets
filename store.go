package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var errNotFound = errors.New("not found")

type Pattern struct {
	ID              int64     `json:"id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	Family          string    `json:"family"`
	Explanation     string    `json:"explanation"`
	UseCases        string    `json:"use_cases"`
	Invariants      []string  `json:"invariants"`
	Readings        []Reading `json:"readings"`
	Notes           string    `json:"notes"`
	CurriculumOrder *int      `json:"curriculum_order"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}

// Reading is one entry of a pattern's readings list.
type Reading struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// String renders a reading the way the --reading flag accepts it.
func (r Reading) String() string { return r.Title + " - " + r.URL }

// parseReading splits "Title - URL" on its last " - " so titles may contain
// dashes; a bare URL is accepted with the URL as its title.
func parseReading(s string) (Reading, error) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, " - "); i >= 0 {
		return Reading{Title: strings.TrimSpace(s[:i]), URL: strings.TrimSpace(s[i+3:])}, nil
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return Reading{Title: s, URL: s}, nil
	}
	return Reading{}, fmt.Errorf("reading %q: want 'Title - URL'", s)
}

// jsonText encodes a list column. nil encodes as [] rather than null so the
// CHECK (json_type(...) = 'array') on the column holds.
func jsonText(v any) (string, error) {
	if v == nil {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if string(b) == "null" {
		return "[]", nil
	}
	return string(b), nil
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

type Engine struct {
	ID        int64  `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Approach struct {
	ID          int64  `json:"id"`
	PatternID   int64  `json:"pattern_id"`
	PatternSlug string `json:"pattern"`
	EngineID    int64  `json:"engine_id"`
	EngineSlug  string `json:"engine"`
	Title       string `json:"title"`
	Primitives  string `json:"primitives"`
	Writeup     string `json:"writeup"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Attempt struct {
	ID            int64  `json:"id"`
	PatternID     int64  `json:"pattern_id"`
	PatternSlug   string `json:"pattern"`
	EngineID      int64  `json:"engine_id"`
	EngineSlug    string `json:"engine"`
	ApproachID    *int64 `json:"approach_id,omitempty"`
	ApproachTitle string `json:"approach_title,omitempty"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Lessons       string `json:"lessons"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// fields carries the column/value pairs a create or edit wants to write.
// Only columns the user actually supplied on the command line end up here, so
// an edit never clobbers a field that was left off the command.
type fields struct {
	cols []string
	vals []any
}

func (f *fields) set(col string, val any) {
	f.cols = append(f.cols, col)
	f.vals = append(f.vals, val)
}

func (f *fields) empty() bool { return len(f.cols) == 0 }

func (f *fields) insert(db *sql.DB, table string) (int64, error) {
	ts := now()
	f.set("created_at", ts)
	f.set("updated_at", ts)
	marks := strings.TrimSuffix(strings.Repeat("?,", len(f.cols)), ",")
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(f.cols, ", "), marks)
	res, err := db.Exec(q, f.vals...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (f *fields) update(db *sql.DB, table string, id int64) error {
	if f.empty() {
		return errors.New("nothing to update: pass at least one field flag")
	}
	f.set("updated_at", now())
	sets := make([]string, 0, len(f.cols))
	for _, c := range f.cols {
		sets = append(sets, c+" = ?")
	}
	q := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", table, strings.Join(sets, ", "))
	res, err := db.Exec(q, append(f.vals, id)...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

// ---- patterns -------------------------------------------------------------

const patternCols = "id, slug, name, family, explanation, use_cases, invariants, readings, notes, curriculum_order, created_at, updated_at"

func scanPattern(row interface{ Scan(...any) error }) (Pattern, error) {
	var p Pattern
	var invariants, readings string
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.Family, &p.Explanation, &p.UseCases, &invariants, &readings, &p.Notes, &p.CurriculumOrder, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(invariants), &p.Invariants); err != nil {
		return p, fmt.Errorf("pattern %s invariants: %w", p.Slug, err)
	}
	if err := json.Unmarshal([]byte(readings), &p.Readings); err != nil {
		return p, fmt.Errorf("pattern %s readings: %w", p.Slug, err)
	}
	return p, nil
}

func getPattern(db *sql.DB, slug string) (Pattern, error) {
	p, err := scanPattern(db.QueryRow("SELECT "+patternCols+" FROM patterns WHERE slug = ?", slug))
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("pattern %q: %w", slug, errNotFound)
	}
	return p, err
}

func listPatterns(db *sql.DB, family string) ([]Pattern, error) {
	q := "SELECT " + patternCols + " FROM patterns"
	var args []any
	if family != "" {
		q += " WHERE family = ?"
		args = append(args, family)
	}
	q += " ORDER BY curriculum_order IS NULL, curriculum_order, family, slug"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pattern
	for rows.Next() {
		p, err := scanPattern(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- engines --------------------------------------------------------------

const engineCols = "id, slug, name, notes, created_at, updated_at"

func scanEngine(row interface{ Scan(...any) error }) (Engine, error) {
	var e Engine
	err := row.Scan(&e.ID, &e.Slug, &e.Name, &e.Notes, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func getEngine(db *sql.DB, slug string) (Engine, error) {
	e, err := scanEngine(db.QueryRow("SELECT "+engineCols+" FROM engines WHERE slug = ?", slug))
	if errors.Is(err, sql.ErrNoRows) {
		return e, fmt.Errorf("engine %q: %w", slug, errNotFound)
	}
	return e, err
}

func listEngines(db *sql.DB) ([]Engine, error) {
	rows, err := db.Query("SELECT " + engineCols + " FROM engines ORDER BY slug")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Engine
	for rows.Next() {
		e, err := scanEngine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- approaches -----------------------------------------------------------

const approachSelect = `
SELECT a.id, a.pattern_id, p.slug, a.engine_id, e.slug, a.title, a.primitives,
       a.writeup, a.created_at, a.updated_at
FROM approaches a
JOIN patterns p ON p.id = a.pattern_id
JOIN engines  e ON e.id = a.engine_id`

func scanApproach(row interface{ Scan(...any) error }) (Approach, error) {
	var a Approach
	err := row.Scan(&a.ID, &a.PatternID, &a.PatternSlug, &a.EngineID, &a.EngineSlug, &a.Title,
		&a.Primitives, &a.Writeup, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func getApproach(db *sql.DB, id int64) (Approach, error) {
	a, err := scanApproach(db.QueryRow(approachSelect+" WHERE a.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, fmt.Errorf("approach %d: %w", id, errNotFound)
	}
	return a, err
}

type approachFilter struct {
	pattern string
	engine  string
}

func listApproaches(db *sql.DB, f approachFilter) ([]Approach, error) {
	q := approachSelect
	var where []string
	var args []any
	if f.pattern != "" {
		where, args = append(where, "p.slug = ?"), append(args, f.pattern)
	}
	if f.engine != "" {
		where, args = append(where, "e.slug = ?"), append(args, f.engine)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY p.slug, e.slug, a.id"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Approach
	for rows.Next() {
		a, err := scanApproach(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- attempts -------------------------------------------------------------

const attemptSelect = `
SELECT t.id, t.pattern_id, p.slug, t.engine_id, e.slug, t.approach_id,
       COALESCE(a.title, ''), t.title, t.status, t.lessons, t.created_at, t.updated_at
FROM attempts t
JOIN patterns p ON p.id = t.pattern_id
JOIN engines  e ON e.id = t.engine_id
LEFT JOIN approaches a ON a.id = t.approach_id`

func scanAttempt(row interface{ Scan(...any) error }) (Attempt, error) {
	var t Attempt
	err := row.Scan(&t.ID, &t.PatternID, &t.PatternSlug, &t.EngineID, &t.EngineSlug, &t.ApproachID,
		&t.ApproachTitle, &t.Title, &t.Status, &t.Lessons, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func getAttempt(db *sql.DB, id int64) (Attempt, error) {
	t, err := scanAttempt(db.QueryRow(attemptSelect+" WHERE t.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, fmt.Errorf("attempt %d: %w", id, errNotFound)
	}
	return t, err
}

type attemptFilter struct {
	pattern string
	engine  string
	status  string
}

func listAttempts(db *sql.DB, f attemptFilter) ([]Attempt, error) {
	q := attemptSelect
	var where []string
	var args []any
	if f.pattern != "" {
		where, args = append(where, "p.slug = ?"), append(args, f.pattern)
	}
	if f.engine != "" {
		where, args = append(where, "e.slug = ?"), append(args, f.engine)
	}
	if f.status != "" {
		where, args = append(where, "t.status = ?"), append(args, f.status)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY t.updated_at DESC, t.id DESC"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attempt
	for rows.Next() {
		t, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---- matrix ---------------------------------------------------------------

// matrixCell holds what one (pattern, engine) square of the core map knows.
type matrixCell struct {
	Primitives string `json:"primitives"`
	Approaches int    `json:"approaches"`
	Attempts   int    `json:"attempts"`
}

type matrixRow struct {
	Pattern         string                `json:"pattern"`
	Name            string                `json:"name"`
	Family          string                `json:"family"`
	CurriculumOrder *int                  `json:"curriculum_order"`
	Cells           map[string]matrixCell `json:"cells"`
}

func buildMatrix(db *sql.DB, family string) ([]matrixRow, []Engine, error) {
	patterns, err := listPatterns(db, family)
	if err != nil {
		return nil, nil, err
	}
	engines, err := listEngines(db)
	if err != nil {
		return nil, nil, err
	}
	approaches, err := listApproaches(db, approachFilter{})
	if err != nil {
		return nil, nil, err
	}
	attempts, err := listAttempts(db, attemptFilter{})
	if err != nil {
		return nil, nil, err
	}

	rows := make([]matrixRow, 0, len(patterns))
	index := make(map[string]int, len(patterns))
	for _, p := range patterns {
		index[p.Slug] = len(rows)
		rows = append(rows, matrixRow{Pattern: p.Slug, Name: p.Name, Family: p.Family, CurriculumOrder: p.CurriculumOrder, Cells: map[string]matrixCell{}})
	}
	for _, a := range approaches {
		i, ok := index[a.PatternSlug]
		if !ok {
			continue
		}
		c := rows[i].Cells[a.EngineSlug]
		c.Approaches++
		if c.Primitives == "" {
			c.Primitives = a.Primitives
		}
		rows[i].Cells[a.EngineSlug] = c
	}
	for _, t := range attempts {
		i, ok := index[t.PatternSlug]
		if !ok {
			continue
		}
		c := rows[i].Cells[t.EngineSlug]
		c.Attempts++
		rows[i].Cells[t.EngineSlug] = c
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CurriculumOrder != nil && rows[j].CurriculumOrder != nil {
			return *rows[i].CurriculumOrder < *rows[j].CurriculumOrder
		}
		if rows[i].CurriculumOrder != nil {
			return true
		}
		if rows[j].CurriculumOrder != nil {
			return false
		}
		if rows[i].Family != rows[j].Family {
			return rows[i].Family < rows[j].Family
		}
		return rows[i].Pattern < rows[j].Pattern
	})
	return rows, engines, nil
}
