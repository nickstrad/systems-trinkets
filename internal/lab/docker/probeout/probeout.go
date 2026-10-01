// Package probeout is the line format the harness's fixture binary
// (internal/lab/docker/probe) prints and the harness reads back. It is plain
// standard library so the probe can import it without linking the moby
// client, and it holds no verdicts: a line says what a check saw, and the
// lesson decides what that means.
//
// A line is one of
//
//	name: OK
//	name: OK <detail>
//	name: DENIED (<error>)
//
// OK means the check ran and the action or read succeeded, with the value in
// the detail (cap-eff: OK 0000000000000000). DENIED carries the error text.
// Names are lower-case [a-z0-9._-] starting with a letter or digit. Details
// hold no control characters, so one line is always one line of output.
package probeout

import (
	"errors"
	"fmt"
	"strings"
)

// Line is one parsed or constructed probe line. A line that did not parse
// keeps its text in Raw with Err set; its other fields mean nothing.
type Line struct {
	Name   string
	OK     bool
	Detail string
	Raw    string // malformed lines only: the text as read
	Err    error  // malformed lines only: why it did not parse
}

// OK builds a successful line. The detail is sanitized; name must satisfy
// ValidName.
func OK(name, detail string) Line { return Line{Name: name, OK: true, Detail: Sanitize(detail)} }

// Denied builds a failed line carrying the error text, sanitized.
func Denied(name string, err error) Line {
	return Line{Name: name, Detail: Sanitize(err.Error())}
}

// Malformed reports whether the line failed to parse.
func (l Line) Malformed() bool { return l.Err != nil }

// String is the canonical text of the line, without a newline. A malformed
// line gives back exactly what was read.
func (l Line) String() string {
	switch {
	case l.Err != nil:
		return l.Raw
	case !l.OK:
		return l.Name + ": DENIED (" + l.Detail + ")"
	case l.Detail == "":
		return l.Name + ": OK"
	}
	return l.Name + ": OK " + l.Detail
}

// ValidName reports whether s is a legal check name.
func ValidName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case i > 0 && (c == '.' || c == '_' || c == '-'):
		default:
			return false
		}
	}
	return true
}

// ValidDetail reports whether s has no control characters (bytes below 0x20
// or 0x7f). Other bytes pass, UTF-8 or not, so a round trip is byte exact.
func ValidDetail(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return false
		}
	}
	return true
}

// Sanitize replaces every control byte with a space so any text, such as an
// error message with a newline in it, can ride in a detail.
func Sanitize(s string) string {
	if ValidDetail(s) {
		return s
	}
	b := []byte(s)
	for i, c := range b {
		if c < 0x20 || c == 0x7f {
			b[i] = ' '
		}
	}
	return string(b)
}

// parseLine parses one line without its newline.
func parseLine(raw string) Line {
	bad := func(format string, a ...any) Line {
		return Line{Raw: raw, Err: fmt.Errorf(format, a...)}
	}
	if raw == "" {
		return Line{Raw: raw, Err: errors.New("empty line")}
	}
	i := strings.Index(raw, ": ")
	if i < 0 {
		return bad("no \": \" separator")
	}
	name, rest := raw[:i], raw[i+2:]
	if !ValidName(name) {
		return bad("invalid name %q", name)
	}
	switch {
	case rest == "OK":
		return Line{Name: name, OK: true}
	case strings.HasPrefix(rest, "OK "):
		d := rest[len("OK "):]
		if d == "" {
			return bad("OK followed by a space but no detail")
		}
		if !ValidDetail(d) {
			return bad("control character in detail")
		}
		return Line{Name: name, OK: true, Detail: d}
	case strings.HasPrefix(rest, "DENIED (") && strings.HasSuffix(rest, ")"):
		d := rest[len("DENIED (") : len(rest)-1]
		if !ValidDetail(d) {
			return bad("control character in detail")
		}
		return Line{Name: name, Detail: d}
	}
	return bad("status is neither OK nor DENIED (<error>)")
}

// Parse reads probe output. Every line of s becomes one entry, in order, so
// nothing is dropped: a line that is not a probe line comes back with Err set
// and its text in Raw. A final line without a newline still counts (a killed
// probe may stop mid-line), and an empty line is malformed.
func Parse(s string) []Line {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	parts := strings.Split(s, "\n")
	lines := make([]Line, len(parts))
	for i, p := range parts {
		lines[i] = parseLine(p)
	}
	return lines
}

// Format is the inverse of Parse: each line, newline-terminated. Parsing and
// formatting any text gives the text back, with a newline added if the last
// line lacked one.
func Format(lines []Line) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// Find returns the first well-formed line with the given name.
func Find(lines []Line, name string) (Line, bool) {
	for _, l := range lines {
		if !l.Malformed() && l.Name == name {
			return l, true
		}
	}
	return Line{}, false
}

// Malformed returns the lines that did not parse.
func Malformed(lines []Line) []Line {
	var bad []Line
	for _, l := range lines {
		if l.Malformed() {
			bad = append(bad, l)
		}
	}
	return bad
}
