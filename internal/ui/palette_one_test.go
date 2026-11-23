package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/joshdick/onedark.vim (autoload/onedark.vim); One Light from https://github.com/navarasu/onedark.nvim (lua/onedark/palette.lua)
// Hex values are pinned here. A token that missed 4.5:1 on its own background
// was replaced by the closest entry of the same palette that passes; each
// swap is listed in oneSwaps.
func testOnePalettes() []Palette {
	return []Palette{
		{
			Name:       "one-dark",
			Background: "#282c34",
			Text:       "#abb2bf", Strong: "#abb2bf", Muted: "#56b6c2", Rule: "#3e4452",
			OK: "#98c379", Warn: "#e5c07b", Fail: "#d19a66",
			Key: "#61afef",
		},
		{
			Name:       "one-light",
			Background: "#fafafa",
			Text:       "#383a42", Strong: "#383a42", Muted: "#79428a", Rule: "#dcdcdc",
			OK: "#101012", Warn: "#986801", Fail: "#833b3b",
			Key: "#7c5c20",
		},
	}
}

// oneSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var oneSwaps = []struct {
	palette, token, from, fromName string
}{
	{"one-dark", "muted", "#5c6370", "comment_grey"}, // 2.32:1 -> cyan #56b6c2 5.91:1
	{"one-dark", "fail", "#e06c75", "red"},           // 4.38:1 -> dark_yellow #d19a66 5.68:1
	{"one-light", "muted", "#a0a1a7", "grey"},        // 2.47:1 -> dark_purple #79428a 6.80:1
	{"one-light", "ok", "#50a14f", "green"},          // 3.07:1 -> black #101012 18.21:1
	{"one-light", "fail", "#e45649", "red"},          // 3.51:1 -> dark_red #833b3b 7.58:1
	{"one-light", "key", "#c18401", "orange"},        // 3.06:1 -> dark_yellow #7c5c20 5.90:1
}

func TestOnePalettesMatchUpstream(t *testing.T) {
	for _, want := range testOnePalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestOneTokensReachAAContrast(t *testing.T) {
	// rule is decorative and exempt.
	for _, want := range testOnePalettes() {
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

func TestOneSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed 4.5:1, and
	// the palette no longer carries it.
	for _, sw := range oneSwaps {
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

func TestOneFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"one-dark", true, "one-dark"},
		{"one-dark", false, "one-dark"},
		{"one-light", true, "one-light"},
		{"one-light", false, "one-light"},
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
