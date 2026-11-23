package ui

// Upstream: https://github.com/rebelot/kanagawa.nvim (lua/kanagawa/colors.lua)
// Hex values come from the upstream repository. A token that missed 4.5:1 on its
// own background was replaced by the closest entry of the same palette that
// passes; each swap is noted on its line and pinned in the test.

func init() {
	register(Family{Name: "kanagawa", Dark: "kanagawa-wave", Light: "kanagawa-lotus"},
		Palette{
			Name: "kanagawa-wave",
			// muted: upstream fujiGray #727169 is 3.33:1; oniViolet2 used instead.
			Background: "#1f1f28",
			Text:       "#dcd7ba", Strong: "#dcd7ba", Muted: "#b8b4d0", Rule: "#363646",
			OK: "#98bb6c", Warn: "#e6c384", Fail: "#e46876",
			Key: "#ffa066",
		},
		Palette{
			Name: "kanagawa-lotus",
			// muted: upstream lotusGray2 #716e61 is 4.26:1; lotusInk1 used instead.
			// ok: upstream lotusGreen #6f894e is 3.26:1; lotusInk1 used instead.
			// warn: upstream lotusYellow #77713f is 4.15:1; lotusViolet4 used instead.
			// fail: upstream lotusRed #c84053 is 4.06:1; lotusInk2 used instead.
			// key: upstream lotusOrange #cc6d00 is 3.04:1; lotusBlue5 used instead.
			Background: "#f2ecbc",
			Text:       "#545464", Strong: "#545464", Muted: "#545464", Rule: "#e4d794",
			OK: "#545464", Warn: "#624c83", Fail: "#43436c",
			Key: "#5d57a3",
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
