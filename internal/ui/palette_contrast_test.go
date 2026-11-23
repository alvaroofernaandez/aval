package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
)

// DESIGN.md section 10, "State colour rule (all themes)":
//   - text tokens carry words and need 4.5:1;
//   - state tokens colour a glyph only (the word uses text) and need 3:1;
//   - a state token keeps its hue, or falls back to strong (drawn bold);
//   - ok, warn and fail stay distinct unless they fell back to strong.

const (
	textFloor  = 4.5
	stateFloor = 3.0
)

func isStateToken(token string) bool {
	return token == "ok" || token == "warn" || token == "fail"
}

// contrastFloor is the minimum ratio a token must reach on its background.
func contrastFloor(token string) float64 {
	if isStateToken(token) {
		return stateFloor
	}
	return textFloor
}

// colourPalettes is every registered palette that paints real colours, so not
// terminal (ANSI) or mono (attributes only).
func colourPalettes() []Palette {
	var out []Palette
	for _, n := range ThemeNames() {
		if p, ok := palettes[n]; ok && strings.HasPrefix(p.Background, "#") {
			out = append(out, p)
		}
	}
	return out
}

// swapReason documents one token that differs from its upstream entry.
type swapReason struct {
	upstreamName, upstreamHex string
	chosenName, chosenHex     string // chosenHex is empty for a fallback
	neutral                   bool   // text token swapped to a neutral entry
	fallback                  bool   // state token drawn in strong, bold
	reason                    string
}

func textSwap(upName, upHex, name, hex, why string) swapReason {
	return swapReason{upstreamName: upName, upstreamHex: upHex, chosenName: name, chosenHex: hex, neutral: true, reason: why}
}

func fallbackStrong(upName, upHex, why string) swapReason {
	return swapReason{upstreamName: upName, upstreamHex: upHex, chosenName: "strong", fallback: true, reason: "fallback: strong. " + why}
}

// expectedSwaps is palette -> token -> upstream name -> chosen name -> reason.
// A token absent from here must equal its upstream entry. No state token is
// swapped to another colour: a state either keeps its upstream hex or falls
// back to strong.
var expectedSwaps = map[string]map[string]swapReason{
	"catppuccin-latte": {
		"muted": textSwap("subtext0", "#6c6f85", "subtext1", "#5c5f77", "subtext0 is 4.37:1; subtext1 is the next neutral"),
		"key":   textSwap("peach", "#fe640b", "subtext1", "#5c5f77", "peach is 2.64:1; neutral subtext1"),
		"ok":    fallbackStrong("green", "#40a02b", "green is 2.96:1. Teal #179299 passes but catppuccin does not name it a green variant, so it is a different hue."),
		"warn":  fallbackStrong("yellow", "#df8e1d", "yellow is 2.31:1; no other yellow entry exists (peach is orange)."),
	},
	"everforest-dark": {
		"muted": textSwap("grey1", "#859289", "grey2", "#9da9a0", "grey1 is 3.84:1; grey2 is 5.12:1"),
	},
	"everforest-light": {
		"muted": textSwap("grey1", "#939f91", "fg", "#5c6a72", "grey1 is 2.56:1 and grey2 is 3.08:1; fg is the neutral that passes"),
		"key":   textSwap("orange", "#f57d26", "fg", "#5c6a72", "orange is 2.48:1; neutral fg"),
		"ok":    fallbackStrong("green", "#8da101", "green is 2.69:1 and aqua is another hue."),
		"warn":  fallbackStrong("yellow", "#dfa000", "yellow is 2.12:1 and orange (2.48:1) is another hue."),
	},
	"gruvbox-dark": {
		"muted": textSwap("gray_245", "#928374", "light4", "#a89984", "gray_245 is 4.02:1; light4 is 5.30:1"),
	},
	"gruvbox-light": {
		"muted": textSwap("dark4", "#7c6f64", "dark3", "#665c54", "dark4 is 4.29:1; dark3 is 5.74:1"),
	},
	"kanagawa-wave": {
		"muted": textSwap("fujiGray", "#727169", "oldWhite", "#c8c093", "fujiGray is 3.33:1; oldWhite is the neutral that passes (8.89:1)"),
	},
	"kanagawa-lotus": {
		"muted": textSwap("lotusGray2", "#716e61", "lotusInk1", "#545464", "lotusGray2 is 4.26:1; lotusInk1 is 6.19:1"),
		"key":   textSwap("lotusOrange", "#cc6d00", "lotusInk1", "#545464", "lotusOrange is 3.04:1; neutral lotusInk1"),
	},
	"nord": {
		"muted": textSwap("nord3", "#4c566a", "nord4", "#d8dee9", "nord3 is 1.69:1; nord4 is the only neutral that passes"),
	},
	"one-dark": {
		"muted": textSwap("comment_grey", "#5c6370", "white", "#abb2bf", "comment_grey is 2.32:1; white (the foreground) is the only neutral that passes"),
	},
	"one-light": {
		"muted": textSwap("grey", "#a0a1a7", "fg", "#383a42", "grey is 2.47:1 and light_grey 3.64:1; fg is the neutral that passes"),
		"key":   textSwap("orange", "#c18401", "fg", "#383a42", "orange is 3.06:1; neutral fg"),
	},
	"rose-pine-dawn": {
		"muted": textSwap("subtle", "#797593", "text", "#464261", "subtle is 4.02:1; text is the neutral that passes"),
		"key":   textSwap("rose", "#d7827e", "text", "#464261", "rose is 2.60:1; neutral text"),
		"warn":  fallbackStrong("gold", "#ea9d34", "gold is 2.05:1 and no other entry is yellow."),
	},
	"dracula": {
		"muted": textSwap("comment", "#6272a4", "foreground", "#f8f8f2", "comment is 3.03:1; foreground is the only neutral that passes"),
	},
	"solarized-dark": {
		"muted": textSwap("base01", "#586e75", "base0", "#839496", "base01 is 2.79:1; base0 is 4.75:1"),
		"key":   textSwap("orange", "#cb4b16", "base1", "#93a1a1", "orange is 3.26:1; neutral base1"),
	},
	"solarized-light": {
		"text":  textSwap("base00", "#657b83", "base01", "#586e75", "base00 is 4.13:1; base01 is 4.99:1"),
		"muted": textSwap("base1", "#93a1a1", "base01", "#586e75", "base1 is 2.48:1; base01 is 4.99:1"),
		"key":   textSwap("orange", "#cb4b16", "base01", "#586e75", "orange is 4.27:1; neutral base01"),
		"ok":    fallbackStrong("green", "#859900", "green is 2.97:1 and cyan is another hue."),
		"warn":  fallbackStrong("yellow", "#b58900", "yellow is 2.98:1 and orange is another hue."),
	},
	"tokyo-night": {
		"muted": textSwap("comment", "#565f89", "fg_dark", "#a9b1d6", "comment is 2.76:1; fg_dark is the neutral that passes"),
	},
	"tokyo-night-storm": {
		"muted": textSwap("comment", "#565f89", "fg_dark", "#a9b1d6", "comment is 2.35:1; fg_dark is the neutral that passes"),
	},
	"tokyo-night-day": {
		"muted": textSwap("comment", "#848cb5", "fg", "#3760bf", "comment is 2.54:1 and fg_dark 3.57:1; fg is the neutral that passes"),
		"key":   textSwap("orange", "#b15c00", "fg", "#3760bf", "orange is 3.69:1; neutral fg"),
	},
}

func paletteToken(p Palette, token string) string {
	if token == "text" {
		return p.Text
	}
	return p.meaningful()[token]
}

func ratioOrFatal(t *testing.T, fg, bg string) float64 {
	t.Helper()
	r, err := gate.Ratio(fg, bg)
	if err != nil {
		t.Fatalf("Ratio(%s, %s): %v", fg, bg, err)
	}
	return r
}

func TestEveryPaletteMeetsItsContrastFloor(t *testing.T) {
	for _, p := range colourPalettes() {
		for token, fg := range p.meaningful() {
			r := ratioOrFatal(t, fg, p.Background)
			floor := contrastFloor(token)
			switch {
			case !isStateToken(token):
				if r < floor {
					t.Errorf("%s.%s %s on %s = %.2f:1, text tokens need >= %.1f", p.Name, token, fg, p.Background, r, floor)
				}
			case r < floor && fg != p.Strong:
				t.Errorf("%s.%s %s on %s = %.2f:1, state tokens need >= %.1f or the strong fallback", p.Name, token, fg, p.Background, r, floor)
			}
		}
	}
}

func TestStateTokensAreDistinctUnlessFallenBack(t *testing.T) {
	for _, p := range colourPalettes() {
		states := []struct{ name, hex string }{{"ok", p.OK}, {"warn", p.Warn}, {"fail", p.Fail}}
		for i := range states {
			for j := i + 1; j < len(states); j++ {
				a, b := states[i], states[j]
				if a.hex != b.hex {
					continue
				}
				if a.hex == p.Strong {
					continue // both fell back to strong
				}
				t.Errorf("%s: %s and %s share %s without falling back to strong", p.Name, a.name, b.name, a.hex)
			}
		}
	}
}

// hue is the HSL hue in degrees, or -1 for a grey.
func hue(t *testing.T, hex string) float64 {
	t.Helper()
	h := strings.TrimPrefix(hex, "#")
	var c [3]float64
	for i := range c {
		v, err := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
		if err != nil {
			t.Fatalf("bad hex %q: %v", hex, err)
		}
		c[i] = float64(v) / 255
	}
	mx, mn := math.Max(c[0], math.Max(c[1], c[2])), math.Min(c[0], math.Min(c[1], c[2]))
	if mx-mn < 0.04 {
		return -1
	}
	d := mx - mn
	var deg float64
	switch mx {
	case c[0]:
		deg = math.Mod((c[1]-c[2])/d, 6)
	case c[1]:
		deg = (c[2]-c[0])/d + 2
	default:
		deg = (c[0]-c[1])/d + 4
	}
	deg *= 60
	if deg < 0 {
		deg += 360
	}
	return deg
}

func TestStateTokensKeepTheirHue(t *testing.T) {
	// A state is never a different hue: warn is never red, fail is never grey
	// or white. A token that fell back to strong is exempt.
	roles := map[string]func(h float64) bool{
		"ok":   func(h float64) bool { return h >= 50 && h <= 200 },
		"warn": func(h float64) bool { return h >= 15 && h <= 70 },
		"fail": func(h float64) bool { return h >= 335 || (h >= 0 && h <= 25) },
	}
	for _, p := range colourPalettes() {
		for token, inRole := range roles {
			fg := paletteToken(p, token)
			if fg == p.Strong {
				continue
			}
			h := hue(t, fg)
			if h < 0 || !inRole(h) {
				t.Errorf("%s.%s %s has hue %.0f, outside the %s family", p.Name, token, fg, h, token)
			}
		}
	}
}

func TestExpectedSwapsAreDocumentedAndHold(t *testing.T) {
	for name, tokens := range expectedSwaps {
		p, ok := palettes[name]
		if !ok {
			t.Errorf("expectedSwaps names unknown palette %q", name)
			continue
		}
		for token, sw := range tokens {
			where := name + "." + token
			if sw.reason == "" || sw.upstreamName == "" || sw.chosenName == "" {
				t.Errorf("%s: swap lacks an upstream name, a chosen name or a reason: %+v", where, sw)
			}
			got := paletteToken(p, token)
			floor := contrastFloor(token)
			if r := ratioOrFatal(t, sw.upstreamHex, p.Background); r >= floor {
				t.Errorf("%s: upstream %s %s is %.2f:1, it already passes %.1f", where, sw.upstreamName, sw.upstreamHex, r, floor)
			}
			if got == sw.upstreamHex {
				t.Errorf("%s still carries the failing upstream %s", where, sw.upstreamHex)
			}
			switch {
			case isStateToken(token) && !sw.fallback:
				t.Errorf("%s: a state token is only ever swapped to the strong fallback", where)
			case sw.fallback:
				if got != p.Strong {
					t.Errorf("%s: fallback should equal strong %s, got %s", where, p.Strong, got)
				}
			default:
				if got != sw.chosenHex {
					t.Errorf("%s = %s, want %s (%s)", where, got, sw.chosenHex, sw.chosenName)
				}
				if !sw.neutral {
					t.Errorf("%s: a text token may only be swapped to a neutral entry", where)
				}
				if r := ratioOrFatal(t, sw.chosenHex, p.Background); r < floor {
					t.Errorf("%s: chosen %s is %.2f:1, want >= %.1f", where, sw.chosenName, r, floor)
				}
			}
		}
	}
}

func TestEveryFailingUpstreamTokenHasADocumentedSwap(t *testing.T) {
	// The per-theme swap lists are the record of what failed upstream. Every
	// entry there needs a reason here, and the other way round.
	listed := map[string]bool{}
	for _, sw := range allThemeSwaps() {
		key := sw.palette + "." + sw.token
		listed[key] = true
		want, ok := expectedSwaps[sw.palette][sw.token]
		if !ok {
			t.Errorf("%s is swapped in the theme test but has no entry in expectedSwaps", key)
			continue
		}
		if want.upstreamHex != sw.from || want.upstreamName != sw.fromName {
			t.Errorf("%s: theme test says %s %s, expectedSwaps says %s %s", key, sw.fromName, sw.from, want.upstreamName, want.upstreamHex)
		}
	}
	for name, tokens := range expectedSwaps {
		for token := range tokens {
			if !listed[name+"."+token] {
				t.Errorf("%s.%s is in expectedSwaps but missing from the theme swap list", name, token)
			}
		}
	}
}

type themeSwap = struct {
	palette, token, from, fromName string
}

func allThemeSwaps() []themeSwap {
	var out []themeSwap
	for _, l := range [][]themeSwap{
		catppuccinSwaps, draculaSwaps, everforestSwaps, gruvboxSwaps, kanagawaSwaps,
		nordSwaps, oneSwaps, rosePineSwaps, solarizedSwaps, tokyoNightSwaps,
	} {
		out = append(out, l...)
	}
	return out
}
