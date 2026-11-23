package ui

// Upstream: https://github.com/morhetz/gruvbox (colors/gruvbox.vim)
// Hex values come from the upstream repository. Text tokens need 4.5:1 and are
// swapped only to a neutral entry. State tokens need 3:1 (glyph only) and keep
// their upstream hex or fall back to strong. Each swap is noted on its line and
// pinned in the test.

func init() {
	register(Family{Name: "gruvbox", Dark: "gruvbox-dark", Light: "gruvbox-light"},
		Palette{
			Name: "gruvbox-dark",
			// muted: upstream gray_245 #928374 is 4.02:1; light4 used instead.
			Background: "#282828",
			Text:       "#ebdbb2", Strong: "#ebdbb2", Muted: "#a89984", Rule: "#504945",
			OK: "#b8bb26", Warn: "#fabd2f", Fail: "#fb4934",
			Key: "#83a598",
		},
		Palette{
			Name: "gruvbox-light",
			// muted: upstream dark4 #7c6f64 is 4.29:1; dark3 used instead.
			Background: "#fbf1c7",
			Text:       "#3c3836", Strong: "#3c3836", Muted: "#665c54", Rule: "#d5c4a1",
			OK: "#79740e", Warn: "#b57614", Fail: "#9d0006",
			Key: "#af3a03",
		},
	)
}
