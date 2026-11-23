package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cfgEnv struct {
	home, path     string
	env            map[string]string
	stdout, stderr bytes.Buffer
}

func newCfgEnv(t *testing.T) *cfgEnv {
	t.Helper()
	home := t.TempDir()
	return &cfgEnv{
		home: home,
		path: filepath.Join(home, ".config", "aval", "config"),
		env:  map[string]string{},
	}
}

func (e *cfgEnv) run(args ...string) error {
	e.stdout.Reset()
	e.stderr.Reset()
	return runConfig(args, &e.stdout, &e.stderr, func(k string) string { return e.env[k] }, e.home)
}

func (e *cfgEnv) write(t *testing.T, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(e.path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigListShowsValueAndSource(t *testing.T) {
	e := newCfgEnv(t)
	e.write(t, "theme = catppuccin\n")
	e.env["AVAL_MOTION"] = "off"
	if err := e.run("list"); err != nil {
		t.Fatal(err)
	}
	out := e.stdout.String()
	lines := map[string][]string{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 {
			lines[f[0]] = f
		}
	}
	if got := strings.Join(lines["theme"], " "); got != "theme catppuccin file:1" {
		t.Errorf("theme line = %q\n%s", got, out)
	}
	if got := strings.Join(lines["motion"], " "); got != "motion off env" {
		t.Errorf("motion line = %q\n%s", got, out)
	}
	if got := strings.Join(lines["glyphs"], " "); !strings.HasSuffix(got, " default") {
		t.Errorf("glyphs line = %q\n%s", got, out)
	}
}

func TestConfigListAlignsColumnsAndReportsUnknown(t *testing.T) {
	e := newCfgEnv(t)
	e.write(t, "theme = aval\nfoo = 1\n")
	if err := e.run(); err != nil { // bare config behaves like list
		t.Fatal(err)
	}
	out := e.stdout.String()
	if !strings.Contains(out, `unknown key "foo" (file:2)`) {
		t.Errorf("unknown key missing:\n%s", out)
	}
	// source column starts at the same offset on every key row
	col := -1
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "unknown") || strings.TrimSpace(l) == "" {
			continue
		}
		f := strings.Fields(l)
		i := strings.LastIndex(l, f[len(f)-1])
		if col == -1 {
			col = i
		} else if i != col {
			t.Errorf("misaligned row %q (col %d, want %d)", l, i, col)
		}
	}
}

func TestConfigListWarningsGoToStderr(t *testing.T) {
	e := newCfgEnv(t)
	e.write(t, "theme = nope\n")
	if err := e.run("list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.stderr.String(), "theme") {
		t.Errorf("stderr = %q, want a warning", e.stderr.String())
	}
	if strings.Contains(e.stdout.String(), "config:") {
		t.Errorf("warning leaked to stdout:\n%s", e.stdout.String())
	}
}

func TestConfigGetPrintsEffectiveValueOnly(t *testing.T) {
	e := newCfgEnv(t)
	e.write(t, "theme = nord\n")
	if err := e.run("get", "theme"); err != nil {
		t.Fatal(err)
	}
	if got := e.stdout.String(); got != "nord\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestConfigGetUnknownKeyFails(t *testing.T) {
	e := newCfgEnv(t)
	err := e.run("get", "bogus")
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{`"bogus"`, "theme", "glyphs"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if e.stdout.Len() != 0 {
		t.Errorf("stdout = %q", e.stdout.String())
	}
}

func TestConfigSetWritesFile(t *testing.T) {
	e := newCfgEnv(t)
	if err := e.run("set", "theme", "catppuccin"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(e.path)
	if err != nil || !strings.Contains(string(b), "theme = catppuccin") {
		t.Fatalf("file = %q, err %v", b, err)
	}
	want := "theme = catppuccin  (" + filepath.Join("~", ".config", "aval", "config") + ")\n"
	if got := e.stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestConfigSetInvalidLeavesFileUntouched(t *testing.T) {
	e := newCfgEnv(t)
	orig := "# mine\ntheme = nord\nfoo = 1\n"
	e.write(t, orig)
	err := e.run("set", "theme", "catpuccin")
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "Did you mean") {
		t.Errorf("error = %q", err)
	}
	b, _ := os.ReadFile(e.path)
	if string(b) != orig {
		t.Errorf("file changed: %q", b)
	}
}

func TestConfigPath(t *testing.T) {
	e := newCfgEnv(t)
	if err := e.run("path"); err != nil {
		t.Fatal(err)
	}
	if got := e.stdout.String(); got != e.path+"\n" {
		t.Errorf("stdout = %q, want %q", got, e.path)
	}
}

func TestConfigUsageErrors(t *testing.T) {
	e := newCfgEnv(t)
	for _, args := range [][]string{{"get"}, {"set", "theme"}, {"frobnicate"}} {
		if err := e.run(args...); err == nil {
			t.Errorf("%v: want an error", args)
		}
	}
}

func TestConfigIsListedInUsage(t *testing.T) {
	if !knownCommand("config") {
		t.Fatal("config is not a known command")
	}
	var b bytes.Buffer
	printUsage(&b)
	if !strings.Contains(b.String(), "aval config") {
		t.Error("usage does not list config")
	}
}

func TestBareConfigOnATerminalOpensSettings(t *testing.T) {
	if openSettings == nil {
		t.Fatal("openSettings is not wired: bare aval config can never open the settings view")
	}
	origOpen, origTerm := openSettings, isTerminal
	t.Cleanup(func() { openSettings, isTerminal = origOpen, origTerm })

	var gotPath string
	openSettings = func(path string, getenv func(string) string) error {
		gotPath = path
		return nil
	}
	isTerminal = func(io.Writer) bool { return true }

	e := newCfgEnv(t)
	if err := e.run(); err != nil {
		t.Fatal(err)
	}
	if gotPath != e.path {
		t.Errorf("openSettings got path %q, want %q", gotPath, e.path)
	}
	if e.stdout.Len() != 0 {
		t.Errorf("bare config on a terminal also listed:\n%s", e.stdout.String())
	}
	// A subcommand never opens the view, terminal or not.
	gotPath = ""
	if err := e.run("list"); err != nil || gotPath != "" {
		t.Errorf("config list opened settings (err %v)", err)
	}
}

func TestBareConfigWithoutATerminalStillLists(t *testing.T) {
	origOpen := openSettings
	t.Cleanup(func() { openSettings = origOpen })
	openSettings = func(string, func(string) string) error {
		t.Error("settings opened without a terminal")
		return nil
	}
	e := newCfgEnv(t)
	if err := e.run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.stdout.String(), "theme") {
		t.Errorf("bare config without a terminal did not list:\n%s", e.stdout.String())
	}
}
