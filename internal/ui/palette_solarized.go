package ui

// Upstream: https://github.com/altercation/solarized (README.md palette table)
// Hex values come from the upstream repository. A token that missed 4.5:1 on its
// own background was replaced by the closest entry of the same palette that
// passes; each swap is noted on its line and pinned in the test.

func init() {
	register(Family{Name: "solarized", Dark: "solarized-dark", Light: "solarized-light"},
		Palette{
			Name: "solarized-dark",
			// muted: upstream base01 #586e75 is 2.79:1; base0 used instead.
			// fail: upstream red #dc322f is 3.25:1; base0 used instead.
			// key: upstream orange #cb4b16 is 3.26:1; base1 used instead.
			Background: "#002b36",
			Text:       "#839496", Strong: "#93a1a1", Muted: "#839496", Rule: "#073642",
			OK: "#859900", Warn: "#b58900", Fail: "#839496",
			Key: "#93a1a1",
		},
		Palette{
			Name: "solarized-light",
			// text: upstream base00 #657b83 is 4.13:1; base01 used instead.
			// muted: upstream base1 #93a1a1 is 2.48:1; base01 used instead.
			// ok: upstream green #859900 is 2.97:1; base01 used instead.
			// warn: upstream yellow #b58900 is 2.98:1; base02 used instead.
			// fail: upstream red #dc322f is 4.29:1; base03 used instead.
			// key: upstream orange #cb4b16 is 4.27:1; base01 used instead.
			Background: "#fdf6e3",
			Text:       "#586e75", Strong: "#586e75", Muted: "#586e75", Rule: "#eee8d5",
			OK: "#586e75", Warn: "#073642", Fail: "#002b36",
			Key: "#586e75",
		},
	)
}
