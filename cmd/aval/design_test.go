package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDesign(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "a.css")
	if err := os.WriteFile(css, []byte("a { margin: 13px; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	theme := filepath.Join(dir, "theme.css")
	if err := os.WriteFile(theme, []byte(":root{--x:1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	tests := []struct {
		name   string
		args   []string
		failed bool // errChecksFailed
		usage  bool // a usage error
		want   string
	}{
		{"contrast passes", []string{"gate", "contrast", "#767676", "#fff"}, false, false, "4.54:1"},
		{"contrast fails", []string{"gate", "contrast", "#777", "#fff"}, true, false, "FAIL"},
		{"contrast large flag", []string{"gate", "contrast", "#777", "#fff", "--large"}, false, false, "4.48:1"},
		{"contrast bad arity", []string{"gate", "contrast", "#777"}, false, true, ""},
		{"hardcodes finds a violation", []string{"gate", "hardcodes", css}, true, false, "hardcoded px '13px'"},
		{"hardcodes ext filter", []string{"gate", "hardcodes", "--ext", "tsx", css}, true, false, "no lintable"},
		{"theme-refs with --theme", []string{"gate", "theme-refs", "--theme", theme, css}, false, false, "OK"},
		{"theme-refs two-arg form", []string{"gate", "theme-refs", theme, css}, false, false, "OK"},
		{"native clean", []string{"gate", "native", theme}, false, false, "clean"},
		{"native no paths", []string{"gate", "native"}, false, true, ""},
		{"emoji clean", []string{"gate", "emoji", theme}, false, false, "OK"},
		{"unknown gate", []string{"gate", "nope"}, false, true, ""},
		{"no gate", []string{"gate"}, false, true, ""},
		{"unknown subcommand", []string{"zzz"}, false, true, ""},
		{"ack without surface", []string{"ack"}, false, true, ""},
		{"ack records", []string{"ack", "pricing"}, false, false, "design-ack: pricing recorded"},
	}
	for _, tc := range tests {
		var b bytes.Buffer
		err := runDesign(tc.args, dir, &b, os.Getenv)
		switch {
		case tc.failed && !errors.Is(err, errChecksFailed):
			t.Errorf("%s: err = %v, want errChecksFailed\n%s", tc.name, err, b.String())
		case tc.usage && !isUsage(err):
			t.Errorf("%s: err = %v, want a usage error", tc.name, err)
		case !tc.failed && !tc.usage && err != nil:
			t.Errorf("%s: unexpected err %v\n%s", tc.name, err, b.String())
		}
		if !strings.Contains(b.String(), tc.want) {
			t.Errorf("%s: output lacks %q:\n%s", tc.name, tc.want, b.String())
		}
	}
}
