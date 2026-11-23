package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func cfgFile(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "aval", "config")
	if text == "" {
		return p // missing file
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveUIDefaults(t *testing.T) {
	s, warns, err := resolveUI(uiFlags{}, envOf(nil), cfgFile(t, ""))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	want := uiSettings{Theme: "aval", Background: "auto", Glyphs: "auto", Motion: "auto", UpdateCheck: true}
	if s != want {
		t.Errorf("defaults = %+v, want %+v", s, want)
	}
}

// Precedence is flag > env > file > default. Each row sets only the layers it
// names, so the winning layer is the only one that can explain the value.
func TestResolveUIPrecedence(t *testing.T) {
	type row struct {
		name  string
		flags uiFlags
		env   map[string]string
		file  string
		get   func(uiSettings) any
		want  any
	}
	theme := func(s uiSettings) any { return s.Theme }
	bg := func(s uiSettings) any { return s.Background }
	gl := func(s uiSettings) any { return s.Glyphs }
	mo := func(s uiSettings) any { return s.Motion }
	uc := func(s uiSettings) any { return s.UpdateCheck }
	rows := []row{
		{"theme file", uiFlags{}, nil, "theme = nord\n", theme, "nord"},
		{"theme env beats file", uiFlags{}, map[string]string{"AVAL_THEME": "dracula"}, "theme = nord\n", theme, "dracula"},
		{"theme flag beats env", uiFlags{Theme: "gruvbox"}, map[string]string{"AVAL_THEME": "dracula"}, "theme = nord\n", theme, "gruvbox"},
		{"background file", uiFlags{}, nil, "background = light\n", bg, "light"},
		{"background env beats file", uiFlags{}, map[string]string{"AVAL_BACKGROUND": "dark"}, "background = light\n", bg, "dark"},
		{"glyphs file", uiFlags{}, nil, "glyphs = ascii\n", gl, "ascii"},
		{"glyphs env beats file", uiFlags{}, map[string]string{"AVAL_GLYPHS": "unicode"}, "glyphs = ascii\n", gl, "unicode"},
		{"motion file", uiFlags{}, nil, "motion = off\n", mo, "off"},
		{"motion env beats file", uiFlags{}, map[string]string{"AVAL_MOTION": "auto"}, "motion = off\n", mo, "auto"},
		{"update_check file off", uiFlags{}, nil, "update_check = off\n", uc, false},
		{"update_check env beats file on", uiFlags{}, map[string]string{"AVAL_NO_UPDATE_CHECK": "1"}, "update_check = on\n", uc, false},
		{"update_check default on", uiFlags{}, nil, "", uc, true},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			s, warns, err := resolveUI(r.flags, envOf(r.env), cfgFile(t, r.file))
			if err != nil || len(warns) != 0 {
				t.Fatalf("err=%v warns=%v", err, warns)
			}
			if got := r.get(s); got != r.want {
				t.Errorf("got %v, want %v", got, r.want)
			}
		})
	}
}

func TestResolveUIPlainMeansMonoAsciiNoMotion(t *testing.T) {
	// --plain outranks every layer, including an explicit --theme-less env and file.
	s, _, err := resolveUI(uiFlags{Plain: true},
		envOf(map[string]string{"AVAL_THEME": "dracula", "AVAL_GLYPHS": "unicode", "AVAL_MOTION": "auto"}),
		cfgFile(t, "theme = nord\nglyphs = unicode\nmotion = auto\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Theme != "mono" || s.Glyphs != "ascii" || s.Motion != "off" {
		t.Errorf("plain = %+v", s)
	}
}

func TestResolveUINoColorWinsOverEverything(t *testing.T) {
	s, _, err := resolveUI(uiFlags{Theme: "dracula"}, envOf(map[string]string{"NO_COLOR": "1", "COLORTERM": "truecolor"}),
		cfgFile(t, "theme = nord\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !s.ColorOff {
		t.Fatal("NO_COLOR did not set ColorOff")
	}
	c := caps.Detect(s.capsInputs(envOf(map[string]string{"COLORTERM": "truecolor"}), "linux", true, 80, 24))
	if c.Profile != termenv.Ascii {
		t.Errorf("profile = %v, want Ascii", c.Profile)
	}
}

func TestResolveUIBadFileValueWarnsOnceAndFallsBack(t *testing.T) {
	p := cfgFile(t, "# c\ntheme = tokio-night\nmotion = off\n")
	s, warns, err := resolveUI(uiFlags{}, envOf(nil), p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Theme != "aval" {
		t.Errorf("theme = %q, want fallback aval", s.Theme)
	}
	if s.Motion != "off" {
		t.Errorf("a good sibling value was lost: motion = %q", s.Motion)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], p+":2") || !strings.Contains(warns[0], "tokyo-night") {
		t.Errorf("warnings = %q", warns)
	}
}

func TestResolveUIBadThemeFlagIsAnErrorWithSuggestion(t *testing.T) {
	_, _, err := resolveUI(uiFlags{Theme: "tokio-night"}, envOf(nil), cfgFile(t, ""))
	if err == nil {
		t.Fatal("a bad --theme must be an error")
	}
	if !strings.Contains(err.Error(), "--theme") || !strings.Contains(err.Error(), "tokyo-night") {
		t.Errorf("err = %v", err)
	}
}

func TestResolveUIUnreadableConfigWarnsAndUsesDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "aval", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil { // a directory where the file should be
		t.Fatal(err)
	}
	s, warns, err := resolveUI(uiFlags{}, envOf(nil), dir)
	if err != nil || len(warns) != 1 || s.Theme != "aval" {
		t.Errorf("s=%+v warns=%q err=%v", s, warns, err)
	}
}

func TestUISettingsCapsInputs(t *testing.T) {
	s := uiSettings{Theme: "mono", Background: "light", Glyphs: "ascii", Motion: "off", ColorOff: true}
	in := s.capsInputs(envOf(nil), "linux", true, 100, 30)
	if in.Glyphs != "ascii" || in.Motion != "off" || in.Background != "light" || !in.ColorOff || !in.IsTTY || in.Width != 100 || in.Height != 30 || in.GOOS != "linux" {
		t.Errorf("inputs = %+v", in)
	}
}

func TestSettingsForCommandSkipsConfigForHotPaths(t *testing.T) {
	old := loadUIConfig
	defer func() { loadUIConfig = old }()
	called := 0
	loadUIConfig = func(string) (*config.File, []config.Warning, error) {
		called++
		return old("")
	}
	for _, cmd := range []string{"hook", "statusline", "tmux", "config", "version", "budget", "sync"} {
		if _, err := settingsForCommand(cmd, uiFlags{}, envOf(nil), t.TempDir(), io.Discard); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}
	if called != 0 {
		t.Fatalf("config was loaded %d time(s) for commands that must not read it", called)
	}
	for _, cmd := range []string{"install", "doctor", ""} {
		before := called
		if _, err := settingsForCommand(cmd, uiFlags{}, envOf(nil), t.TempDir(), io.Discard); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		if called != before+1 {
			t.Errorf("%q must load the config exactly once", cmd)
		}
	}
}

func TestSettingsForCommandWarnsOnceExceptDoctor(t *testing.T) {
	home := t.TempDir()
	path := config.Path(envOf(nil), home, runtime.GOOS)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("theme = tokio-night\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errb strings.Builder
	s, err := settingsForCommand("install", uiFlags{}, envOf(nil), home, &errb)
	if err != nil || s.Theme != "aval" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
	if got := strings.Count(errb.String(), "warning:"); got != 1 || !strings.Contains(errb.String(), "tokyo-night") {
		t.Errorf("stderr = %q", errb.String())
	}
	errb.Reset()
	_, _ = settingsForCommand("doctor", uiFlags{}, envOf(nil), home, &errb)
	if errb.Len() != 0 {
		t.Errorf("doctor reports through its config check, not stderr: %q", errb.String())
	}
}

func TestSettingsForCommandBadThemeFlagFails(t *testing.T) {
	if _, err := settingsForCommand("install", uiFlags{Theme: "nope"}, envOf(nil), t.TempDir(), io.Discard); err == nil {
		t.Error("bad --theme must fail install")
	}
}
