package ui

// Upstream: https://github.com/nordtheme/nord (src/nord.css)
// Hex values come from the upstream repository. Text tokens need 4.5:1 and are
// swapped only to a neutral entry. State tokens need 3:1 (glyph only) and keep
// their upstream hex or fall back to strong. Each swap is noted on its line and
// pinned in the test.

func init() {
	register(Family{Name: "nord", Dark: "nord", Light: "nord"},
		Palette{
			Name: "nord",
			// muted: upstream nord3 #4c566a is 1.69:1; nord4 used instead.
			Background: "#2e3440",
			Text:       "#d8dee9", Strong: "#d8dee9", Muted: "#d8dee9", Rule: "#434c5e",
			OK: "#a3be8c", Warn: "#ebcb8b", Fail: "#bf616a",
			Key: "#88c0d0",
		},
	)
}
