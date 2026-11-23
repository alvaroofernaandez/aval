# Theme sources

Every hex in `internal/ui/palette_<theme>.go` was read from the upstream file below, fetched from its default branch on 2026-11-23. The contrast rule is DESIGN.md section 10, "State colour rule (all themes)":

- Text tokens (`text`, `strong`, `muted`, `key`) need 4.5:1. One that misses is swapped only to a neutral entry of the same palette (foreground, subtext, comment, base or overlay greys).
- State tokens (`ok`, `warn`, `fail`) colour a glyph only and need 3:1. One that misses keeps its hue: it takes a passing entry of the same hue family, or falls back to `strong` (recorded as `fallback: strong`, drawn bold). A state never changes hue.
- `ok`, `warn` and `fail` stay pairwise distinct unless they fell back to strong.

Every swap is listed in the theme's test and documented, with a reason, in `palette_contrast_test.go` (`expectedSwaps`).

| Theme | Upstream | Files read | Licence |
| :--- | :--- | :--- | :--- |
| catppuccin | https://github.com/catppuccin/palette | `palette.json` | MIT |
| tokyo-night | https://github.com/folke/tokyonight.nvim | `extras/lua/tokyonight_{night,storm,day}.lua` | **Apache-2.0** |
| gruvbox | https://github.com/morhetz/gruvbox | `colors/gruvbox.vim` | MIT/X11 (stated in the README; the repository has no LICENSE file) |
| dracula | https://github.com/dracula/dracula-theme | `README.md` (Dracula and Alucard tables) | MIT |
| nord | https://github.com/nordtheme/nord | `src/nord.css` | MIT |
| kanagawa | https://github.com/rebelot/kanagawa.nvim | `lua/kanagawa/colors.lua` | MIT |
| rose-pine | https://github.com/rose-pine/neovim | `lua/rose-pine/palette.lua` | MIT |
| solarized | https://github.com/altercation/solarized | `README.md` palette table | MIT |
| one-dark | https://github.com/joshdick/onedark.vim | `autoload/onedark.vim` | MIT |
| one-light | https://github.com/navarasu/onedark.nvim | `lua/onedark/palette.lua` (`light`) | MIT |
| everforest | https://github.com/sainnhe/everforest | `autoload/everforest.vim` (medium contrast) | MIT |

Notes:

- tokyo-night is Apache-2.0, not MIT. Only colour values are used, not code.
- rose-pine takes its palette from the `neovim` port, whose `dawn` text is `#464261`. The theme site publishes `#575279` there.
- one-light has no hex palette in the original Atom package (it uses HSL), so the `light` palette of onedark.nvim is used.
- State fallbacks to strong: catppuccin-latte (ok, warn), everforest-light (ok, warn), rose-pine-dawn (warn), solarized-light (ok, warn). No entry of the same hue reaches 3:1 there. catppuccin-latte teal `#179299` passes but the palette does not name it a green variant, so it is a different hue and was not used.
