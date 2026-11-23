package ui

import (
	"testing"

	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

func modelWithTheme(name string) Model {
	c := caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true, Width: 80, Height: 24}
	return NewModel(&plan.Plan{}, install.New(install.Options{Home: "/nonexistent", Symlinks: true}),
		Options{Caps: c, Theme: name})
}

func TestOptionsThemeSelectsThePalette(t *testing.T) {
	c := caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true}
	want, err := NewTheme("nord", c, nil)
	if err != nil {
		t.Fatal(err)
	}
	def, _ := NewTheme(DefaultThemeName, c, nil)
	if want.Success.GetForeground() == def.Success.GetForeground() {
		t.Skip("nord and the default share a success colour; pick another palette")
	}
	if got := modelWithTheme("nord").theme.Success.GetForeground(); got != want.Success.GetForeground() {
		t.Errorf("Options.Theme = nord ignored: got %v want %v", got, want.Success.GetForeground())
	}
	if got := modelWithTheme("").theme.Success.GetForeground(); got != def.Success.GetForeground() {
		t.Errorf("empty Options.Theme must keep the default: got %v", got)
	}
	if got := modelWithTheme("not-a-theme").theme.Success.GetForeground(); got != def.Success.GetForeground() {
		t.Errorf("an unknown theme must fall back to the default: got %v", got)
	}
}

// The resolved language reaches the frame every view draws with, for the
// installer and settings (Options) and for the hub (NewHub).
func TestTheLanguageReachesTheChrome(t *testing.T) {
	c := caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true, Width: 80, Height: 24}
	m := NewModel(&plan.Plan{}, install.New(install.Options{Home: "/nonexistent", Symlinks: true}),
		Options{Caps: c, Lang: i18n.Spanish})
	if got := m.tr.Lang(); got != i18n.Spanish {
		t.Errorf("installer chrome speaks %q, want es", got)
	}
	if got := NewSettings(Options{Caps: c, Lang: i18n.Spanish}).tr.Lang(); got != i18n.Spanish {
		t.Errorf("settings chrome speaks %q, want es", got)
	}
	if got := NewHub(DefaultTheme(), "macOS (arm64)", Services{}, false, i18n.Spanish).(App).tr.Lang(); got != i18n.Spanish {
		t.Errorf("hub chrome speaks %q, want es", got)
	}
	if got := NewModel(&plan.Plan{}, nil, Options{Caps: c}).tr.Lang(); got != i18n.English {
		t.Errorf("no language must be English, got %q", got)
	}
}
