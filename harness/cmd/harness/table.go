package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"
)

// nullCell is what an SQL NULL renders as in the box and markdown formats.
const nullCell = "∅"

// Table is one result set: ordered column names plus raw cell values. It is
// built from whatever columns a query happened to return, so nothing here
// knows the schema.
type Table struct {
	Cols []string
	Rows [][]any
}

// ScanTable drains rows into a Table. []byte cells become string so they
// format as text and marshal as JSON strings rather than base64.
func ScanTable(rows *sql.Rows) (*Table, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}
	t := &Table{Cols: cols}
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		for i, c := range cells {
			if b, ok := c.([]byte); ok {
				cells[i] = string(b)
			}
		}
		t.Rows = append(t.Rows, cells)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read rows: %w", err)
	}
	return t, nil
}

// Render writes the table as format: "box" (aligned columns under a header
// rule), "md" (a markdown table) or "json" (an array of objects).
func (t *Table) Render(w io.Writer, format string) error {
	switch format {
	case "", "box":
		return t.renderBox(w)
	case "md":
		return t.renderMarkdown(w)
	case "json":
		return t.renderJSON(w)
	default:
		return fmt.Errorf("unknown format %q (want box, md or json)", format)
	}
}

// cells formats every row once, so the widths and the output agree.
func (t *Table) cells() [][]string {
	out := make([][]string, len(t.Rows))
	for i, row := range t.Rows {
		line := make([]string, len(t.Cols))
		for j := range t.Cols {
			if j < len(row) {
				line[j] = formatCell(row[j])
			}
		}
		out[i] = line
	}
	return out
}

// numericCols reports, per column, whether every non-nil value in it is a
// number — those columns are right-aligned.
func (t *Table) numericCols() []bool {
	num := make([]bool, len(t.Cols))
	seen := make([]bool, len(t.Cols))
	for j := range num {
		num[j] = true
	}
	for _, row := range t.Rows {
		for j := range t.Cols {
			if j >= len(row) || row[j] == nil {
				continue
			}
			seen[j] = true
			if !isNumeric(row[j]) {
				num[j] = false
			}
		}
	}
	for j := range num {
		num[j] = num[j] && seen[j]
	}
	return num
}

func (t *Table) renderBox(w io.Writer) error {
	cells := t.cells()
	right := t.numericCols()
	width := make([]int, len(t.Cols))
	for j, c := range t.Cols {
		width[j] = utf8.RuneCountInString(c)
	}
	for _, row := range cells {
		for j, c := range row {
			width[j] = max(width[j], utf8.RuneCountInString(c))
		}
	}

	var b bytes.Buffer
	writeRow := func(vals []string) {
		var line strings.Builder
		for j, v := range vals {
			if j > 0 {
				line.WriteString("  ")
			}
			line.WriteString(pad(v, width[j], right[j]))
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	writeRow(t.Cols)
	rule := make([]string, len(t.Cols))
	for j := range rule {
		rule[j] = strings.Repeat("-", width[j])
	}
	writeRow(rule)
	for _, row := range cells {
		writeRow(row)
	}
	if len(cells) == 0 {
		b.WriteString("(0 rows)\n")
	}
	_, err := w.Write(b.Bytes())
	return err
}

func (t *Table) renderMarkdown(w io.Writer) error {
	right := t.numericCols()
	var b bytes.Buffer
	b.WriteString("| " + strings.Join(escapeAll(t.Cols), " | ") + " |\n")
	rule := make([]string, len(t.Cols))
	for j := range rule {
		if right[j] {
			rule[j] = "---:"
		} else {
			rule[j] = "---"
		}
	}
	b.WriteString("| " + strings.Join(rule, " | ") + " |\n")
	for _, row := range t.cells() {
		b.WriteString("| " + strings.Join(escapeAll(row), " | ") + " |\n")
	}
	_, err := w.Write(b.Bytes())
	return err
}

// renderJSON writes an array of objects, keeping the column order (a Go map
// would sort the keys).
func (t *Table) renderJSON(w io.Writer) error {
	var b bytes.Buffer
	b.WriteString("[\n")
	for i, row := range t.Rows {
		b.WriteString("  {")
		for j, col := range t.Cols {
			if j > 0 {
				b.WriteString(", ")
			}
			key, err := json.Marshal(col)
			if err != nil {
				return fmt.Errorf("marshal column %q: %w", col, err)
			}
			var cell any
			if j < len(row) {
				cell = jsonValue(row[j])
			}
			val, err := json.Marshal(cell)
			if err != nil {
				return fmt.Errorf("marshal %s: %w", col, err)
			}
			b.Write(key)
			b.WriteString(": ")
			b.Write(val)
		}
		b.WriteString("}")
		if i < len(t.Rows)-1 {
			b.WriteString(",")
		}
		b.WriteByte('\n')
	}
	b.WriteString("]\n")
	_, err := w.Write(b.Bytes())
	return err
}

// formatCell renders one cell: times as RFC3339, floats with two decimals,
// NULL as ∅, everything else with its default Go formatting.
func formatCell(v any) string {
	switch x := v.(type) {
	case nil:
		return nullCell
	case time.Time:
		return x.Format(time.RFC3339)
	case float32:
		return fmt.Sprintf("%.2f", x)
	case float64:
		return fmt.Sprintf("%.2f", x)
	default:
		return fmt.Sprint(x)
	}
}

// jsonValue adapts a cell for encoding/json: DuckDB's big integers and
// decimals become their decimal text. (ScanTable already turned []byte into
// string.)
func jsonValue(v any) any {
	switch x := v.(type) {
	case *big.Int:
		return x.String()
	case *big.Float:
		return x.Text('f', -1)
	default:
		return v
	}
}

func isNumeric(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, *big.Int, *big.Float:
		return true
	default:
		return false
	}
}

func pad(s string, width int, right bool) string {
	gap := width - utf8.RuneCountInString(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

func escapeAll(vals []string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = strings.ReplaceAll(strings.ReplaceAll(v, "|", "\\|"), "\n", " ")
	}
	return out
}
