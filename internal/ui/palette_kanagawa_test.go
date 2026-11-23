package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/rebelot/kanagawa.nvim (lua/kanagawa/colors.lua)
// Hex values are pinned here. Text tokens need 4.5:1 and swap only to a
// neutral entry; state tokens need 3:1 and fall back to strong. Each swap is listed in kanagawaSwaps.
func testKanagawaPalettes() []Palette {
	return []Palette{
		{
			Name:       "kanagawa-wave",
			Background: "#1f1f28",
			Text:       "#dcd7ba", Strong: "#dcd7ba", Muted: "#c8c093", Rule: "#363646",
			OK: "#98bb6c", Warn: "#e6c384", Fail: "#e46876",
			Key: "#ffa066",
		},
		{
			Name:       "kanagawa-dragon",
			Background: "#181616",
			Text:       "#c5c9c5", Strong: "#c5c9c5", Muted: "#7a8382", Rule: "#393836",
			OK: "#8a9a7b", Warn: "#c4b28a", Fail: "#c4746e",
			Key: "#b6927b",
		},
		{
			Name:       "kanagawa-lotus",
			Background: "#f2ecbc",
			Text:       "#545464", Strong: "#545464", Muted: "#545464", Rule: "#e4d794",
			OK: "#6f894e", Warn: "#77713f", Fail: "#c84053",
			Key: "#545464",
		},
	}
}

// kanagawaSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var kanagawaSwaps = []struct {
	palette, token, from, fromName string
}{
	{"kanagawa-wave", "muted", "#727169", "fujiGray"},    // 3.33:1 -> oldWhite #c8c093 8.89:1
	{"kanagawa-lotus", "muted", "#716e61", "lotusGray2"}, // 4.26:1 -> lotusInk1 #545464 6.19:1
	{"kanagawa-lotus", "key", "#cc6d00", "lotusOrange"},  // 3.04:1 -> lotusInk1 #545464 6.19:1
}

func TestKanagawaPalettesMatchUpstream(t *testing.T) {
	for _, want := range testKanagawaPalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestKanagawaSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed its contrast floor, and
	// the palette no longer carries it.
	for _, sw := range kanagawaSwaps {
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

func TestKanagawaFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"kanagawa", true, "kanagawa-wave"},
		{"kanagawa", false, "kanagawa-lotus"},
		{"kanagawa-dragon", true, "kanagawa-dragon"},
		{"kanagawa-dragon", false, "kanagawa-dragon"},
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
