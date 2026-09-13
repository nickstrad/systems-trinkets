package main

import (
	"database/sql"
	"flag"
	"fmt"
	"strings"
)

// cmdMatrix prints the core map: patterns down the side, engines across the
// top, the primitives that carry the guarantee in each square.
func cmdMatrix(db *sql.DB, args []string) error {
	fs := flag.NewFlagSet("matrix", flag.ContinueOnError)
	fs.Usage = usageFor(fs, "trinkets matrix [--family F] [--counts] [--width N] [--json]")
	family := fs.String("family", "", "only patterns in this family")
	counts := fs.Bool("counts", false, "show approach/attempt counts instead of primitives")
	width := fs.Int("width", 34, "characters per cell when showing primitives")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rows, engines, err := buildMatrix(db, *family)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(rows)
	}
	if len(engines) == 0 {
		return fmt.Errorf("no engines yet; run \"trinkets seed\" or \"trinkets engine add\"")
	}

	w := newTabWriter()
	header := []string{"ORDER", "PATTERN"}
	for _, e := range engines {
		header = append(header, strings.ToUpper(e.Slug))
	}
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, r := range rows {
		cells := []string{curriculumOrderText(r.CurriculumOrder), r.Pattern}
		for _, e := range engines {
			c := r.Cells[e.Slug]
			if *counts {
				cells = append(cells, fmt.Sprintf("%d approach / %d attempt", c.Approaches, c.Attempts))
				continue
			}
			text := truncate(c.Primitives, *width)
			if text == "" {
				text = "-"
			}
			if c.Attempts > 0 {
				text += fmt.Sprintf(" [%d tried]", c.Attempts)
			}
			cells = append(cells, text)
		}
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d pattern(s) x %d engine(s)\n", len(rows), len(engines))
	return nil
}
