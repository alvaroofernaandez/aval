// Package cli styles status cells for plain command output (doctor, the
// headless installer). It carries no layout: callers keep their own columns and
// ask the Printer only for a glyph, a word and a colour.
package cli

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/ui"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// Printer styles status cells. The zero value and Plain() return text as is.
type Printer struct {
	styled  bool
	glyphs  ui.Glyphs
	theme   ui.Theme
	colored bool
}

// New builds a Printer for a terminal. Glyphs follow the Unicode tier of c;
// colour comes from the theme, which is already flattened when c has none.
func New(c caps.Caps, t ui.Theme) *Printer {
	return &Printer{styled: true, glyphs: ui.GlyphsFor(c), theme: t}
}

// Plain returns a Printer that adds nothing: no glyph, no escapes.
func Plain() *Printer { return &Printer{} }

// OK is the glyph, a space and the word, in the OK colour.
func (p *Printer) OK(word string) string { return p.cell(p.glyphs.OK, word, p.theme.OK) }

// Warn is the warning glyph and word.
func (p *Printer) Warn(word string) string { return p.cell(p.glyphs.Warn, word, p.theme.Warn) }

// Fail is the failure glyph and word.
func (p *Printer) Fail(word string) string { return p.cell(p.glyphs.Fail, word, p.theme.Fail) }

// Muted dims secondary text.
func (p *Printer) Muted(s string) string { return p.text(s, p.theme.Muted) }

// Strong emphasises text.
func (p *Printer) Strong(s string) string { return p.text(s, p.theme.Strong) }

// Width is the visible width of s, escape codes excluded.
func (p *Printer) Width(s string) int { return lipgloss.Width(s) }

func (p *Printer) cell(glyph, word string, st lipgloss.Style) string {
	if !p.styled {
		return word
	}
	return st.Render(glyph + " " + word)
}

func (p *Printer) text(s string, st lipgloss.Style) string {
	if !p.styled {
		return s
	}
	return st.Render(s)
}
