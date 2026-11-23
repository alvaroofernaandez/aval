package ui

// Upstream: https://github.com/catppuccin/palette (palette.json)
// Hex values come from the upstream repository. A token that missed 4.5:1 on its
// own background was replaced by the closest entry of the same palette that
// passes; each swap is noted on its line and pinned in the test.

func init() {
	register(Family{Name: "catppuccin", Dark: "catppuccin-mocha", Light: "catppuccin-latte"},
		Palette{
			Name:       "catppuccin-mocha",
			Background: "#1e1e2e",
			Text:       "#cdd6f4", Strong: "#cdd6f4", Muted: "#a6adc8", Rule: "#45475a",
			OK: "#a6e3a1", Warn: "#f9e2af", Fail: "#f38ba8",
			Key: "#fab387",
		},
		Palette{
			Name: "catppuccin-latte",
			// muted: upstream subtext0 #6c6f85 is 4.37:1; subtext1 used instead.
			// ok: upstream green #40a02b is 2.96:1; text used instead.
			// warn: upstream yellow #df8e1d is 2.31:1; red used instead.
			// key: upstream peach #fe640b is 2.64:1; subtext1 used instead.
			Background: "#eff1f5",
			Text:       "#4c4f69", Strong: "#4c4f69", Muted: "#5c5f77", Rule: "#bcc0cc",
			OK: "#4c4f69", Warn: "#d20f39", Fail: "#d20f39",
			Key: "#5c5f77",
		},
	)
	register(Family{Name: "catppuccin-frappe", Dark: "catppuccin-frappe", Light: "catppuccin-frappe"},
		Palette{
			Name:       "catppuccin-frappe",
			Background: "#303446",
			Text:       "#c6d0f5", Strong: "#c6d0f5", Muted: "#a5adce", Rule: "#51576d",
			OK: "#a6d189", Warn: "#e5c890", Fail: "#e78284",
			Key: "#ef9f76",
		},
	)
	register(Family{Name: "catppuccin-macchiato", Dark: "catppuccin-macchiato", Light: "catppuccin-macchiato"},
		Palette{
			Name:       "catppuccin-macchiato",
			Background: "#24273a",
			Text:       "#cad3f5", Strong: "#cad3f5", Muted: "#a5adcb", Rule: "#494d64",
			OK: "#a6da95", Warn: "#eed49f", Fail: "#ed8796",
			Key: "#f5a97f",
		},
	)
}
