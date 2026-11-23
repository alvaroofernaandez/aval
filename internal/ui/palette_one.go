package ui

// Upstream: https://github.com/joshdick/onedark.vim (autoload/onedark.vim); One Light from https://github.com/navarasu/onedark.nvim (lua/onedark/palette.lua)
// Hex values come from the upstream repository. Text tokens need 4.5:1 and are
// swapped only to a neutral entry. State tokens need 3:1 (glyph only) and keep
// their upstream hex or fall back to strong. Each swap is noted on its line and
// pinned in the test.

func init() {
	register(Family{Name: "one-dark", Dark: "one-dark", Light: "one-dark"},
		Palette{
			Name: "one-dark",
			// muted: upstream comment_grey #5c6370 is 2.32:1; white used instead.
			Background: "#282c34",
			Text:       "#abb2bf", Strong: "#abb2bf", Muted: "#abb2bf", Rule: "#3e4452",
			OK: "#98c379", Warn: "#e5c07b", Fail: "#e06c75",
			Key: "#61afef",
		},
	)
	register(Family{Name: "one-light", Dark: "one-light", Light: "one-light"},
		Palette{
			Name: "one-light",
			// muted: upstream grey #a0a1a7 is 2.47:1; fg used instead.
			// key: upstream orange #c18401 is 3.06:1; fg used instead.
			Background: "#fafafa",
			Text:       "#383a42", Strong: "#383a42", Muted: "#383a42", Rule: "#dcdcdc",
			OK: "#50a14f", Warn: "#986801", Fail: "#e45649",
			Key: "#383a42",
		},
	)
}
