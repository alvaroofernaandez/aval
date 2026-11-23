package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/rose-pine/neovim (lua/rose-pine/palette.lua)
// Hex values are pinned here. A token that missed 4.5:1 on its own background
// was replaced by the closest entry of the same palette that passes; each
// swap is listed in rosePineSwaps.
func testRosePinePalettes() []Palette {
	return []Palette{
		{
			Name:       "rose-pine",
			Background: "#191724",
			Text:       "#e0def4", Strong: "#e0def4", Muted: "#908caa", Rule: "#403d52",
			OK: "#95b1ac", Warn: "#f6c177", Fail: "#eb6f92",
			Key: "#ebbcba",
		},
		{
			Name:       "rose-pine-moon",
			Background: "#232136",
			Text:       "#e0def4", Strong: "#e0def4", Muted: "#908caa", Rule: "#44415a",
			OK: "#95b1ac", Warn: "#f6c177", Fail: "#eb6f92",
			Key: "#ea9a97",
		},
		{
			Name:       "rose-pine-dawn",
			Background: "#faf4ed",
			Text:       "#464261", Strong: "#464261", Muted: "#464261", Rule: "#dfdad9",
			OK: "#286983", Warn: "#464261", Fail: "#464261",
			Key: "#464261",
		},
	}
}

// rosePineSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var rosePineSwaps = []struct {
	palette, token, from, fromName string
}{
	{"rose-pine", "ok", "#31748f", "pine"},           // 3.38:1 -> leaf #95b1ac 7.70:1
	{"rose-pine-moon", "ok", "#3e8fb0", "pine"},      // 4.29:1 -> leaf #95b1ac 6.83:1
	{"rose-pine-dawn", "muted", "#797593", "subtle"}, // 4.02:1 -> text #464261 8.69:1
	{"rose-pine-dawn", "warn", "#ea9d34", "gold"},    // 2.05:1 -> text #464261 8.69:1
	{"rose-pine-dawn", "fail", "#b4637a", "love"},    // 3.84:1 -> text #464261 8.69:1
	{"rose-pine-dawn", "key", "#d7827e", "rose"},     // 2.60:1 -> text #464261 8.69:1
}

func TestRosePinePalettesMatchUpstream(t *testing.T) {
	for _, want := range testRosePinePalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestRosePineTokensReachAAContrast(t *testing.T) {
	// rule is decorative and exempt.
	for _, want := range testRosePinePalettes() {
		p, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		for token, fg := range p.meaningful() {
			r, err := gate.Ratio(fg, p.Background)
			if err != nil {
				t.Fatalf("%s.%s: %v", p.Name, token, err)
			}
			if r < 4.5 {
				t.Errorf("%s.%s %s on %s = %.2f:1, want >= 4.5", p.Name, token, fg, p.Background, r)
			}
		}
	}
}

func TestRosePineSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed 4.5:1, and
	// the palette no longer carries it.
	for _, sw := range rosePineSwaps {
		p, err := LookupPalette(sw.palette, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", sw.palette, err)
		}
		r, err := gate.Ratio(sw.from, p.Background)
		if err != nil {
			t.Fatal(err)
		}
		if r >= 4.5 {
			t.Errorf("%s.%s swapped out %s (%s) at %.2f:1, but it already passes", sw.palette, sw.token, sw.fromName, sw.from, r)
		}
		if p.meaningful()[sw.token] == sw.from {
			t.Errorf("%s.%s still uses %s", sw.palette, sw.token, sw.from)
		}
	}
}

func TestRosePineFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"rose-pine", true, "rose-pine"},
		{"rose-pine", false, "rose-pine-dawn"},
		{"rose-pine-moon", true, "rose-pine-moon"},
		{"rose-pine-moon", false, "rose-pine-moon"},
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
