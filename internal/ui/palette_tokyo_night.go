package ui

// Upstream: https://github.com/folke/tokyonight.nvim (extras/lua/tokyonight_<variant>.lua)
// Hex values come from the upstream repository. A token that missed 4.5:1 on its
// own background was replaced by the closest entry of the same palette that
// passes; each swap is noted on its line and pinned in the test.

func init() {
	register(Family{Name: "tokyo-night", Dark: "tokyo-night", Light: "tokyo-night-day"},
		Palette{
			Name: "tokyo-night",
			// muted: upstream comment #565f89 is 2.76:1; green2 used instead.
			Background: "#1a1b26",
			Text:       "#c0caf5", Strong: "#c0caf5", Muted: "#41a6b5", Rule: "#3b4261",
			OK: "#9ece6a", Warn: "#e0af68", Fail: "#f7768e",
			Key: "#ff9e64",
		},
		Palette{
			Name: "tokyo-night-day",
			// muted: upstream comment #848cb5 is 2.54:1; fg used instead.
			// ok: upstream green #587539 is 4.04:1; blue6 used instead.
			// warn: upstream yellow #8c6c3e is 3.75:1; purple used instead.
			// fail: upstream red #f52a65 is 3.01:1; fg used instead.
			// key: upstream orange #b15c00 is 3.69:1; blue5 used instead.
			Background: "#e1e2e7",
			Text:       "#3760bf", Strong: "#3760bf", Muted: "#3760bf", Rule: "#a8aecb",
			OK: "#2e5857", Warn: "#7847bd", Fail: "#3760bf",
			Key: "#006a83",
		},
	)
	register(Family{Name: "tokyo-night-storm", Dark: "tokyo-night-storm", Light: "tokyo-night-storm"},
		Palette{
			Name: "tokyo-night-storm",
			// muted: upstream comment #565f89 is 2.35:1; green2 used instead.
			Background: "#24283b",
			Text:       "#c0caf5", Strong: "#c0caf5", Muted: "#41a6b5", Rule: "#3b4261",
			OK: "#9ece6a", Warn: "#e0af68", Fail: "#f7768e",
			Key: "#ff9e64",
		},
	)
}
