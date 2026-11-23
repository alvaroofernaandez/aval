package doctor

import (
	"path/filepath"
	"strings"
	"testing"
)

func configResult(t *testing.T, path string) Result {
	t.Helper()
	rs := RunOnly(Options{Home: t.TempDir(), Repo: t.TempDir(), GOOS: "linux", ConfigPath: path}, "config")
	if len(rs) != 1 {
		t.Fatalf("want one config result, got %+v", rs)
	}
	return rs[0]
}

func TestConfigCheckMissingFileIsOKDefaults(t *testing.T) {
	r := configResult(t, filepath.Join(t.TempDir(), "absent"))
	if r.Status != OK || !strings.Contains(r.Message, "defaults") {
		t.Errorf("got %+v", r)
	}
}

func TestConfigCheckValidFileIsOK(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	w(t, p, "theme = nord\nmotion = off\n")
	if r := configResult(t, p); r.Status != OK {
		t.Errorf("got %+v", r)
	}
}

func TestConfigCheckWarnsOnBadValueAndUnknownKeyWithLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	w(t, p, "# aval\ntheme = tokio-night\ncolour = red\n")
	r := configResult(t, p)
	if r.Status != Warn {
		t.Fatalf("status = %s, want warn: %+v", r.Status, r)
	}
	for _, want := range []string{p + ":2", "tokyo-night", p + ":3", "colour"} {
		if !strings.Contains(r.Message, want) {
			t.Errorf("message lacks %q: %s", want, r.Message)
		}
	}
}

func TestConfigCheckIsOptIn(t *testing.T) {
	for _, r := range Run(Options{Home: t.TempDir(), Repo: t.TempDir(), GOOS: "linux"}) {
		if r.ID == "config" {
			t.Fatal("config check ran without a ConfigPath")
		}
	}
}
