package ui

// Upstream: https://github.com/folke/tokyonight.nvim (extras/lua/tokyonight_<variant>.lua)
// Hex values come from the upstream repository. Text tokens need 4.5:1 and are
// swapped only to a neutral entry. State tokens need 3:1 (glyph only) and keep
// their upstream hex or fall back to strong. Each swap is noted on its line and
// pinned in the test.

func init() {
	register(Family{Name: "tokyo-night", Dark: "tokyo-night", Light: "tokyo-night-day"},
		Palette{
			Name: "tokyo-night",
			// muted: upstream comment #565f89 is 2.76:1; fg_dark used instead.
			Background: "#1a1b26",
			Text:       "#c0caf5", Strong: "#c0caf5", Muted: "#a9b1d6", Rule: "#3b4261",
			OK: "#9ece6a", Warn: "#e0af68", Fail: "#f7768e",
			Key: "#ff9e64",
		},
		Palette{
			Name: "tokyo-night-day",
			// muted: upstream comment #848cb5 is 2.54:1; fg used instead.
			// key: upstream orange #b15c00 is 3.69:1; fg used instead.
			Background: "#e1e2e7",
			Text:       "#3760bf", Strong: "#3760bf", Muted: "#3760bf", Rule: "#a8aecb",
			OK: "#587539", Warn: "#8c6c3e", Fail: "#f52a65",
			Key: "#3760bf",
		},
	)
	register(Family{Name: "tokyo-night-storm", Dark: "tokyo-night-storm", Light: "tokyo-night-storm"},
		Palette{
			Name: "tokyo-night-storm",
			// muted: upstream comment #565f89 is 2.35:1; fg_dark used instead.
			Background: "#24283b",
			Text:       "#c0caf5", Strong: "#c0caf5", Muted: "#a9b1d6", Rule: "#3b4261",
			OK: "#9ece6a", Warn: "#e0af68", Fail: "#f7768e",
			Key: "#ff9e64",
		},
	)
}
