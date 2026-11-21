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
		name    string
		args    []string
		failed  bool // errChecksFailed
		wantErr string
		want    string
	}{
		{"contrast passes", []string{"gate", "contrast", "#767676", "#fff"}, false, "", "4.54:1"},
		{"contrast fails", []string{"gate", "contrast", "#777", "#fff"}, true, "", "FAIL"},
		{"contrast large flag", []string{"gate", "contrast", "#777", "#fff", "--large"}, false, "", "4.48:1"},
		{"contrast bad arity", []string{"gate", "contrast", "#777"}, false, "usage", ""},
		{"hardcodes finds a violation", []string{"gate", "hardcodes", css}, true, "", "hardcoded px '13px'"},
		{"hardcodes ext filter", []string{"gate", "hardcodes", "--ext", "tsx", css}, true, "", "no lintable"},
		{"theme-refs with --theme", []string{"gate", "theme-refs", "--theme", theme, css}, false, "", "OK"},
		{"theme-refs two-arg form", []string{"gate", "theme-refs", theme, css}, false, "", "OK"},
		{"native clean", []string{"gate", "native", theme}, false, "", "clean"},
		{"native no paths", []string{"gate", "native"}, false, "usage", ""},
		{"emoji clean", []string{"gate", "emoji", theme}, false, "", "OK"},
		{"unknown gate", []string{"gate", "nope"}, false, "unknown gate", ""},
		{"no gate", []string{"gate"}, false, "usage", ""},
		{"unknown subcommand", []string{"zzz"}, false, "usage", ""},
		{"ack without surface", []string{"ack"}, false, "usage", ""},
		{"ack records", []string{"ack", "pricing"}, false, "", "design-ack: pricing recorded"},
	}
	for _, tc := range tests {
		var b bytes.Buffer
		err := runDesign(tc.args, dir, &b)
		switch {
		case tc.failed && !errors.Is(err, errChecksFailed):
			t.Errorf("%s: err = %v, want errChecksFailed\n%s", tc.name, err, b.String())
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.wantErr)
		case !tc.failed && tc.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected err %v\n%s", tc.name, err, b.String())
		}
		if !strings.Contains(b.String(), tc.want) {
			t.Errorf("%s: output lacks %q:\n%s", tc.name, tc.want, b.String())
		}
	}
}
