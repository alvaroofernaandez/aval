package ui

// Upstream: https://github.com/morhetz/gruvbox (colors/gruvbox.vim)
// Hex values come from the upstream repository. A token that missed 4.5:1 on its
// own background was replaced by the closest entry of the same palette that
// passes; each swap is noted on its line and pinned in the test.

func init() {
	register(Family{Name: "gruvbox", Dark: "gruvbox-dark", Light: "gruvbox-light"},
		Palette{
			Name: "gruvbox-dark",
			// muted: upstream gray_245 #928374 is 4.02:1; light4 used instead.
			// fail: upstream bright_red #fb4934 is 4.29:1; bright_orange used instead.
			Background: "#282828",
			Text:       "#ebdbb2", Strong: "#ebdbb2", Muted: "#a89984", Rule: "#504945",
			OK: "#b8bb26", Warn: "#fabd2f", Fail: "#fe8019",
			Key: "#83a598",
		},
		Palette{
			Name: "gruvbox-light",
			// muted: upstream dark4 #7c6f64 is 4.29:1; dark3 used instead.
			// ok: upstream faded_green #79740e is 4.29:1; dark3 used instead.
			// warn: upstream faded_yellow #b57614 is 3.33:1; faded_orange used instead.
			Background: "#fbf1c7",
			Text:       "#3c3836", Strong: "#3c3836", Muted: "#665c54", Rule: "#d5c4a1",
			OK: "#665c54", Warn: "#af3a03", Fail: "#9d0006",
			Key: "#af3a03",
		},
	)
}
