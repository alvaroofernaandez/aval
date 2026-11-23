package ui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// trueColorCaps is a dark Unicode terminal with 24-bit colour.
var trueColorCaps = caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true, Width: 100, Height: 30}

// trueColorTheme builds name on a TrueColor renderer, so colour escapes are
// emitted and a theme change is visible in the output.
func trueColorTheme(t *testing.T, name string) Theme {
	t.Helper()
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	th, err := NewTheme(name, trueColorCaps, r)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func envOf(vals map[string]string) func(string) string {
	return func(k string) string { return vals[k] }
}

// settingsModel opens the standalone settings screen over a temp config file
// holding text ("" leaves the file missing).
func settingsModel(t *testing.T, text string, env map[string]string) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "aval", "config")
	if text != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := NewSettings(Options{
		Version: "0.2.0", Caps: trueColorCaps,
		Settings: SettingsEnv{Path: path, Home: dir, Getenv: envOf(env)},
	})
	return m.WithTheme(trueColorTheme(t, DefaultThemeName)), path
}

func TestSettingsArrowsCycleTheValueAndRepaintThePreview(t *testing.T) {
	m, _ := settingsModel(t, "", nil)
	if m.Screen() != ScreenSettings {
		t.Fatalf("NewSettings opened screen %v", m.Screen())
	}
	before := renderAtSize(m, 100, 30)
	avalOK := trueColorTheme(t, "aval").State(StateOK, "linked")
	if !strings.Contains(before, avalOK) {
		t.Fatalf("preview is not in the aval theme:\n%q", before)
	}

	m = send(m, "right")
	after := renderAtSize(m, 100, 30)
	catOK := trueColorTheme(t, "catppuccin").State(StateOK, "linked")
	if catOK == avalOK {
		t.Fatal("catppuccin and aval share the ok colour; pick another theme")
	}
	if !strings.Contains(after, catOK) {
		t.Errorf("right did not repaint the preview in catppuccin:\n%q", after)
	}
	if !strings.Contains(after, "catppuccin") {
		t.Errorf("the theme row does not show catppuccin:\n%s", after)
	}
	if got := renderAtSize(send(m, "left"), 100, 30); got != before {
		t.Errorf("left did not cycle back to aval:\n%s", got)
	}

	// Glyphs: down to the third row; auto -> unicode -> ascii.
	g := send(send(send(send(m, "down"), "down"), "right"), "right")
	if v := renderAtSize(g, 100, 30); !strings.Contains(v, "[x]") || strings.Contains(v, "[√]") {
		t.Errorf("glyphs = ascii did not repaint the preview with ASCII glyphs:\n%s", v)
	}
}

func TestSettingsThemeCycleIsFamiliesOnly(t *testing.T) {
	m, _ := settingsModel(t, "", nil)
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		seen[m.settings.values["theme"]] = true
		m = send(m, "right")
	}
	for _, want := range []string{"aval", "catppuccin", "tokyo-night", "terminal", "mono"} {
		if !seen[want] {
			t.Errorf("theme cycle never reached %q: %v", want, seen)
		}
	}
	for _, pinned := range []string{"aval-dark", "catppuccin-latte", "gruvbox-light"} {
		if seen[pinned] {
			t.Errorf("theme cycle includes the pinned variant %q", pinned)
		}
	}
	if len(seen) > 14 {
		t.Errorf("theme cycle has %d values", len(seen))
	}

	pinned, _ := settingsModel(t, "theme = catppuccin-latte\n", nil)
	if v := renderAtSize(pinned, 100, 30); !strings.Contains(v, "catppuccin-latte") {
		t.Errorf("a pinned variant in the file is not shown:\n%s", v)
	}
}

func TestSettingsEnterWritesChangedKeysThroughConfigSet(t *testing.T) {
	m, path := settingsModel(t, "# mine\nupdate_check = on\n", nil)
	m = send(m, "right") // theme -> catppuccin
	for i := 0; i < 4; i++ {
		m = send(m, "down")
	}
	m = send(m, "right") // update_check -> off
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	if cmd == nil {
		t.Error("enter on the standalone settings screen did not quit")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"# mine\n", "theme = catppuccin\n", "update_check = off\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("config lacks %q:\n%s", want, got)
		}
	}
	for _, unchanged := range []string{"glyphs", "background", "motion"} {
		if strings.Contains(got, unchanged) {
			t.Errorf("unchanged key %q was written:\n%s", unchanged, got)
		}
	}
	f, warns, err := config.Load(path)
	if err != nil || len(warns) > 0 {
		t.Fatalf("written file does not load cleanly: %v %v", err, warns)
	}
	if v, _ := f.Get("theme"); v != "catppuccin" {
		t.Errorf("theme = %q after save", v)
	}
}

func TestSettingsEscLeavesTheFileByteIdentical(t *testing.T) {
	text := "# keep me\ntheme   =   nord\nmystery = 1\n"
	m, path := settingsModel(t, text, nil)
	m = send(send(send(m, "right"), "down"), "right")
	next, cmd := m.Update(key("esc"))
	if cmd == nil {
		t.Error("esc on the standalone settings screen did not quit")
	}
	_ = next
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != text {
		t.Errorf("esc changed the file:\n%q\nwant\n%q", b, text)
	}
}

func TestSettingsQQuitsWithoutWriting(t *testing.T) {
	m, path := settingsModel(t, "", nil)
	next, cmd := send(m, "right").Update(key("q"))
	if cmd == nil || !next.(Model).Quitting {
		t.Error("q did not quit")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("q wrote the config: %v", err)
	}
}

func TestSettingsNotesAnEnvOverrideAndStillEditsTheFile(t *testing.T) {
	m, path := settingsModel(t, "", map[string]string{"AVAL_THEME": "nord"})
	const note = "theme is set by $AVAL_THEME in this shell; the file value applies elsewhere"
	if v := renderAtSize(m, 100, 30); !strings.Contains(v, note) {
		t.Errorf("no env note:\n%s", v)
	}
	m = send(m, "right")
	if got := m.settings.values["theme"]; got != "catppuccin" {
		t.Errorf("right edited %q, want the file value to move to catppuccin", got)
	}
	send(m, "enter")
	f, _, _ := config.Load(path)
	if v, _ := f.Get("theme"); v != "catppuccin" {
		t.Errorf("file theme = %q, want catppuccin", v)
	}

	plain, _ := settingsModel(t, "", nil)
	if v := renderAtSize(plain, 100, 30); strings.Contains(v, "in this shell") {
		t.Errorf("a note without any override:\n%s", v)
	}
}

func TestSettingsHeaderShowsTheConfigPath(t *testing.T) {
	m, _ := settingsModel(t, "", nil)
	v := renderAtSize(m, 100, 30)
	if !strings.Contains(v, "Settings") || !strings.Contains(v, filepath.Join("~", "aval", "config")) {
		t.Errorf("header lacks the title or the path:\n%s", v)
	}
	for _, row := range []string{"Theme", "Background", "Glyphs", "Motion", "Update check", "Repository"} {
		if !strings.Contains(v, row) {
			t.Errorf("no %s row:\n%s", row, v)
		}
	}
}

func TestSettingsOpensFromSelectAndDoneAndReturns(t *testing.T) {
	for _, s := range []Screen{ScreenSelect, ScreenDone} {
		m := screenFixture(t, s, true)
		before := renderAt(m, 80)
		open := send(m, "s")
		if open.Screen() != ScreenSettings {
			t.Fatalf("screen %v: s opened %v", s, open.Screen())
		}
		back := send(open, "esc")
		if back.Screen() != s {
			t.Errorf("esc from settings went to %v, want %v", back.Screen(), s)
		}
		if got := renderAt(back, 80); got != before {
			t.Errorf("screen %v: returning did not restore the view:\n%s", s, got)
		}
	}
}

func TestSavingFromTheInstallerAppliesTheNewTheme(t *testing.T) {
	m := screenFixture(t, ScreenSelect, true)
	path := filepath.Join(t.TempDir(), "config")
	m.settingsEnv.Path = path
	m = m.WithTheme(trueColorTheme(t, DefaultThemeName))
	m = send(send(send(m, "s"), "right"), "enter")
	if m.Screen() != ScreenSelect {
		t.Fatalf("enter went to %v, want the select screen", m.Screen())
	}
	if m.Quitting {
		t.Error("saving from the installer quit it")
	}
	if !strings.HasPrefix(m.theme.Name, "catppuccin") {
		t.Errorf("theme after save = %q, want catppuccin", m.theme.Name)
	}
}

func TestCompactHintsKeepHelpOnEveryScreen(t *testing.T) {
	for _, unicode := range []bool{true, false} {
		for _, s := range goldenScreens {
			m := screenFixture(t, s.screen, unicode)
			for _, w := range []int{20, 30, 36, 39} {
				for _, h := range []int{12, 40} {
					if v := renderAtSize(m, w, h); !strings.Contains(v, "? help") {
						t.Errorf("%s unicode=%v %dx%d: no ? help:\n%s", s.name, unicode, w, h, v)
					}
				}
			}
		}
	}
}
