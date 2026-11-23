package ui

import (
	"testing"
	"unicode/utf8"

	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

func TestASCIIGlyphsArePureASCII(t *testing.T) {
	g := GlyphsFor(caps.Caps{Unicode: false})
	for role, s := range g.all() {
		if s == "" {
			t.Errorf("ascii %s is empty", role)
		}
		for i := 0; i < len(s); i++ {
			if s[i] > 0x7f {
				t.Errorf("ascii %s = %q contains non-ASCII byte %#x", role, s, s[i])
			}
		}
	}
}

func TestGlyphTiersMatchDesignContract(t *testing.T) {
	// Pinned against .agents/DESIGN.md section 10. The check marks are √ and ×,
	// never the Dingbats, which fail `aval design gate emoji`.
	want := map[bool]Glyphs{
		true: {
			Cursor: "›", Checked: "[√]", Unchecked: "[ ]", Partial: "[-]",
			OK: "√", Warn: "!", Fail: "×",
			BarFull: "━", BarEmpty: "─",
			Sep: "·", Ellipsis: "…",
			UpDown: "↑↓", LeftRight: "←→",
		},
		false: {
			Cursor: ">", Checked: "[x]", Unchecked: "[ ]", Partial: "[-]",
			OK: "ok", Warn: "!", Fail: "x",
			BarFull: "#", BarEmpty: "-",
			Sep: "  ", Ellipsis: "...",
			UpDown: "up/down", LeftRight: "left/right",
		},
	}
	for uni, w := range want {
		if got := GlyphsFor(caps.Caps{Unicode: uni}); got != w {
			t.Errorf("unicode=%v:\n got  %+v\n want %+v", uni, got, w)
		}
	}
}

func TestProgressGlyphsAreOneCellWide(t *testing.T) {
	// ProgressBar counts cells, so each bar glyph must be a single rune.
	for _, uni := range []bool{true, false} {
		g := GlyphsFor(caps.Caps{Unicode: uni})
		if utf8.RuneCountInString(g.BarFull) != 1 || utf8.RuneCountInString(g.BarEmpty) != 1 {
			t.Errorf("unicode=%v bar glyphs %q %q are not single runes", uni, g.BarFull, g.BarEmpty)
		}
	}
}
