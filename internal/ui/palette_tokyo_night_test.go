package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/folke/tokyonight.nvim (extras/lua/tokyonight_<variant>.lua)
// Hex values are pinned here. Text tokens need 4.5:1 and swap only to a
// neutral entry; state tokens need 3:1 and fall back to strong. Each swap is listed in tokyoNightSwaps.
func testTokyoNightPalettes() []Palette {
	return []Palette{
		{
			Name:       "tokyo-night",
			Background: "#1a1b26",
			Text:       "#c0caf5", Strong: "#c0caf5", Muted: "#a9b1d6", Rule: "#3b4261",
			OK: "#9ece6a", Warn: "#e0af68", Fail: "#f7768e",
			Key: "#ff9e64",
		},
		{
			Name:       "tokyo-night-storm",
			Background: "#24283b",
			Text:       "#c0caf5", Strong: "#c0caf5", Muted: "#a9b1d6", Rule: "#3b4261",
			OK: "#9ece6a", Warn: "#e0af68", Fail: "#f7768e",
			Key: "#ff9e64",
		},
		{
			Name:       "tokyo-night-day",
			Background: "#e1e2e7",
			Text:       "#3760bf", Strong: "#3760bf", Muted: "#3760bf", Rule: "#a8aecb",
			OK: "#587539", Warn: "#8c6c3e", Fail: "#f52a65",
			Key: "#3760bf",
		},
	}
}

// tokyoNightSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var tokyoNightSwaps = []struct {
	palette, token, from, fromName string
}{
	{"tokyo-night", "muted", "#565f89", "comment"},       // 2.76:1 -> fg_dark #a9b1d6 8.10:1
	{"tokyo-night-storm", "muted", "#565f89", "comment"}, // 2.35:1 -> fg_dark #a9b1d6 6.90:1
	{"tokyo-night-day", "muted", "#848cb5", "comment"},   // 2.54:1 -> fg #3760bf 4.52:1
	{"tokyo-night-day", "key", "#b15c00", "orange"},      // 3.69:1 -> fg #3760bf 4.52:1
}

func TestTokyoNightPalettesMatchUpstream(t *testing.T) {
	for _, want := range testTokyoNightPalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestTokyoNightSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed its contrast floor, and
	// the palette no longer carries it.
	for _, sw := range tokyoNightSwaps {
		p, err := LookupPalette(sw.palette, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", sw.palette, err)
		}
		r, err := gate.Ratio(sw.from, p.Background)
		if err != nil {
			t.Fatal(err)
		}
		if r >= contrastFloor(sw.token) {
			t.Errorf("%s.%s swapped out %s (%s) at %.2f:1, but it already passes", sw.palette, sw.token, sw.fromName, sw.from, r)
		}
		if p.meaningful()[sw.token] == sw.from {
			t.Errorf("%s.%s still uses %s", sw.palette, sw.token, sw.from)
		}
	}
}

func TestTokyoNightFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"tokyo-night", true, "tokyo-night"},
		{"tokyo-night", false, "tokyo-night-day"},
		{"tokyo-night-storm", true, "tokyo-night-storm"},
		{"tokyo-night-storm", false, "tokyo-night-storm"},
	} {
		p, err := LookupPalette(c.name, c.dark)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", c.name, err)
		}
		if p.Name != c.want {
			t.Errorf("%s with dark=%v resolved to %q, want %q", c.name, c.dark, p.Name, c.want)
		}
	}
}
