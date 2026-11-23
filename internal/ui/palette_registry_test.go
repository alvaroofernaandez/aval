package ui

import (
	"slices"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/config"
)

func TestThemeNamesMatchConfigThemes(t *testing.T) {
	// config validates the theme key against config.Themes; the registry is
	// what resolves it. A name in one and not the other is a theme that is
	// either rejected or unresolvable.
	got := ThemeNames()
	want := slices.Clone(config.Themes)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		for _, n := range got {
			if !slices.Contains(want, n) {
				t.Errorf("registered theme %q is missing from config.Themes", n)
			}
		}
		for _, n := range want {
			if !slices.Contains(got, n) {
				t.Errorf("config.Themes lists %q but the registry has no such theme", n)
			}
		}
	}
}

func TestEveryRegisteredThemeResolvesBothBackgrounds(t *testing.T) {
	for _, n := range ThemeNames() {
		for _, dark := range []bool{true, false} {
			if _, err := LookupPalette(n, dark); err != nil {
				t.Errorf("LookupPalette(%q, dark=%v): %v", n, dark, err)
			}
		}
	}
}
