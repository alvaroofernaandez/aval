package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/joshdick/onedark.vim (autoload/onedark.vim); One Light from https://github.com/navarasu/onedark.nvim (lua/onedark/palette.lua)
// Hex values are pinned here. Text tokens need 4.5:1 and swap only to a
// neutral entry; state tokens need 3:1 and fall back to strong. Each swap is listed in oneSwaps.
func testOnePalettes() []Palette {
	return []Palette{
		{
			Name:       "one-dark",
			Background: "#282c34",
			Text:       "#abb2bf", Strong: "#abb2bf", Muted: "#abb2bf", Rule: "#3e4452",
			OK: "#98c379", Warn: "#e5c07b", Fail: "#e06c75",
			Key: "#61afef",
		},
		{
			Name:       "one-light",
			Background: "#fafafa",
			Text:       "#383a42", Strong: "#383a42", Muted: "#383a42", Rule: "#dcdcdc",
			OK: "#50a14f", Warn: "#986801", Fail: "#e45649",
			Key: "#383a42",
		},
	}
}

// oneSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var oneSwaps = []struct {
	palette, token, from, fromName string
}{
	{"one-dark", "muted", "#5c6370", "comment_grey"}, // 2.32:1 -> white #abb2bf 6.57:1
	{"one-light", "muted", "#a0a1a7", "grey"},        // 2.47:1 -> fg #383a42 10.86:1
	{"one-light", "key", "#c18401", "orange"},        // 3.06:1 -> fg #383a42 10.86:1
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

func TestOneSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed its contrast floor, and
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
		if r >= contrastFloor(sw.token) {
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
