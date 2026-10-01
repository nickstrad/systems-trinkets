package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

// Labels every harness-created object carries, so cleanup never touches the
// backing services or anyone else's containers.
const (
	LabelHarness = "trinkets.harness"
	LabelLesson  = "trinkets.lesson"
)

// Labels returns the labels for a harness object belonging to a lesson.
func Labels(lesson string) map[string]string {
	return map[string]string{LabelHarness: "1", LabelLesson: lesson}
}

// DefaultFixtureUser is the image USER when FixtureImage.User is empty: the
// numeric non-root identity the restricted baseline runs as. USER must be
// numeric because the image has no /etc/passwd to resolve a name against.
const DefaultFixtureUser = "10001:10001"

// fixtureBinary is where the one binary lives in the image, and the
// ENTRYPOINT. Owned directories may not sit on or under it.
const fixtureBinary = "/fixture"

// OwnedDir is a directory baked into the image and owned by a numeric
// identity. A named volume mounted there copies that ownership, so a non-root
// worker can write the fresh volume (see knowledge/docker-harness.md). Path
// must be absolute and already clean, and is limited to the characters
// [A-Za-z0-9._@+-] and "/": the builder expands "$" and "\" inside COPY
// arguments even in JSON form and does not accept a quote, so those would
// land at a different path. A path is rejected, never tidied, so a typo such
// as /work/../etc fails loudly instead of becoming a different path. The
// order of Dirs does not matter: parents are created before their children,
// because COPY --chown only sets the owner of a destination it creates.
// Missing parent directories that are not listed are created root-owned.
type OwnedDir struct {
	Path     string
	UID, GID int
}

// FixtureImage describes one fixture: a Go main package built into a
// FROM scratch image holding that single binary and the owned directories.
type FixtureImage struct {
	// Lesson names the lesson, for the trinkets.lesson label and the tag.
	Lesson string
	// Tag must be trinkets-<Lesson>-<name>:dev, with <name> lower-case
	// letters, digits and hyphens.
	Tag string
	// Package is what go build is given: an import path, or a path relative
	// to the working directory such as ./probe.
	Package string
	// User is the numeric "uid:gid" the image runs as; "" means
	// DefaultFixtureUser. A launcher spec can still override it per container.
	User string
	// Dirs are the owned directories, created in this order.
	Dirs []OwnedDir
}

var (
	lessonRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	nameRE   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	userRE   = regexp.MustCompile(`^[0-9]+:[0-9]+$`)
)

// maxID is the largest valid uid or gid: 4294967295 is (uid_t)-1.
const maxID = 4294967294

func validID(n int64) bool { return n >= 0 && n <= maxID }

// user is the image USER with the default applied.
func (img FixtureImage) user() string {
	if img.User == "" {
		return DefaultFixtureUser
	}
	return img.User
}

// ownedPathChar reports whether c is in the alphabet an owned-directory path may use, besides
// "/". Space is out too: the builder reads the COPY argument in more than one
// way, and every character that any of them treats specially was a real bug
// ("$", a backslash, a quote), so the rule is an allow-list.
func ownedPathChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("/._@+-", c) >= 0
}

// validateOwnedPath is strict on purpose: the path goes into a Dockerfile
// COPY destination, so anything outside the allow-list, and anything that
// path.Clean would change, is an error.
func validateOwnedPath(p string) error {
	switch {
	case p == "":
		return errors.New("empty path")
	case p[0] != '/':
		return errors.New("path must be absolute")
	case p == "/":
		return errors.New("the root directory cannot be an owned directory")
	}
	for i := 0; i < len(p); i++ {
		if !ownedPathChar(p[i]) {
			return fmt.Errorf("character %q is not allowed (use letters, digits and ._@+-)", p[i])
		}
	}
	switch {
	case path.Clean(p) != p:
		return fmt.Errorf("path is not clean (want %q)", path.Clean(p))
	case p == fixtureBinary || strings.HasPrefix(p, fixtureBinary+"/"):
		return fmt.Errorf("path collides with the fixture binary %s", fixtureBinary)
	}
	return nil
}

// validate checks everything that can be checked without a daemon or a
// compiler, so a bad description fails before any work starts.
func (img FixtureImage) validate() error {
	if !lessonRE.MatchString(img.Lesson) {
		return fmt.Errorf("docker: fixture lesson %q must match %s", img.Lesson, lessonRE)
	}
	rest, hasPrefix := strings.CutPrefix(img.Tag, "trinkets-"+img.Lesson+"-")
	name, hasSuffix := strings.CutSuffix(rest, ":dev")
	if !hasPrefix || !hasSuffix || !nameRE.MatchString(name) {
		return fmt.Errorf("docker: fixture tag %q must be trinkets-%s-<name>:dev with <name> matching %s",
			img.Tag, img.Lesson, nameRE)
	}
	if img.Package == "" || strings.HasPrefix(img.Package, "-") {
		return fmt.Errorf("docker: fixture package %q must be a package path, not empty or a flag", img.Package)
	}
	u := img.user()
	if !userRE.MatchString(u) {
		return fmt.Errorf("docker: fixture user %q must be numeric uid:gid (the image has no passwd file)", u)
	}
	uid, gid, _ := strings.Cut(u, ":")
	for _, id := range []string{uid, gid} {
		if n, err := strconv.ParseInt(id, 10, 64); err != nil || !validID(n) {
			return fmt.Errorf("docker: fixture user %q has an id out of range", u)
		}
	}
	seen := map[string]bool{}
	for _, d := range img.Dirs {
		if err := validateOwnedPath(d.Path); err != nil {
			return fmt.Errorf("docker: owned dir %q: %w", d.Path, err)
		}
		if !validID(int64(d.UID)) || !validID(int64(d.GID)) {
			return fmt.Errorf("docker: owned dir %q has uid:gid %d:%d out of range", d.Path, d.UID, d.GID)
		}
		if seen[d.Path] {
			return fmt.Errorf("docker: owned dir %q listed twice", d.Path)
		}
		seen[d.Path] = true
	}
	return nil
}

// jsonString quotes s for a Dockerfile exec-form array. Escaping of < > & is
// off because the Dockerfile parser reads plain JSON either way and the
// unescaped text is easier to read in an error message.
func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a valid-UTF-8 string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}

// ownedSrc is the build-context name of the i-th owned directory. Context
// names are generated, never taken from a user path, so no path can climb out
// of the context.
func ownedSrc(i int) string { return "d/" + strconv.Itoa(i) }

// sortedDirs returns the owned directories with every parent before its
// children. Sorting by path does it: an ancestor is a strict prefix of its
// descendants, so it sorts first. The order matters because COPY --chown does
// not chown a destination that already exists; if /a/b were copied first, /a
// would be created root-owned and a later COPY of /a would leave it so.
func (img FixtureImage) sortedDirs() []OwnedDir {
	dirs := slices.Clone(img.Dirs)
	slices.SortFunc(dirs, func(a, b OwnedDir) int { return strings.Compare(a.Path, b.Path) })
	return dirs
}

// probeMarker is the environment variable every fixture image sets. The probe
// refuses commands that change the machine unless it is present, so running
// the binary by hand on a host cannot mount, chown or fork there.
const probeMarker = "TRINKETS_PROBE"

// fixtureDockerfile generates the Dockerfile. Exec-form arrays keep each path
// in its own argument; the path alphabet (see OwnedDir) keeps the builder from
// expanding it. The binary's mode comes from the tar header (COPY --chmod is
// not available to every builder).
func fixtureDockerfile(img FixtureImage) string {
	var b strings.Builder
	b.WriteString("FROM scratch\n")
	fmt.Fprintf(&b, "ENV %s=1\n", probeMarker)
	fmt.Fprintf(&b, "COPY [%s, %s]\n", jsonString("fixture"), jsonString(fixtureBinary))
	for i, d := range img.sortedDirs() {
		fmt.Fprintf(&b, "COPY --chown=%d:%d [%s, %s]\n", d.UID, d.GID, jsonString(ownedSrc(i)), jsonString(d.Path))
	}
	fmt.Fprintf(&b, "USER %s\n", img.user())
	fmt.Fprintf(&b, "ENTRYPOINT [%s]\n", jsonString(fixtureBinary))
	return b.String()
}

// contextFile is one entry of the build-context tar.
type contextFile struct {
	Name string // relative and clean, no trailing slash
	Dir  bool
	Mode int64
	Data []byte
}

// fixtureContext lists the files of the build context: Dockerfile, the
// binary, and one empty directory per owned directory.
func fixtureContext(img FixtureImage, binary []byte) ([]contextFile, error) {
	if err := img.validate(); err != nil {
		return nil, err
	}
	files := []contextFile{
		{Name: "Dockerfile", Mode: 0o644, Data: []byte(fixtureDockerfile(img))},
		{Name: "fixture", Mode: 0o755, Data: binary},
	}
	if len(img.Dirs) > 0 {
		files = append(files, contextFile{Name: "d", Dir: true, Mode: 0o755})
	}
	for i := range img.Dirs {
		files = append(files, contextFile{Name: ownedSrc(i), Dir: true, Mode: 0o755})
	}
	return files, nil
}

// writeTar writes files as a tar archive owned by root with a fixed
// timestamp, so identical input gives identical layers and the builder cache
// can hit. It enforces InvContextNamesClean first.
func writeTar(w io.Writer, files []contextFile) error {
	if bad := contextNamesClean(files); len(bad) > 0 {
		return fmt.Errorf("docker: unsafe build context: %s", strings.Join(bad, "; "))
	}
	tw := tar.NewWriter(w)
	for _, f := range files {
		h := &tar.Header{Name: f.Name, Mode: f.Mode, ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
		if f.Dir {
			h.Typeflag, h.Name = tar.TypeDir, f.Name+"/"
		} else {
			h.Typeflag, h.Size = tar.TypeReg, int64(len(f.Data))
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if _, err := tw.Write(f.Data); err != nil {
			return err
		}
	}
	return tw.Close()
}

// fixtureContextTar is the whole build context as an in-memory tar.
func fixtureContextTar(img FixtureImage, binary []byte) ([]byte, error) {
	files, err := fixtureContext(img, binary)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeTar(&buf, files); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildBinary compiles pkg into a static Linux executable for goarch and
// returns its bytes. -buildvcs=false avoids "error obtaining VCS status"
// outside a clean checkout; the flags make the binary small and reproducible.
func buildBinary(ctx context.Context, pkg, goarch string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "trinkets-fixture-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "fixture")
	cmd := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-trimpath", "-ldflags", "-s -w", "-o", out, pkg)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goarch)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker: go build %s for linux/%s: %w\n%s", pkg, goarch, err, output)
	}
	return os.ReadFile(out)
}

// BuildFixture compiles img.Package for the engine's architecture and builds
// the image tagged img.Tag, labelled for the harness. It rebuilds every call
// (about 3 s, and the layer cache makes an unchanged one faster), so a
// fixture is never stale. It uses the classic builder through ImageBuild; if
// that path ever fails on an engine, the fallback is writing the same context
// to a directory and running docker build, not a different builder here.
// Like EngineArch, it panics when the engine's architecture is unknown;
// every other failure, including a build the daemon reports as failed, is a
// returned error.
func BuildFixture(ctx context.Context, cli *client.Client, img FixtureImage) error {
	if err := img.validate(); err != nil {
		return err
	}
	binary, err := buildBinary(ctx, img.Package, EngineArch(ctx, cli))
	if err != nil {
		return err
	}
	tarCtx, err := fixtureContextTar(img, binary)
	if err != nil {
		return err
	}
	return buildContext(ctx, cli, bytes.NewReader(tarCtx), img.Tag, img.Lesson)
}

// buildContext sends a build-context tar to the daemon, tags and labels the
// result, reads the progress stream to its end, and turns a reported failure
// into an error. It then asks the daemon for the tag, because a stream cut
// short can look like a clean one. Tests call it with a hand-made tar to
// exercise a broken Dockerfile.
func buildContext(ctx context.Context, cli *client.Client, tarCtx io.Reader, tag, lesson string) error {
	res, err := cli.ImageBuild(ctx, tarCtx, client.ImageBuildOptions{
		Tags:        []string{tag},
		Dockerfile:  "Dockerfile",
		Labels:      Labels(lesson),
		Remove:      true,
		ForceRemove: true,
	})
	if err != nil {
		return fmt.Errorf("docker: build %s: %w", tag, err)
	}
	defer res.Body.Close()
	if err := parseBuildStream(res.Body); err != nil {
		return fmt.Errorf("docker: build %s: %w", tag, err)
	}
	if _, err := cli.ImageInspect(ctx, tag); err != nil {
		return fmt.Errorf("docker: build %s reported no error but the image is missing: %w", tag, err)
	}
	return nil
}

// parseBuildStream reads the daemon's build output and returns an error
// exactly when it reports a failure or is not well formed.
//
// The format is JSON lines: every line that is not blank holds one JSON
// object. Blank means only spaces, tabs and carriage returns, so a trailing
// newline, an empty stream or CRLF line ends are fine. A line that is not a
// JSON object (text, an array, a number, null, two objects on one line, a
// cut-off object) makes the stream malformed. A well-formed object reports a
// failure when it has an "errorDetail" or an "error" key, whatever the value:
// a stream that mentions an error is not a success. Every other object
// ("stream", "aux", "status") is progress and ignored.
//
// It always reads to the end, so the daemon is never cut off mid-build and a
// failure after a malformed line is still found. When the stream holds both,
// the failure is returned, because it says what went wrong.
func parseBuildStream(r io.Reader) error {
	br := bufio.NewReader(r)
	var failure, malformed error
	for n := 1; ; n++ {
		line, readErr := br.ReadBytes('\n')
		if text := bytes.Trim(line, " \t\r\n"); len(text) > 0 {
			var obj map[string]json.RawMessage
			err := json.Unmarshal(text, &obj)
			switch {
			case err == nil && obj == nil:
				err = errors.New("null")
				fallthrough
			case err != nil:
				if malformed == nil {
					malformed = fmt.Errorf("build output line %d is not a JSON object: %w", n, err)
				}
			case failure == nil:
				failure = buildFailure(obj, text)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if malformed == nil {
				malformed = fmt.Errorf("reading build output: %w", readErr)
			}
			break
		}
	}
	if failure != nil {
		return failure
	}
	return malformed
}

// buildFailure is nil for a progress object and an error for one that
// reports a failure. The message is errorDetail.message, else the error
// string, else the line itself.
func buildFailure(obj map[string]json.RawMessage, text []byte) error {
	_, hasDetail := obj["errorDetail"]
	_, hasError := obj["error"]
	if !hasDetail && !hasError {
		return nil
	}
	var detail struct{ Message string }
	if json.Unmarshal(obj["errorDetail"], &detail) == nil && detail.Message != "" {
		return fmt.Errorf("build failed: %s", detail.Message)
	}
	var msg string
	if json.Unmarshal(obj["error"], &msg) == nil && msg != "" {
		return fmt.Errorf("build failed: %s", msg)
	}
	return fmt.Errorf("build failed: %s", text)
}
