package docker

import (
	"archive/tar"
	"bytes"
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"

	"pgregory.net/rapid"
)

// ---- build stream parser -------------------------------------------------

// wantStreamFailure is a second, separate reading of the build-stream rules
// in parseBuildStream's doc comment. It splits on newlines itself and decodes
// each line into a generic value, where the parser streams into a map of raw
// messages, so the two share only encoding/json's definition of valid JSON.
// bad reports a line that is not a JSON object; failed reports an object with
// an errorDetail or error key.
func wantStreamFailure(data []byte) (bad, failed bool) {
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.Trim(line, " \t\r")
		if len(line) == 0 {
			continue
		}
		// json.Valid first, then a decode that keeps numbers as text:
		// decoding into float64 would reject valid JSON such as 1e400.
		if !json.Valid(line) {
			bad = true
			continue
		}
		var v any
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			bad = true
			continue
		}
		obj, ok := v.(map[string]any)
		if !ok {
			bad = true // a number, string, array, bool or null
			continue
		}
		if _, ok := obj["errorDetail"]; ok {
			failed = true
		}
		if _, ok := obj["error"]; ok {
			failed = true
		}
	}
	return bad, failed
}

// checkBuildStream is the property FuzzBuildStream and TestProp_BuildStream
// assert: the parser returns an error exactly when the stream holds a
// failure message or a line that is not a JSON object.
func checkBuildStream(data []byte) error {
	bad, failed := wantStreamFailure(data)
	err := parseBuildStream(bytes.NewReader(data))
	if want := bad || failed; (err != nil) != want {
		return fmt.Errorf("parseBuildStream(%q) = %v, want error=%v", data, err, want)
	}
	if failed && !strings.Contains(err.Error(), "build failed") {
		return fmt.Errorf("failure %q is not reported as a build failure", err)
	}
	return nil
}

func TestBuildStreamExamples(t *testing.T) {
	const progress = `{"stream":"Step 1/4 : FROM scratch\n"}` + "\n" + `{"aux":{"ID":"sha256:abc"}}` + "\n" +
		`{"stream":"error: this text is progress, not a failure\n"}` + "\n"
	tests := []struct {
		name, in string
		want     string // "" = success, else a substring of the error
	}{
		{"empty", "", ""},
		{"blank lines only", "\n  \n\t\r\n", ""},
		{"progress", progress, ""},
		{"no trailing newline", `{"stream":"x"}`, ""},
		{"crlf", "{\"stream\":\"x\"}\r\n{\"stream\":\"y\"}\r\n", ""},
		{"blank line between", `{"a":1}` + "\n\n" + `{"b":2}` + "\n", ""},
		{"empty object", "{}\n", ""},
		{"number beyond float64 is still JSON", `{"a":1e400}` + "\n", ""},
		{"errorDetail", progress + `{"errorDetail":{"code":1,"message":"unknown instruction: BOGUS"},"error":"unknown instruction: BOGUS"}` + "\n", "unknown instruction: BOGUS"},
		{"error only", `{"error":"pull access denied"}`, "pull access denied"},
		{"errorDetail only", `{"errorDetail":{"message":"boom"}}`, "boom"},
		{"empty error string still fails", `{"error":""}`, "build failed"},
		{"errorDetail null still fails", `{"errorDetail":null}`, "build failed"},
		{"error wrong type still fails", `{"error":5}`, "build failed"},
		{"failure then progress", `{"error":"x"}` + "\n" + progress, "x"},
		{"not json", "Step 1/2\n", "not a JSON object"},
		{"array", "[1]\n", "not a JSON object"},
		{"string", `"x"` + "\n", "not a JSON object"},
		{"number", "42\n", "not a JSON object"},
		{"null", "null\n", "not a JSON object"},
		{"two objects on a line", `{"a":1}{"b":2}` + "\n", "not a JSON object"},
		{"cut off", `{"stream":"x"`, "not a JSON object"},
		{"garbage after progress", progress + "oops\n", "line 4"},
		{"failure wins over garbage", `{"error":"real cause"}` + "\n" + "oops\n", "real cause"},
		{"failure wins over earlier garbage", "oops\n" + `{"error":"real cause"}` + "\n", "real cause"},
	}
	for _, tc := range tests {
		err := parseBuildStream(strings.NewReader(tc.in))
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: error %v, want one containing %q", tc.name, err, tc.want)
		}
		if err := checkBuildStream([]byte(tc.in)); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	if err := parseBuildStream(iotest.ErrReader(errors.New("connection reset"))); err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("a read error must surface, got %v", err)
	}
	if err := parseBuildStream(io.MultiReader(strings.NewReader(`{"a":1}`+"\n"), iotest.ErrReader(errors.New("cut")))); err == nil {
		t.Error("a stream cut by a read error must be an error even after good lines")
	}
}

// propBuildStream builds a stream from lines whose kind is known, so the
// expected answer comes from the construction and not from reading the
// stream back.
func propBuildStream(t *rapid.T) {
	good := rapid.Map(rapid.StringMatching(`[ -~]{0,20}`), func(s string) string {
		b, _ := json.Marshal(map[string]string{"stream": s})
		return string(b)
	})
	progress := rapid.SampledFrom([]string{`{"aux":{"ID":"sha256:abc"}}`, `{"status":"Pulling","progressDetail":{}}`, `{}`, `{"stream":"error: not a failure"}`})
	blank := rapid.SampledFrom([]string{"", " ", "\t", "\r", "  \t "})
	failure := rapid.Custom(func(t *rapid.T) string {
		j, _ := json.Marshal(rapid.StringMatching(`[ -~]{0,20}`).Draw(t, "message"))
		return rapid.SampledFrom([]string{
			`{"errorDetail":{"message":` + string(j) + `},"error":` + string(j) + `}`,
			`{"error":` + string(j) + `}`,
			`{"errorDetail":{"message":` + string(j) + `}}`,
			`{"error":null}`, `{"errorDetail":{}}`,
		}).Draw(t, "failure shape")
	})
	garbage := rapid.SampledFrom([]string{"oops", "[]", "[1,2]", `"s"`, "7", "null", "true", `{"a":`, `{"a":1}{"b":2}`, "{'a':1}", `{"stream":"x"} trailing`})
	type kind struct {
		line         string
		bad, failure bool
	}
	line := rapid.OneOf(
		rapid.Map(good, func(s string) kind { return kind{line: s} }),
		rapid.Map(progress, func(s string) kind { return kind{line: s} }),
		rapid.Map(blank, func(s string) kind { return kind{line: s} }),
		rapid.Map(failure, func(s string) kind { return kind{line: s, failure: true} }),
		rapid.Map(garbage, func(s string) kind { return kind{line: s, bad: true} }),
	)
	lines := rapid.SliceOfN(line, 0, 10).Draw(t, "lines")
	sep := rapid.SampledFrom([]string{"\n", "\r\n"}).Draw(t, "line end")
	var sb strings.Builder
	var want bool
	for i, l := range lines {
		sb.WriteString(l.line)
		if i < len(lines)-1 || rapid.Bool().Draw(t, "final newline") {
			sb.WriteString(sep)
		}
		want = want || l.bad || l.failure
	}
	err := parseBuildStream(strings.NewReader(sb.String()))
	if (err != nil) != want {
		t.Fatalf("stream %q: error %v, want error=%v", sb.String(), err, want)
	}
	if err := checkBuildStream([]byte(sb.String())); err != nil {
		t.Fatal(err)
	}
}

func TestProp_BuildStream(t *testing.T) { rapid.Check(t, propBuildStream) }

func FuzzBuildStream(f *testing.F) {
	for _, s := range []string{
		"", "\n", `{"stream":"Step 1/4 : FROM scratch\n"}` + "\n",
		`{"errorDetail":{"message":"x"},"error":"x"}` + "\n", `{"error":"x"}`, "oops\n", "null\n", "[]\n",
		`{"a":1}{"error":"x"}`, "{\"a\":1}\r\n\r\n{\"error\":1}\r\n", `{"a":`, "  \n\t{\"a\":1}  \n",
		`{"errorDetail":null}`, "\x00", "{\"a\":1}\n\xff\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if err := checkBuildStream(data); err != nil {
			t.Fatal(err)
		}
	})
}

// ---- validation, Dockerfile, build context -------------------------------

func TestValidateOwnedPath(t *testing.T) {
	good := []string{"/work", "/a/b/c", "/a.b", "/a_b", "/a@b", "/a+b", "/a-b", "/A9", "/.hidden", "/...", "/fixtures", "/x/fixture", "/a..b",
		"/.wh", "/.whx", "/x.wh.y", "/wh.", "/.Wh.x"}
	bad := []string{"", "/", "work", "./work", "/work/", "/work/../etc", "/work/./x", "//work", "/a//b", "/..", "/.",
		"/with space", `/quo"te`, "/quo'te", "/back\\slash", "/w$ork", "/x${y:-z}", "/a`b", "/a;b", "/a*b", "/a?b", "/a~b", "/a:b", "/a=b", "/a,b", "/a%b", "/a#b", "/a!b", "/a&b", "/a<b", "/é",
		"/wo\nrk", "/wo\x00rk", "/wo\trk", "/wo\x7frk", "/\xff", "/fixture", "/fixture/sub",
		"/.wh.work", "/a/.wh.b", "/a/.wh..wh..opq", "/.wh."}
	for _, p := range good {
		if err := validateOwnedPath(p); err != nil {
			t.Errorf("validateOwnedPath(%q) = %v, want ok", p, err)
		}
	}
	for _, p := range bad {
		if err := validateOwnedPath(p); err == nil {
			t.Errorf("validateOwnedPath(%q) accepted a bad path", p)
		}
	}
}

func TestFixtureImageValidation(t *testing.T) {
	base := FixtureImage{Lesson: "harness", Tag: "trinkets-harness-probe:dev", Package: "./probe"}
	with := func(f func(*FixtureImage)) FixtureImage { img := base; f(&img); return img }
	tests := []struct {
		name string
		img  FixtureImage
		ok   bool
	}{
		{"base", base, true},
		{"hyphenated lesson and name", FixtureImage{Lesson: "peer-broker", Tag: "trinkets-peer-broker-worker-2:dev", Package: "x/y"}, true},
		{"explicit user", with(func(i *FixtureImage) { i.User = "20000:30000" }), true},
		{"root user", with(func(i *FixtureImage) { i.User = "0:0" }), true},
		{"empty lesson", with(func(i *FixtureImage) { i.Lesson = "" }), false},
		{"upper-case lesson", with(func(i *FixtureImage) { i.Lesson = "Harness" }), false},
		{"tag for another lesson", with(func(i *FixtureImage) { i.Lesson = "other" }), false},
		{"tag without prefix", with(func(i *FixtureImage) { i.Tag = "probe:dev" }), false},
		{"tag with latest", with(func(i *FixtureImage) { i.Tag = "trinkets-harness-probe:latest" }), false},
		{"tag without version", with(func(i *FixtureImage) { i.Tag = "trinkets-harness-probe" }), false},
		{"tag with empty name", with(func(i *FixtureImage) { i.Tag = "trinkets-harness-:dev" }), false},
		{"tag with registry", with(func(i *FixtureImage) { i.Tag = "evil.example/trinkets-harness-probe:dev" }), false},
		{"tag with underscore name", with(func(i *FixtureImage) { i.Tag = "trinkets-harness-a_b:dev" }), false},
		{"empty package", with(func(i *FixtureImage) { i.Package = "" }), false},
		{"package that is a flag", with(func(i *FixtureImage) { i.Package = "-race" }), false},
		{"named user", with(func(i *FixtureImage) { i.User = "nobody" }), false},
		{"user without gid", with(func(i *FixtureImage) { i.User = "1000" }), false},
		{"user id out of range", with(func(i *FixtureImage) { i.User = "4294967295:1" }), false},
		{"negative user", with(func(i *FixtureImage) { i.User = "-1:1" }), false},
		{"owned dir", with(func(i *FixtureImage) { i.Dirs = []OwnedDir{{"/work", 10001, 10001}} }), true},
		{"owned dir with relative path", with(func(i *FixtureImage) { i.Dirs = []OwnedDir{{"work", 1, 1}} }), false},
		{"owned dir with negative uid", with(func(i *FixtureImage) { i.Dirs = []OwnedDir{{"/work", -1, 1}} }), false},
		{"owned dir with huge gid", with(func(i *FixtureImage) { i.Dirs = []OwnedDir{{"/work", 1, 4294967295}} }), false},
		{"duplicate owned dir", with(func(i *FixtureImage) { i.Dirs = []OwnedDir{{"/work", 1, 1}, {"/work", 2, 2}} }), false},
		{"nested owned dirs", with(func(i *FixtureImage) { i.Dirs = []OwnedDir{{"/a", 1, 1}, {"/a/b", 2, 2}} }), true},
	}
	for _, tc := range tests {
		err := tc.img.validate()
		if (err == nil) != tc.ok {
			t.Errorf("%s: validate() = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}

func TestFixtureDockerfile(t *testing.T) {
	img := FixtureImage{Lesson: "harness", Tag: "trinkets-harness-probe:dev", Package: "./probe",
		Dirs: []OwnedDir{{"/work/inner", 1, 1}, {"/work", 10001, 10001}, {"/run/a.b_c@d+e-f", 20000, 30000}}}
	want := `FROM scratch
ENV TRINKETS_PROBE=1
COPY ["fixture", "/fixture"]
COPY --chown=20000:30000 ["d/0", "/run/a.b_c@d+e-f"]
COPY --chown=10001:10001 ["d/1", "/work"]
COPY --chown=1:1 ["d/2", "/work/inner"]
USER 10001:10001
ENTRYPOINT ["/fixture"]
`
	if got := fixtureDockerfile(img); got != want {
		t.Errorf("Dockerfile:\n%s\nwant:\n%s", got, want)
	}
	if img.Dirs[0].Path != "/work/inner" {
		t.Error("generating the Dockerfile reordered the caller's Dirs")
	}
	img.User = "20000:30000"
	if got := fixtureDockerfile(img); !strings.Contains(got, "\nUSER 20000:30000\n") {
		t.Errorf("explicit user missing:\n%s", got)
	}
}

type tarEntry struct {
	Dir  bool
	Mode int64
	UID  int
	GID  int
	Data []byte
}

// readTar returns the entries by name, in order, with directory names as the
// archive spells them (trailing slash).
func readTar(data []byte) (names []string, entries map[string]tarEntry, err error) {
	entries = map[string]tarEntry{}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names, entries, nil
		}
		if err != nil {
			return nil, nil, err
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, nil, err
		}
		if _, dup := entries[h.Name]; dup {
			return nil, nil, fmt.Errorf("duplicate entry %q", h.Name)
		}
		names = append(names, h.Name)
		entries[h.Name] = tarEntry{Dir: h.Typeflag == tar.TypeDir, Mode: h.Mode, UID: h.Uid, GID: h.Gid, Data: body}
	}
}

// copyRE reads a generated COPY line back: optional --chown, then the
// exec-form JSON array [source, destination].
var copyRE = regexp.MustCompile(`^COPY (?:--chown=([0-9]+):([0-9]+) )?(\[.*\])$`)

// checkContext is the property for one generated image: the build context is
// produced exactly when the description is valid; every name in it is
// relative and clean; the tar reads back to the same Dockerfile, binary and
// directories; and the Dockerfile read back with its own small parser
// (JSON arrays, not the generator's formatting) holds exactly the requested
// destinations and owners, one instruction each, with nothing injected.
func checkContext(img FixtureImage, binary []byte, wantValid bool) error {
	tarBytes, err := fixtureContextTar(img, binary)
	if (err == nil) != wantValid {
		return fmt.Errorf("fixtureContextTar(%+v) error = %v, want valid=%v", img, err, wantValid)
	}
	if err != nil {
		return nil
	}
	names, entries, err := readTar(tarBytes)
	if err != nil {
		return err
	}
	wantNames := []string{"Dockerfile", "fixture"}
	if len(img.Dirs) > 0 {
		wantNames = append(wantNames, "d/")
		for i := range img.Dirs {
			wantNames = append(wantNames, "d/"+strconv.Itoa(i)+"/")
		}
	}
	if fmt.Sprint(names) != fmt.Sprint(wantNames) {
		return fmt.Errorf("tar entries %q, want %q", names, wantNames)
	}
	for _, n := range names {
		e := entries[n]
		clean := strings.TrimSuffix(n, "/")
		if e.Dir != strings.HasSuffix(n, "/") {
			return fmt.Errorf("%q: directory flag disagrees with the name", n)
		}
		if path.IsAbs(clean) || path.Clean(clean) != clean || clean == "." || clean == ".." ||
			strings.HasPrefix(clean, "../") || strings.ContainsAny(clean, "\\\x00") || !utf8.ValidString(clean) {
			return fmt.Errorf("tar name %q is not relative and clean", n)
		}
		if e.UID != 0 || e.GID != 0 {
			return fmt.Errorf("%q is owned by %d:%d in the context, want root", n, e.UID, e.GID)
		}
	}
	if !bytes.Equal(entries["fixture"].Data, binary) || entries["fixture"].Mode != 0o755 {
		return errors.New("the binary did not read back identically, executable")
	}
	lines := strings.Split(strings.TrimSuffix(string(entries["Dockerfile"].Data), "\n"), "\n")
	wantLines := 5 + len(img.Dirs)
	if len(lines) != wantLines || lines[0] != "FROM scratch" || lines[1] != "ENV "+probeMarker+"=1" ||
		lines[len(lines)-2] != "USER "+img.user() || lines[len(lines)-1] != `ENTRYPOINT ["/fixture"]` {
		return fmt.Errorf("Dockerfile has %d lines, want %d with fixed first, second and last lines:\n%s", len(lines), wantLines, entries["Dockerfile"].Data)
	}
	// Read the COPY lines back with a JSON decoder: the binary first, then
	// the owned directories, which must carry exactly the requested paths and
	// owners with every parent ahead of its children.
	var got []OwnedDir
	for i, line := range lines[2 : len(lines)-2] {
		m := copyRE.FindStringSubmatch(line)
		if m == nil {
			return fmt.Errorf("line %q is not a COPY instruction", line)
		}
		var args []string
		if err := json.Unmarshal([]byte(m[3]), &args); err != nil || len(args) != 2 {
			return fmt.Errorf("line %q: arguments %q: %v", line, m[3], err)
		}
		if i == 0 {
			if m[1] != "" || args[0] != "fixture" || args[1] != "/fixture" {
				return fmt.Errorf("binary copy line %q", line)
			}
			continue
		}
		if args[0] != "d/"+strconv.Itoa(i-1) {
			return fmt.Errorf("line %q: source %q, want d/%d", line, args[0], i-1)
		}
		uid, _ := strconv.Atoi(m[1])
		gid, _ := strconv.Atoi(m[2])
		got = append(got, OwnedDir{args[1], uid, gid})
	}
	want := map[OwnedDir]int{}
	for _, d := range img.Dirs {
		want[d]++
	}
	for i, d := range got {
		if want[d]--; want[d] < 0 {
			return fmt.Errorf("Dockerfile carries %+v, which was not requested", d)
		}
		for _, earlier := range got[:i] {
			if strings.HasPrefix(earlier.Path, d.Path+"/") {
				return fmt.Errorf("%q is copied after its child %q: COPY --chown would leave it root-owned", d.Path, earlier.Path)
			}
		}
	}
	if len(got) != len(img.Dirs) {
		return fmt.Errorf("Dockerfile has %d directories, want %d", len(got), len(img.Dirs))
	}
	return nil
}

// genPath draws an owned-directory path and says whether it is valid. A valid
// path is built from allow-listed characters in segments that cannot be "."
// or "..". A hostile one is a
// valid one broken in a way that is wrong by construction.
func genPath(t *rapid.T, label string) (string, bool) {
	segment := rapid.StringMatching(`[a-zA-Z0-9_@+-][a-zA-Z0-9_@+.-]{0,6}`)
	segs := rapid.SliceOfN(segment, 1, 4).Draw(t, label+" segments")
	p := "/" + strings.Join(segs, "/")
	if p == fixtureBinary || strings.HasPrefix(p, fixtureBinary+"/") {
		p = "/x" + p
	}
	switch rapid.SampledFrom([]string{"", "", "", "", "", "", "", "", "", "", "slash", "relative", "double", "dot", "dotdot", "newline", "nul", "tab", "del", "utf8", "root", "empty", "fixture", "fixture-sub", "unsafe", "unsafe", "whiteout"}).Draw(t, label+" breakage") {
	case "":
		return p, true
	case "slash":
		return p + "/", false
	case "relative":
		return p[1:], false
	case "double":
		return "/" + p, false
	case "dot":
		return p + "/.", false
	case "dotdot":
		return p + "/../x", false
	case "newline":
		return p + "\nCOPY evil /evil", false
	case "nul":
		return p + "\x00", false
	case "tab":
		return p + "\t", false
	case "del":
		return p + "\x7f", false
	case "utf8":
		return p + "\xff", false
	case "root":
		return "/", false
	case "empty":
		return "", false
	case "fixture":
		return "/fixture", false
	case "whiteout":
		// a layer whiteout marker as one segment
		segs[rapid.IntRange(0, len(segs)-1).Draw(t, label+" whiteout segment")] = ".wh." + segs[0]
		return "/" + strings.Join(segs, "/"), false
	case "unsafe":
		// characters the builder expands or mishandles, or outside the allow-list
		return p + rapid.SampledFrom([]string{"$x", "${y:-z}", "\\", `"`, "'", " ", "`", ";", "*", "?", "~", ":", "é", "<", "&"}).Draw(t, label+" unsafe"), false
	}
	return "/fixture/sub", false
}

func genID(t *rapid.T, label string) (int, bool) {
	if rapid.IntRange(0, 9).Draw(t, label+" breakage") == 0 {
		return rapid.SampledFrom([]int{-1, 4294967295, 1 << 40}).Draw(t, label+" bad"), false
	}
	return rapid.SampledFrom([]int{0, 1, 999, 10001, 20000, 4294967294}).Draw(t, label), true
}

// genImage draws a description and whether it is valid, from known-good
// parts and known-bad mutations of them.
func genImage(t *rapid.T) (FixtureImage, bool) {
	valid := true
	lesson := rapid.StringMatching(`[a-z0-9]{1,6}(-[a-z0-9]{1,6}){0,2}`).Draw(t, "lesson")
	name := rapid.StringMatching(`[a-z0-9]{1,6}(-[a-z0-9]{1,6}){0,2}`).Draw(t, "name")
	if rapid.IntRange(0, 11).Draw(t, "lesson breakage") == 0 {
		lesson = rapid.SampledFrom([]string{"", "A", "a b", "a_b", "-a", "a-", "a/b", "a:b", "a--b"}).Draw(t, "bad lesson")
		valid = false
	}
	tag := "trinkets-" + lesson + "-" + name + ":dev"
	if rapid.IntRange(0, 11).Draw(t, "tag breakage") == 0 {
		tag = rapid.SampledFrom([]string{
			"trinkets-" + lesson + "-" + name, "trinkets-" + lesson + "-" + name + ":latest",
			"trinkets-" + lesson + "x-" + name + ":dev", lesson + "-" + name + ":dev",
			"trinkets-" + lesson + "-:dev", "trinkets-" + lesson + "-" + name + "_x:dev",
			"r.example/trinkets-" + lesson + "-" + name + ":dev", "TRINKETS-" + lesson + "-" + name + ":dev",
		}).Draw(t, "bad tag")
		valid = false
	}
	pkg := "./probe"
	if rapid.IntRange(0, 14).Draw(t, "package breakage") == 0 {
		pkg = rapid.SampledFrom([]string{"", "-race", "-o=/tmp/x"}).Draw(t, "bad package")
		valid = false
	}
	user := rapid.SampledFrom([]string{"", "", "0:0", "10001:10001", "20000:30000", "4294967294:4294967294"}).Draw(t, "user")
	if rapid.IntRange(0, 14).Draw(t, "user breakage") == 0 {
		user = rapid.SampledFrom([]string{"root", "1", "1:", ":1", "-1:1", "1:-1", "1:2:3", "a:b", "1: 2", "+1:1", "0x1:1", "4294967295:1", "1:4294967295", "99999999999999999999:1", "1:1\nUSER 0"}).Draw(t, "bad user")
		valid = false
	}
	img := FixtureImage{Lesson: lesson, Tag: tag, Package: pkg, User: user}
	n := rapid.IntRange(0, 4).Draw(t, "dirs")
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		p, pv := genPath(t, fmt.Sprintf("path %d", i))
		// Often nest on an earlier valid path (a child, or its parent), so the
		// order in which parents and children are copied gets exercised.
		if i > 0 && rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("nest %d", i)) == 0 {
			if prev := img.Dirs[rapid.IntRange(0, i-1).Draw(t, fmt.Sprintf("nest target %d", i))].Path; validOwnedPathForTest(prev) {
				if parent := path.Dir(prev); parent != "/" && rapid.Bool().Draw(t, fmt.Sprintf("nest up %d", i)) {
					p, pv = parent, true
				} else {
					p, pv = prev+"/child", true
				}
			}
		}
		uid, uv := genID(t, fmt.Sprintf("uid %d", i))
		gid, gv := genID(t, fmt.Sprintf("gid %d", i))
		if i > 0 && rapid.IntRange(0, 5).Draw(t, fmt.Sprintf("dup %d", i)) == 0 {
			p, pv = img.Dirs[0].Path, true // true only so the duplicate itself is what makes it invalid
		}
		valid = valid && pv && uv && gv && !seen[p]
		seen[p] = true
		img.Dirs = append(img.Dirs, OwnedDir{p, uid, gid})
	}
	return img, valid
}

func propContextTar(t *rapid.T) {
	img, valid := genImage(t)
	binary := rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(t, "binary")
	if err := checkContext(img, binary, valid); err != nil {
		t.Fatal(err)
	}
}

func TestProp_ContextTar(t *testing.T) { rapid.Check(t, propContextTar) }
func FuzzContextTar(f *testing.F)      { f.Fuzz(rapid.MakeFuzz(propContextTar)) }

func TestContextNamesCleanInvariant(t *testing.T) {
	ok := []contextFile{{Name: "Dockerfile"}, {Name: "d", Dir: true}, {Name: "d/0", Dir: true}}
	if bad := contextNamesClean(ok); len(bad) != 0 {
		t.Errorf("violations on a clean context: %v", bad)
	}
	for _, name := range []string{"", ".", "..", "../x", "/abs", "a/", "a//b", "a/./b", "a/../b", "a\\b", "a\x00b", "a\xff"} {
		files := []contextFile{{Name: "Dockerfile"}, {Name: name}}
		if bad := contextNamesClean(files); len(bad) == 0 {
			t.Errorf("name %q passed the invariant", name)
		}
		if err := writeTar(io.Discard, files); err == nil {
			t.Errorf("writeTar accepted the name %q", name)
		}
	}
	if bad := contextNamesClean([]contextFile{{Name: "a"}, {Name: "a"}}); len(bad) == 0 {
		t.Error("a duplicate name passed the invariant")
	}
}

// ---- cross-compiling the fixture -----------------------------------------

func TestBuildBinaryIsStaticLinuxELF(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	for arch, machine := range map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64} {
		t.Run(arch, func(t *testing.T) {
			bin, err := buildBinary(t.Context(), "./probe", arch)
			if err != nil {
				t.Fatal(err)
			}
			f, err := elf.NewFile(bytes.NewReader(bin))
			if err != nil {
				t.Fatalf("not an ELF file: %v", err)
			}
			defer f.Close()
			if f.Machine != machine || f.OSABI != elf.ELFOSABI_NONE {
				t.Errorf("machine %v abi %v, want %v", f.Machine, f.OSABI, machine)
			}
			if libs, _ := f.ImportedLibraries(); len(libs) != 0 {
				t.Errorf("dynamic libraries %v: the binary must be static", libs)
			}
			if f.Section(".interp") != nil {
				t.Error("has an interpreter: the binary must be static")
			}
			if f.Section(".symtab") != nil {
				t.Error("symbols not stripped (-ldflags -s -w)")
			}
		})
	}
}

func TestBuildBinaryReportsCompilerOutput(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	_, err := buildBinary(t.Context(), "./no-such-package", "amd64")
	if err == nil || !strings.Contains(err.Error(), "no-such-package") {
		t.Errorf("error %v should carry go build's output", err)
	}
}

// ---- Done-when: GOOS=darwin go vet ---------------------------------------

func TestSpec_H2_DarwinVetPasses(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	if testing.Short() {
		t.Skip("vets the whole module")
	}
	cmd := exec.Command("go", "vet", "./...")
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	cmd.Dir = moduleRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("GOOS=darwin go vet ./...: %v\n%s", err, out)
	}
}

// moduleRoot is the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		t.Fatal("not inside a module")
	}
	return gomod[:len(gomod)-len("/go.mod")]
}

// validOwnedPathForTest says whether a generated path is one genPath made
// valid, without calling the implementation: valid paths start with "/" and use
// only allow-listed characters, with no empty, "." or ".." segment, no
// segment starting with ".wh." and no "fixture" first segment.
func validOwnedPathForTest(p string) bool {
	if !strings.HasPrefix(p, "/") || p == "/" || strings.HasSuffix(p, "/") {
		return false
	}
	for _, seg := range strings.Split(p[1:], "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".wh.") || strings.Trim(seg, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._@+-") != "" {
			return false
		}
	}
	return p != "/fixture" && !strings.HasPrefix(p, "/fixture/")
}
