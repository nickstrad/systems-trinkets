package probeout

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// lineRE is the grammar written out again as a regular expression, so the
// property does not lean on the parser's own string handling. Groups: name,
// "OK" or "", OK detail, DENIED detail.
var lineRE = regexp.MustCompile(`^([a-z0-9][a-z0-9._-]*): (?:(OK)(?: ([^\x00-\x1f\x7f]+))?|DENIED \(([^\x00-\x1f\x7f]*)\))$`)

// checkParse is the property FuzzProbeOutput and TestProp_ProbeOutput assert
// for any text: one entry per line (none dropped), an entry is malformed
// exactly when the grammar rejects the line, well-formed entries carry the
// name, status and detail the grammar finds, a malformed one keeps its text,
// and formatting the entries gives the text back (plus a final newline when
// the text lacked one).
func checkParse(s string) error {
	lines := Parse(s)
	wantCount := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		wantCount++
	}
	if len(lines) != wantCount {
		return fmt.Errorf("%d entries for %d lines in %q", len(lines), wantCount, s)
	}
	text := strings.TrimSuffix(s, "\n")
	for i, l := range lines {
		raw := strings.Split(text, "\n")[i]
		m := lineRE.FindStringSubmatch(raw)
		switch {
		case m == nil && !l.Malformed():
			return fmt.Errorf("line %q parsed but the grammar rejects it: %+v", raw, l)
		case m != nil && l.Malformed():
			return fmt.Errorf("line %q is well formed but was reported malformed: %v", raw, l.Err)
		case m == nil:
			if l.Raw != raw {
				return fmt.Errorf("malformed line lost its text: %q != %q", l.Raw, raw)
			}
		default:
			wantOK := m[2] == "OK"
			wantDetail := m[3] + m[4]
			if l.Name != m[1] || l.OK != wantOK || l.Detail != wantDetail {
				return fmt.Errorf("line %q parsed as %+v", raw, l)
			}
		}
	}
	want := s
	if s != "" && !strings.HasSuffix(s, "\n") {
		want += "\n"
	}
	if got := Format(lines); got != want {
		return fmt.Errorf("Format(Parse(%q)) = %q, want %q", s, got, want)
	}
	return nil
}

func TestParseExamples(t *testing.T) {
	tests := []struct {
		in     string
		name   string
		ok     bool
		detail string
		bad    bool
	}{
		{in: "identity: OK", name: "identity", ok: true},
		{in: "cap-eff: OK 00000000a80425fb", name: "cap-eff", ok: true, detail: "00000000a80425fb"},
		{in: "write: DENIED (open /x: read-only file system)", name: "write", detail: "open /x: read-only file system"},
		{in: "dial: DENIED ()", name: "dial"},
		{in: "x: DENIED (a (b))", name: "x", detail: "a (b)"},
		{in: "memory.max: OK 67108864", name: "memory.max", ok: true, detail: "67108864"},
		{in: "", bad: true},
		{in: "no separator", bad: true},
		{in: "Upper: OK", bad: true},
		{in: ": OK", bad: true},
		{in: "-x: OK", bad: true},
		{in: "x: ok", bad: true},
		{in: "x: OK ", bad: true},
		{in: "x: DENIED", bad: true},
		{in: "x: DENIED (unterminated", bad: true},
		{in: "x: OK a\tb", bad: true},
		{in: "x: OK\r", bad: true},
	}
	for _, tc := range tests {
		lines := Parse(tc.in + "\n")
		if len(lines) != 1 {
			t.Errorf("%q: %d entries", tc.in, len(lines))
			continue
		}
		l := lines[0]
		if l.Malformed() != tc.bad {
			t.Errorf("%q: malformed=%v, want %v (%+v)", tc.in, l.Malformed(), tc.bad, l)
			continue
		}
		if !tc.bad && (l.Name != tc.name || l.OK != tc.ok || l.Detail != tc.detail) {
			t.Errorf("%q parsed as %+v", tc.in, l)
		}
		if l.String() != tc.in {
			t.Errorf("String() of %q = %q", tc.in, l.String())
		}
	}
}

func TestConstructorsSanitize(t *testing.T) {
	l := Denied("write", errors.New("open /x:\nread-only\tfile system\x00"))
	if !ValidDetail(l.Detail) {
		t.Errorf("detail %q still has control characters", l.Detail)
	}
	got := Parse(l.String() + "\n")
	if len(got) != 1 || got[0].Malformed() || got[0].Detail != l.Detail || got[0].OK {
		t.Errorf("round trip of %q gave %+v", l.String(), got)
	}
	if _, ok := Find(Parse("a: OK 1\nb: OK 2\nb: OK 3\n???\n"), "b"); !ok {
		t.Error("Find did not return b")
	}
	if n := len(Malformed(Parse("a: OK\n???\nb: OK\n\n"))); n != 2 {
		t.Errorf("Malformed found %d lines, want 2", n)
	}
}

func propProbeOutput(t *rapid.T) {
	name := rapid.StringMatching(`[a-z0-9][a-z0-9._-]{0,10}`)
	piece := rapid.OneOf(
		rapid.Map(rapid.Custom(func(t *rapid.T) [2]string {
			return [2]string{name.Draw(t, "name"), rapid.String().Draw(t, "detail")}
		}), func(p [2]string) string { return OK(p[0], p[1]).String() }),
		rapid.Map(rapid.Custom(func(t *rapid.T) [2]string {
			return [2]string{name.Draw(t, "name"), rapid.String().Draw(t, "error")}
		}), func(p [2]string) string { return Denied(p[0], errors.New(p[1])).String() }),
		rapid.String(),
		rapid.SampledFrom([]string{"", "x: OK ", "x: DENIED", "A: OK", "x: OK\r", "x: DENIED (a", "x:OK"}),
	)
	pieces := rapid.SliceOfN(piece, 0, 8).Draw(t, "lines")
	s := strings.Join(pieces, "\n")
	if rapid.Bool().Draw(t, "trailing newline") && len(pieces) > 0 {
		s += "\n"
	}
	if err := checkParse(s); err != nil {
		t.Fatal(err)
	}
}

func TestProp_ProbeOutput(t *testing.T) { rapid.Check(t, propProbeOutput) }

func FuzzProbeOutput(f *testing.F) {
	for _, s := range []string{
		"", "\n", "identity: OK uid=10001 gid=10001\n", "write: DENIED (read-only file system)\n",
		"a: OK\nb: DENIED (x)\n", "a: OK\nnot a probe line\n", "a: OK\n\nb: OK\n",
		"x: OK a\x00b\n", "x: DENIED (a)b)\n", "no newline: OK", "x: OK \n", "x: OK\r\n",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if err := checkParse(s); err != nil {
			t.Fatal(err)
		}
	})
}
