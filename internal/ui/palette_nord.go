package ui

// Upstream: https://github.com/nordtheme/nord (src/nord.css)
// Hex values come from the upstream repository. A token that missed 4.5:1 on its
// own background was replaced by the closest entry of the same palette that
// passes; each swap is noted on its line and pinned in the test.

func init() {
	register(Family{Name: "nord", Dark: "nord", Light: "nord"},
		Palette{
			Name: "nord",
			// muted: upstream nord3 #4c566a is 1.69:1; nord9 used instead.
			// fail: upstream nord11 #bf616a is 3.05:1; nord4 used instead.
			Background: "#2e3440",
			Text:       "#d8dee9", Strong: "#d8dee9", Muted: "#81a1c1", Rule: "#434c5e",
			OK: "#a3be8c", Warn: "#ebcb8b", Fail: "#d8dee9",
			Key: "#88c0d0",
		},
	)
}
