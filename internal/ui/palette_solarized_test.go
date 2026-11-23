package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/altercation/solarized (README.md palette table)
// Hex values are pinned here. A token that missed 4.5:1 on its own background
// was replaced by the closest entry of the same palette that passes; each
// swap is listed in solarizedSwaps.
func testSolarizedPalettes() []Palette {
	return []Palette{
		{
			Name:       "solarized-dark",
			Background: "#002b36",
			Text:       "#839496", Strong: "#93a1a1", Muted: "#839496", Rule: "#073642",
			OK: "#859900", Warn: "#b58900", Fail: "#839496",
			Key: "#93a1a1",
		},
		{
			Name:       "solarized-light",
			Background: "#fdf6e3",
			Text:       "#586e75", Strong: "#586e75", Muted: "#586e75", Rule: "#eee8d5",
			OK: "#586e75", Warn: "#073642", Fail: "#002b36",
			Key: "#586e75",
		},
	}
}

// solarizedSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var solarizedSwaps = []struct {
	palette, token, from, fromName string
}{
	{"solarized-dark", "muted", "#586e75", "base01"}, // 2.79:1 -> base0 #839496 4.75:1
	{"solarized-dark", "fail", "#dc322f", "red"},     // 3.25:1 -> base0 #839496 4.75:1
	{"solarized-dark", "key", "#cb4b16", "orange"},   // 3.26:1 -> base1 #93a1a1 5.61:1
	{"solarized-light", "text", "#657b83", "base00"}, // 4.13:1 -> base01 #586e75 4.99:1
	{"solarized-light", "muted", "#93a1a1", "base1"}, // 2.48:1 -> base01 #586e75 4.99:1
	{"solarized-light", "ok", "#859900", "green"},    // 2.97:1 -> base01 #586e75 4.99:1
	{"solarized-light", "warn", "#b58900", "yellow"}, // 2.98:1 -> base02 #073642 12.05:1
	{"solarized-light", "fail", "#dc322f", "red"},    // 4.29:1 -> base03 #002b36 13.92:1
	{"solarized-light", "key", "#cb4b16", "orange"},  // 4.27:1 -> base01 #586e75 4.99:1
}

func TestSolarizedPalettesMatchUpstream(t *testing.T) {
	for _, want := range testSolarizedPalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestSolarizedTokensReachAAContrast(t *testing.T) {
	// rule is decorative and exempt.
	for _, want := range testSolarizedPalettes() {
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

func TestSolarizedSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed 4.5:1, and
	// the palette no longer carries it.
	for _, sw := range solarizedSwaps {
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

func TestSolarizedFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"solarized", true, "solarized-dark"},
		{"solarized", false, "solarized-light"},
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
