package ui

// Upstream: https://github.com/rose-pine/neovim (lua/rose-pine/palette.lua)
// Hex values come from the upstream repository. Text tokens need 4.5:1 and are
// swapped only to a neutral entry. State tokens need 3:1 (glyph only) and keep
// their upstream hex or fall back to strong. Each swap is noted on its line and
// pinned in the test.

func init() {
	register(Family{Name: "rose-pine", Dark: "rose-pine", Light: "rose-pine-dawn"},
		Palette{
			Name:       "rose-pine",
			Background: "#191724",
			Text:       "#e0def4", Strong: "#e0def4", Muted: "#908caa", Rule: "#403d52",
			OK: "#31748f", Warn: "#f6c177", Fail: "#eb6f92",
			Key: "#ebbcba",
		},
		Palette{
			Name: "rose-pine-dawn",
			// muted: upstream subtle #797593 is 4.02:1; text used instead.
			// warn: upstream gold #ea9d34 is 2.05:1; fallback: strong (glyph drawn in strong, bold).
			// key: upstream rose #d7827e is 2.60:1; text used instead.
			Background: "#faf4ed",
			Text:       "#464261", Strong: "#464261", Muted: "#464261", Rule: "#dfdad9",
			OK: "#286983", Warn: "#464261", Fail: "#b4637a",
			Key: "#464261",
		},
	)
	register(Family{Name: "rose-pine-moon", Dark: "rose-pine-moon", Light: "rose-pine-moon"},
		Palette{
			Name:       "rose-pine-moon",
			Background: "#232136",
			Text:       "#e0def4", Strong: "#e0def4", Muted: "#908caa", Rule: "#44415a",
			OK: "#3e8fb0", Warn: "#f6c177", Fail: "#eb6f92",
			Key: "#ea9a97",
		},
	)
}
