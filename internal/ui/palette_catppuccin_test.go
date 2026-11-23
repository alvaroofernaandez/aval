package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// Upstream: https://github.com/catppuccin/palette (palette.json)
// Hex values are pinned here. Text tokens need 4.5:1 and swap only to a
// neutral entry; state tokens need 3:1 and fall back to strong. Each swap is listed in catppuccinSwaps.
func testCatppuccinPalettes() []Palette {
	return []Palette{
		{
			Name:       "catppuccin-mocha",
			Background: "#1e1e2e",
			Text:       "#cdd6f4", Strong: "#cdd6f4", Muted: "#a6adc8", Rule: "#45475a",
			OK: "#a6e3a1", Warn: "#f9e2af", Fail: "#f38ba8",
			Key: "#fab387",
		},
		{
			Name:       "catppuccin-latte",
			Background: "#eff1f5",
			Text:       "#4c4f69", Strong: "#4c4f69", Muted: "#5c5f77", Rule: "#bcc0cc",
			OK: "#4c4f69", Warn: "#4c4f69", Fail: "#d20f39",
			Key: "#5c5f77",
		},
		{
			Name:       "catppuccin-frappe",
			Background: "#303446",
			Text:       "#c6d0f5", Strong: "#c6d0f5", Muted: "#a5adce", Rule: "#51576d",
			OK: "#a6d189", Warn: "#e5c890", Fail: "#e78284",
			Key: "#ef9f76",
		},
		{
			Name:       "catppuccin-macchiato",
			Background: "#24273a",
			Text:       "#cad3f5", Strong: "#cad3f5", Muted: "#a5adcb", Rule: "#494d64",
			OK: "#a6da95", Warn: "#eed49f", Fail: "#ed8796",
			Key: "#f5a97f",
		},
	}
}

// catppuccinSwaps records every token replaced for contrast: the upstream entry that
// failed, and the entry of the same palette used instead.
var catppuccinSwaps = []struct {
	palette, token, from, fromName string
}{
	{"catppuccin-latte", "muted", "#6c6f85", "subtext0"}, // 4.37:1 -> subtext1 #5c5f77 5.53:1
	{"catppuccin-latte", "ok", "#40a02b", "green"},       // 2.96:1 -> fallback: strong
	{"catppuccin-latte", "warn", "#df8e1d", "yellow"},    // 2.31:1 -> fallback: strong
	{"catppuccin-latte", "key", "#fe640b", "peach"},      // 2.64:1 -> subtext1 #5c5f77 5.53:1
}

func TestCatppuccinPalettesMatchUpstream(t *testing.T) {
	for _, want := range testCatppuccinPalettes() {
		got, err := LookupPalette(want.Name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", want.Name, err)
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", want.Name, got, want)
		}
	}
}

func TestCatppuccinSwapsWereNeeded(t *testing.T) {
	// A swap is justified only if the upstream entry really missed its contrast floor, and
	// the palette no longer carries it.
	for _, sw := range catppuccinSwaps {
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

func TestCatppuccinFamiliesResolve(t *testing.T) {
	for _, c := range []struct {
		name string
		dark bool
		want string
	}{
		{"catppuccin", true, "catppuccin-mocha"},
		{"catppuccin", false, "catppuccin-latte"},
		{"catppuccin-frappe", true, "catppuccin-frappe"},
		{"catppuccin-frappe", false, "catppuccin-frappe"},
		{"catppuccin-macchiato", true, "catppuccin-macchiato"},
		{"catppuccin-macchiato", false, "catppuccin-macchiato"},
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
