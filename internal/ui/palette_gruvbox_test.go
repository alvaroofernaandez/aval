package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/morhetz/gruvbox (colors/gruvbox.vim)
// Hex values are pinned here. Text tokens need 4.5:1 and swap only to a
// neutral entry; state tokens need 3:1 and fall back to strong. Each swap is listed in gruvboxSwaps.
func testGruvboxPalettes() []Palette {
	return []Palette{
		{
			Name:       "gruvbox-dark",
			Background: "#282828",
			Text:       "#ebdbb2", Strong: "#ebdbb2", Muted: "#a89984", Rule: "#504945",
			OK: "#b8bb26", Warn: "#fabd2f", Fail: "#fb4934",
			Key: "#83a598",
		},
		{
			Name:       "gruvbox-light",
			Background: "#fbf1c7",
			Text:       "#3c3836", Strong: "#3c3836", Muted: "#665c54", Rule: "#d5c4a1",
			OK: "#79740e", Warn: "#b57614", Fail: "#9d0006",
			Key: "#af3a03",
		},
	}
}

// gruvboxSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var gruvboxSwaps = []struct {
	palette, token, from, fromName string
}{
	{"gruvbox-dark", "muted", "#928374", "gray_245"}, // 4.02:1 -> light4 #a89984 5.30:1
	{"gruvbox-light", "muted", "#7c6f64", "dark4"},   // 4.29:1 -> dark3 #665c54 5.74:1
}

func TestGruvboxPalettesMatchUpstream(t *testing.T) {
	for _, want := range testGruvboxPalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestGruvboxSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed its contrast floor, and
	// the palette no longer carries it.
	for _, sw := range gruvboxSwaps {
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

func TestGruvboxFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"gruvbox", true, "gruvbox-dark"},
		{"gruvbox", false, "gruvbox-light"},
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
