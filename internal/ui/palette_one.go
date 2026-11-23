package ui

// Upstream: https://github.com/joshdick/onedark.vim (autoload/onedark.vim); One Light from https://github.com/navarasu/onedark.nvim (lua/onedark/palette.lua)
// Hex values come from the upstream repository. A token that missed 4.5:1 on its
// own background was replaced by the closest entry of the same palette that
// passes; each swap is noted on its line and pinned in the test.

func init() {
	register(Family{Name: "one-dark", Dark: "one-dark", Light: "one-dark"},
		Palette{
			Name: "one-dark",
			// muted: upstream comment_grey #5c6370 is 2.32:1; cyan used instead.
			// fail: upstream red #e06c75 is 4.38:1; dark_yellow used instead.
			Background: "#282c34",
			Text:       "#abb2bf", Strong: "#abb2bf", Muted: "#56b6c2", Rule: "#3e4452",
			OK: "#98c379", Warn: "#e5c07b", Fail: "#d19a66",
			Key: "#61afef",
		},
	)
	register(Family{Name: "one-light", Dark: "one-light", Light: "one-light"},
		Palette{
			Name: "one-light",
			// muted: upstream grey #a0a1a7 is 2.47:1; dark_purple used instead.
			// ok: upstream green #50a14f is 3.07:1; black used instead.
			// fail: upstream red #e45649 is 3.51:1; dark_red used instead.
			// key: upstream orange #c18401 is 3.06:1; dark_yellow used instead.
			Background: "#fafafa",
			Text:       "#383a42", Strong: "#383a42", Muted: "#79428a", Rule: "#dcdcdc",
			OK: "#101012", Warn: "#986801", Fail: "#833b3b",
			Key: "#7c5c20",
		},
	)
}
