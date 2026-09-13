package suitekit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "counter-go-valkey.toml")
	os.WriteFile(path, []byte(`
pattern  = "counter"
language = "go"
engine   = "valkey"
url      = "http://127.0.0.1:8080"
label    = "v1 INCR"
[expect]
p99_ms = 20
`), 0o644)
	tg, err := LoadTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if tg.Pattern != "counter" || tg.Engine != "valkey" || tg.Expect["p99_ms"] != 20 {
		t.Errorf("%+v", tg)
	}

	bad := filepath.Join(dir, "bad.toml")
	os.WriteFile(bad, []byte(`pattern = "x"`+"\n"+`urll = "typo"`), 0o644)
	if _, err := LoadTarget(bad); err == nil {
		t.Error("want error for unknown key / missing url")
	}
}

func TestTargetEnvRoundTrip(t *testing.T) {
	t.Setenv(EnvTarget, "")
	for _, kv := range TargetEnv(Target{URL: "http://127.0.0.1:9999/", Pattern: "counter", Engine: "valkey"}) {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	tg, ok, err := TargetFromEnv()
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	want := Target{URL: "http://127.0.0.1:9999", Pattern: "counter", Language: "?", Engine: "valkey"}
	if !reflect.DeepEqual(tg, want) {
		t.Errorf("got %+v, want %+v", tg, want)
	}
	t.Setenv(EnvURL, "")
	if _, ok, _ := TargetFromEnv(); ok {
		t.Error("want ok=false with nothing set")
	}
}
