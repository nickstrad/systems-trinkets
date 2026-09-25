package lab

import (
	"encoding/csv"
	"fmt"
	"os"
)

// Measurements is the CSV file a lesson writes and its analyze.sql reads.
// Rows sit in the csv.Writer's buffer until Close flushes them, so a panic
// mid-run leaves a partial file rather than a corrupt one.
type Measurements struct {
	file *os.File
	w    *csv.Writer
}

// NewMeasurements creates measurements.csv in the current directory and
// writes the header row. Lessons run from their own directory, which is where
// analyze.sql expects the file.
func NewMeasurements(columns ...string) *Measurements {
	file, err := os.Create("measurements.csv")
	Check(err)
	m := &Measurements{file: file, w: csv.NewWriter(file)}
	m.Write(columns...)
	return m
}

// Write appends one row. Callers format numbers themselves so the CSV shows
// exactly the precision the lesson chose.
func (m *Measurements) Write(fields ...string) {
	Check(m.w.Write(fields))
}

// Close flushes buffered rows, surfaces any write error, and closes the file.
func (m *Measurements) Close() {
	m.w.Flush()
	Check(m.w.Error())
	Check(m.file.Close())
	fmt.Println("wrote measurements.csv")
}
