package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/dracula/dracula-theme (README.md colour tables)
// Hex values are pinned here. Text tokens need 4.5:1 and swap only to a
// neutral entry; state tokens need 3:1 and fall back to strong. Each swap is listed in draculaSwaps.
func testDraculaPalettes() []Palette {
	return []Palette{
		{
			Name:       "dracula",
			Background: "#282a36",
			Text:       "#f8f8f2", Strong: "#f8f8f2", Muted: "#f8f8f2", Rule: "#44475a",
			OK: "#50fa7b", Warn: "#f1fa8c", Fail: "#ff5555",
			Key: "#ffb86c",
		},
		{
			Name:       "dracula-alucard",
			Background: "#fffbeb",
			Text:       "#1f1f1f", Strong: "#1f1f1f", Muted: "#6c664b", Rule: "#cfcfde",
			OK: "#14710a", Warn: "#846e15", Fail: "#cb3a2a",
			Key: "#a34d14",
		},
	}
}

// draculaSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var draculaSwaps = []struct {
	palette, token, from, fromName string
}{
	{"dracula", "muted", "#6272a4", "comment"}, // 3.03:1 -> foreground #f8f8f2 13.36:1
}

func TestDraculaPalettesMatchUpstream(t *testing.T) {
	for _, want := range testDraculaPalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestDraculaSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed its contrast floor, and
	// the palette no longer carries it.
	for _, sw := range draculaSwaps {
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

func TestDraculaFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"dracula", true, "dracula"},
		{"dracula", false, "dracula-alucard"},
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
