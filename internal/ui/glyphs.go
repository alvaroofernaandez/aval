package ui

import "github.com/alvaroofernaandez/aval/internal/ui/caps"

// Glyphs is one glyph tier from DESIGN.md section 10. A state glyph is always
// followed by its word, so neither tier carries meaning on its own.
type Glyphs struct {
	Cursor                      string
	Checked, Unchecked, Partial string
	OK, Warn, Fail              string
	BarFull, BarEmpty           string
	Sep, Ellipsis               string
	UpDown, LeftRight           string
}

// UnicodeGlyphs is the tier for UTF-8 terminals. The check marks are √ and ×:
// the Dingbat marks fail `aval design gate emoji`.
var UnicodeGlyphs = Glyphs{
	Cursor:  "›",
	Checked: "[√]", Unchecked: "[ ]", Partial: "[-]",
	OK: "√", Warn: "!", Fail: "×",
	BarFull: "━", BarEmpty: "─",
	Sep: "·", Ellipsis: "…",
	UpDown: "↑↓", LeftRight: "←→",
}

// ASCIIGlyphs is the tier for terminals that cannot be trusted with UTF-8.
var ASCIIGlyphs = Glyphs{
	Cursor:  ">",
	Checked: "[x]", Unchecked: "[ ]", Partial: "[-]",
	OK: "ok", Warn: "!", Fail: "x",
	BarFull: "#", BarEmpty: "-",
	Sep: "  ", Ellipsis: "...",
	UpDown: "up/down", LeftRight: "left/right",
}

// GlyphsFor picks the tier the terminal can render.
func GlyphsFor(c caps.Caps) Glyphs {
	if c.Unicode {
		return UnicodeGlyphs
	}
	return ASCIIGlyphs
}

func (g Glyphs) all() map[string]string {
	return map[string]string{
		"cursor": g.Cursor, "checked": g.Checked, "unchecked": g.Unchecked, "partial": g.Partial,
		"ok": g.OK, "warn": g.Warn, "fail": g.Fail,
		"bar_full": g.BarFull, "bar_empty": g.BarEmpty,
		"sep": g.Sep, "ellipsis": g.Ellipsis,
		"up_down": g.UpDown, "left_right": g.LeftRight,
	}
}
