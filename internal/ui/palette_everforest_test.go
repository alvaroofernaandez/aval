package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/sainnhe/everforest (autoload/everforest.vim, medium contrast)
// Hex values are pinned here. A token that missed 4.5:1 on its own background
// was replaced by the closest entry of the same palette that passes; each
// swap is listed in everforestSwaps.
func testEverforestPalettes() []Palette {
	return []Palette{
		{
			Name:       "everforest-dark",
			Background: "#2d353b",
			Text:       "#d3c6aa", Strong: "#d3c6aa", Muted: "#9da9a0", Rule: "#475258",
			OK: "#a7c080", Warn: "#dbbc7f", Fail: "#e67e80",
			Key: "#e69875",
		},
		{
			Name:       "everforest-light",
			Background: "#fdf6e3",
			Text:       "#5c6a72", Strong: "#5c6a72", Muted: "#5c6a72", Rule: "#e6e2cc",
			OK: "#5c6a72", Warn: "#5c6a72", Fail: "#5c6a72",
			Key: "#5c6a72",
		},
	}
}

// everforestSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var everforestSwaps = []struct {
	palette, token, from, fromName string
}{
	{"everforest-dark", "muted", "#859289", "grey1"},  // 3.84:1 -> grey2 #9da9a0 5.12:1
	{"everforest-light", "muted", "#939f91", "grey1"}, // 2.56:1 -> fg #5c6a72 5.18:1
	{"everforest-light", "ok", "#8da101", "green"},    // 2.69:1 -> fg #5c6a72 5.18:1
	{"everforest-light", "warn", "#dfa000", "yellow"}, // 2.12:1 -> fg #5c6a72 5.18:1
	{"everforest-light", "fail", "#f85552", "red"},    // 3.04:1 -> fg #5c6a72 5.18:1
	{"everforest-light", "key", "#f57d26", "orange"},  // 2.48:1 -> fg #5c6a72 5.18:1
}

func TestEverforestPalettesMatchUpstream(t *testing.T) {
	for _, want := range testEverforestPalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestEverforestTokensReachAAContrast(t *testing.T) {
	// rule is decorative and exempt.
	for _, want := range testEverforestPalettes() {
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

func TestEverforestSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed 4.5:1, and
	// the palette no longer carries it.
	for _, sw := range everforestSwaps {
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

func TestEverforestFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"everforest", true, "everforest-dark"},
		{"everforest", false, "everforest-light"},
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
