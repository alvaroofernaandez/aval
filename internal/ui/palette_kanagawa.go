package ui

// Upstream: https://github.com/rebelot/kanagawa.nvim (lua/kanagawa/colors.lua)
// Hex values come from the upstream repository. Text tokens need 4.5:1 and are
// swapped only to a neutral entry. State tokens need 3:1 (glyph only) and keep
// their upstream hex or fall back to strong. Each swap is noted on its line and
// pinned in the test.

func init() {
	register(Family{Name: "kanagawa", Dark: "kanagawa-wave", Light: "kanagawa-lotus"},
		Palette{
			Name: "kanagawa-wave",
			// muted: upstream fujiGray #727169 is 3.33:1; oldWhite used instead.
			Background: "#1f1f28",
			Text:       "#dcd7ba", Strong: "#dcd7ba", Muted: "#c8c093", Rule: "#363646",
			OK: "#98bb6c", Warn: "#e6c384", Fail: "#e46876",
			Key: "#ffa066",
		},
		Palette{
			Name: "kanagawa-lotus",
			// muted: upstream lotusGray2 #716e61 is 4.26:1; lotusInk1 used instead.
			// key: upstream lotusOrange #cc6d00 is 3.04:1; lotusInk1 used instead.
			Background: "#f2ecbc",
			Text:       "#545464", Strong: "#545464", Muted: "#545464", Rule: "#e4d794",
			OK: "#6f894e", Warn: "#77713f", Fail: "#c84053",
			Key: "#545464",
		},
	)
	register(Family{Name: "kanagawa-dragon", Dark: "kanagawa-dragon", Light: "kanagawa-dragon"},
		Palette{
			Name:       "kanagawa-dragon",
			Background: "#181616",
			Text:       "#c5c9c5", Strong: "#c5c9c5", Muted: "#7a8382", Rule: "#393836",
			OK: "#8a9a7b", Warn: "#c4b28a", Fail: "#c4746e",
			Key: "#b6927b",
		},
	)
}
