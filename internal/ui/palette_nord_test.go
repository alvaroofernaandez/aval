package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/nordtheme/nord (src/nord.css)
// Hex values are pinned here. A token that missed 4.5:1 on its own background
// was replaced by the closest entry of the same palette that passes; each
// swap is listed in nordSwaps.
func testNordPalettes() []Palette {
	return []Palette{
		{
			Name:       "nord",
			Background: "#2e3440",
			Text:       "#d8dee9", Strong: "#d8dee9", Muted: "#81a1c1", Rule: "#434c5e",
			OK: "#a3be8c", Warn: "#ebcb8b", Fail: "#d8dee9",
			Key: "#88c0d0",
		},
	}
}

// nordSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var nordSwaps = []struct {
	palette, token, from, fromName string
}{
	{"nord", "muted", "#4c566a", "nord3"}, // 1.69:1 -> nord9 #81a1c1 4.64:1
	{"nord", "fail", "#bf616a", "nord11"}, // 3.05:1 -> nord4 #d8dee9 9.25:1
}

func TestNordPalettesMatchUpstream(t *testing.T) {
	for _, want := range testNordPalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestNordTokensReachAAContrast(t *testing.T) {
	// rule is decorative and exempt.
	for _, want := range testNordPalettes() {
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

func TestNordSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed 4.5:1, and
	// the palette no longer carries it.
	for _, sw := range nordSwaps {
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

func TestNordFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"nord", true, "nord"},
		{"nord", false, "nord"},
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
