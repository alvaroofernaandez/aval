package ui

// Upstream: https://github.com/sainnhe/everforest (autoload/everforest.vim, medium contrast)
// Hex values come from the upstream repository. A token that missed 4.5:1 on its
// own background was replaced by the closest entry of the same palette that
// passes; each swap is noted on its line and pinned in the test.

func init() {
	register(Family{Name: "everforest", Dark: "everforest-dark", Light: "everforest-light"},
		Palette{
			Name: "everforest-dark",
			// muted: upstream grey1 #859289 is 3.84:1; grey2 used instead.
			Background: "#2d353b",
			Text:       "#d3c6aa", Strong: "#d3c6aa", Muted: "#9da9a0", Rule: "#475258",
			OK: "#a7c080", Warn: "#dbbc7f", Fail: "#e67e80",
			Key: "#e69875",
		},
		Palette{
			Name: "everforest-light",
			// muted: upstream grey1 #939f91 is 2.56:1; fg used instead.
			// ok: upstream green #8da101 is 2.69:1; fg used instead.
			// warn: upstream yellow #dfa000 is 2.12:1; fg used instead.
			// fail: upstream red #f85552 is 3.04:1; fg used instead.
			// key: upstream orange #f57d26 is 2.48:1; fg used instead.
			Background: "#fdf6e3",
			Text:       "#5c6a72", Strong: "#5c6a72", Muted: "#5c6a72", Rule: "#e6e2cc",
			OK: "#5c6a72", Warn: "#5c6a72", Fail: "#5c6a72",
			Key: "#5c6a72",
		},
	)
}
