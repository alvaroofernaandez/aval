package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/i18n"
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
	want := uiSettings{Theme: "aval", Background: "auto", Glyphs: "auto", Motion: "auto", Lang: i18n.English, UpdateCheck: true}
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
	if !errors.Is(err, errBadThemeFlag) {
		t.Fatalf("err = %v, want errBadThemeFlag", err)
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

// The language follows flag > env > file > detection > English. Each row sets
// only the layers it names, plus a locale that disagrees with the winner, so
// only the winning layer can explain the result.
func TestResolveUILanguagePrecedence(t *testing.T) {
	esLocale := map[string]string{"LANG": "es_ES.UTF-8"}
	rows := []struct {
		name string
		flag string
		env  map[string]string
		file string
		want i18n.Lang
	}{
		{"nothing set is English", "", nil, "", i18n.English},
		{"C locale is English", "", map[string]string{"LANG": "C"}, "", i18n.English},
		{"detection: LANG es_ES.UTF-8", "", esLocale, "", i18n.Spanish},
		{"detection: LC_ALL beats LANG", "", map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "es_ES.UTF-8"}, "", i18n.English},
		{"file auto detects", "", esLocale, "language = auto\n", i18n.Spanish},
		{"file beats detection (en)", "", esLocale, "language = en\n", i18n.English},
		{"file beats detection (es)", "", map[string]string{"LANG": "en_US.UTF-8"}, "language = es\n", i18n.Spanish},
		{"env beats file (en)", "", map[string]string{"AVAL_LANG": "en", "LANG": "es_ES.UTF-8"}, "language = es\n", i18n.English},
		{"env beats file (es)", "", map[string]string{"AVAL_LANG": "es"}, "language = en\n", i18n.Spanish},
		{"env auto detects over the file", "", map[string]string{"AVAL_LANG": "auto", "LANG": "C"}, "language = es\n", i18n.English},
		{"a bad env is skipped", "", map[string]string{"AVAL_LANG": "klingon"}, "language = es\n", i18n.Spanish},
		{"flag beats env (en)", "en", map[string]string{"AVAL_LANG": "es", "LANG": "es_ES.UTF-8"}, "language = es\n", i18n.English},
		{"flag beats env (es)", "es", map[string]string{"AVAL_LANG": "en"}, "language = en\n", i18n.Spanish},
		{"flag auto detects", "auto", map[string]string{"AVAL_LANG": "en", "LANG": "es_ES.UTF-8"}, "", i18n.Spanish},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			s, warns, err := resolveUI(uiFlags{Lang: r.flag}, envOf(r.env), cfgFile(t, r.file))
			if err != nil || len(warns) != 0 {
				t.Fatalf("err=%v warns=%v", err, warns)
			}
			if s.Lang != r.want {
				t.Errorf("Lang = %q, want %q", s.Lang, r.want)
			}
		})
	}
}

// A bad language in the file warns once, with the file, the line and a
// suggestion, and the next layer (detection) decides.
func TestResolveUIBadLanguageWarnsWithSuggestionAndFallsBack(t *testing.T) {
	for _, c := range []struct {
		locale string
		want   i18n.Lang
	}{{"es_ES.UTF-8", i18n.Spanish}, {"C", i18n.English}} {
		p := cfgFile(t, "language = esp\ntheme = nord\n")
		s, warns, err := resolveUI(uiFlags{}, envOf(map[string]string{"LANG": c.locale}), p)
		if err != nil {
			t.Fatal(err)
		}
		if s.Lang != c.want {
			t.Errorf("LANG=%s: Lang = %q, want the detected %q", c.locale, s.Lang, c.want)
		}
		if s.Theme != "nord" {
			t.Errorf("a good sibling value was lost: theme = %q", s.Theme)
		}
		if len(warns) != 1 || !strings.Contains(warns[0], p+":1") || !strings.Contains(warns[0], "esp") || !strings.Contains(warns[0], " es?") {
			t.Errorf("LANG=%s: warnings = %q", c.locale, warns)
		}
	}
}

// In English the rendered warning is exactly the config package's own text,
// so a language that is English changes nothing.
func TestLanguageWarningInEnglishMatchesTheConfigText(t *testing.T) {
	p := cfgFile(t, "language = esp\n")
	_, cw, _ := config.Load(p)
	_, warns, _ := resolveUI(uiFlags{}, envOf(nil), p)
	if len(cw) != 1 || len(warns) != 1 || warns[0] != cw[0].Message {
		t.Errorf("rendered %q, config says %q", warns, cw)
	}
}

func TestResolveUIBadLangFlagIsAnError(t *testing.T) {
	_, _, err := resolveUI(uiFlags{Lang: "esp"}, envOf(nil), cfgFile(t, ""))
	if !errors.Is(err, errBadLangFlag) {
		t.Fatalf("err = %v, want errBadLangFlag", err)
	}
}

// The language setting accepts auto and exactly the languages with a catalog.
func TestLanguageValuesMatchTheCatalog(t *testing.T) {
	for _, l := range i18n.Languages() {
		if msg := config.Validate("language", string(l)); msg != "" {
			t.Errorf("language = %s has a catalog but config rejects it: %s", l, msg)
		}
	}
	if config.Validate("language", "auto") != "" {
		t.Error("language = auto rejected")
	}
	if config.Validate("language", "fr") == "" {
		t.Error("language = fr accepted without a catalog")
	}
}

func TestNewPrinterForCarriesTheLanguage(t *testing.T) {
	if got := newPrinterFor(io.Discard, uiSettings{Lang: i18n.Spanish}).Lang(); got != i18n.Spanish {
		t.Errorf("printer speaks %q, want es", got)
	}
}

// The settings screen notes a language set by --lang the way it notes --theme,
// so its Language row does not preview a value the flag overrides.
func TestSettingsEnvReportsTheLangFlag(t *testing.T) {
	env := uiSettings{flags: uiFlags{Lang: "es"}}.settingsEnv("/cfg", "/home", func(string) string { return "" }, false, 80, 24)
	if got := env.Flags["language"]; got != "es" {
		t.Errorf("Flags[language] = %q, want es", got)
	}
	if got := env.FlagNames["language"]; got != "--lang" {
		t.Errorf("FlagNames[language] = %q, want --lang", got)
	}
	none := uiSettings{}.settingsEnv("/cfg", "/home", func(string) string { return "" }, false, 80, 24)
	if _, ok := none.Flags["language"]; ok {
		t.Error("language is reported as set by a flag when no flag was given")
	}
}
