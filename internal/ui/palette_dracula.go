package ui

// Upstream: https://github.com/dracula/dracula-theme (README.md colour tables)
// Hex values come from the upstream repository. Text tokens need 4.5:1 and are
// swapped only to a neutral entry. State tokens need 3:1 (glyph only) and keep
// their upstream hex or fall back to strong. Each swap is noted on its line and
// pinned in the test.

func init() {
	register(Family{Name: "dracula", Dark: "dracula", Light: "dracula-alucard"},
		Palette{
			Name: "dracula",
			// muted: upstream comment #6272a4 is 3.03:1; foreground used instead.
			Background: "#282a36",
			Text:       "#f8f8f2", Strong: "#f8f8f2", Muted: "#f8f8f2", Rule: "#44475a",
			OK: "#50fa7b", Warn: "#f1fa8c", Fail: "#ff5555",
			Key: "#ffb86c",
		},
		Palette{
			Name:       "dracula-alucard",
			Background: "#fffbeb",
			Text:       "#1f1f1f", Strong: "#1f1f1f", Muted: "#6c664b", Rule: "#cfcfde",
			OK: "#14710a", Warn: "#846e15", Fail: "#cb3a2a",
			Key: "#a34d14",
		},
	)
}
