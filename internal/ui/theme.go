// Package ui renders the installer.
//
// Themes are a table of palettes (palette.go) mapped onto one fixed set of
// tokens: text, strong, muted, rule, ok, warn, fail, key and cursor. The views
// only ever see the styles derived from those tokens, so a new palette never
// touches a view.
package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// DefaultThemeName is the theme used when none is configured.
const DefaultThemeName = "aval"

// Theme carries every style the views use. It is a struct rather than a set of
// package-level vars so tests can build one without a terminal attached.
type Theme struct {
	// Name is the resolved variant, for example "aval-dark" or "mono".
	Name   string
	Glyphs Glyphs

	// Tokens.
	Text, Strong, Muted, Rule lipgloss.Style
	OK, Warn, Fail            lipgloss.Style
	Key, Cursor               lipgloss.Style

	// Derived styles the current views use. Each is one token, nothing more.
	Title       lipgloss.Style
	Subtitle    lipgloss.Style
	Heading     lipgloss.Style
	Body        lipgloss.Style
	Selected    lipgloss.Style
	Marker      lipgloss.Style
	Checked     lipgloss.Style
	Unchecked   lipgloss.Style
	Success     lipgloss.Style
	Warning     lipgloss.Style
	Failure     lipgloss.Style
	Box         lipgloss.Style
	Bar         lipgloss.Style
	BarEmpty    lipgloss.Style
	Unavailable lipgloss.Style

	// stateGlyph styles the glyph of each StateKind. A palette whose state
	// token fell back to strong has no marker of its own, so the glyph is bold
	// whenever the value equals strong. plain is set under an Ascii profile.
	stateGlyph [3]lipgloss.Style
	plain      bool
	// renderer is the one the styles were built on, so a theme derived from
	// this one (the settings preview) emits the same colour profile.
	renderer *lipgloss.Renderer
}

// NewTheme resolves name (a family that follows c.Dark, or a pinned variant)
// and builds its styles on r. An Ascii profile (NO_COLOR, no TTY) overrides
// every theme with mono, so no colour sequence is ever emitted; bold and
// reverse survive.
//
// Nothing paints a background. The cursor row is reverse video, which reads in
// every terminal theme without assuming one.
func NewTheme(name string, c caps.Caps, r *lipgloss.Renderer) (Theme, error) {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	p, err := LookupPalette(name, c.Dark)
	if err != nil {
		return Theme{}, err
	}
	if c.Profile == termenv.Ascii {
		p = palettes["mono"]
	}

	fg := func(v string) lipgloss.Style {
		s := r.NewStyle()
		if v != "" {
			s = s.Foreground(lipgloss.Color(v))
		}
		return s
	}

	t := Theme{
		Name:     p.Name,
		renderer: r,
		Glyphs:   GlyphsFor(c),

		Text:   fg(p.Text),
		Strong: fg(p.Strong).Bold(true),
		Muted:  fg(p.Muted),
		Rule:   fg(p.Rule),
		OK:     fg(p.OK),
		Warn:   fg(p.Warn),
		Fail:   fg(p.Fail).Bold(true),
		Key:    fg(p.Key).Bold(true),
		Cursor: r.NewStyle().Reverse(true),
	}
	// terminal has no hex rule colour to recede with, so it uses faint. mono
	// stays on bold and reverse only, and muted is never faint anywhere.
	if p.Rule == "" && p.Name == "terminal" {
		t.Rule = t.Rule.Faint(true)
	}

	t.plain = c.Profile == termenv.Ascii
	for k, v := range map[StateKind]struct {
		val   string
		style lipgloss.Style
	}{StateOK: {p.OK, t.OK}, StateWarn: {p.Warn, t.Warn}, StateFail: {p.Fail, t.Fail}} {
		st := v.style
		if p.Strong != "" && v.val == p.Strong {
			st = st.Bold(true)
		}
		t.stateGlyph[k] = st
	}

	t.Title = t.Strong
	t.Subtitle = t.Muted
	t.Heading = t.Strong
	t.Body = t.Text
	t.Selected = t.Cursor
	t.Marker = t.Key
	t.Checked = t.OK.Bold(true)
	t.Unchecked = t.Muted
	t.Success = t.OK
	t.Warning = t.Warn
	t.Failure = t.Fail
	t.Box = r.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Rule.GetForeground()).Padding(0, 1)
	t.Bar = t.Text
	t.BarEmpty = t.Rule
	// Groups that do not apply to this machine are shown, not hidden: a user
	// on Windows should see that the terminal environment exists and why it
	// is unavailable, rather than wonder what is missing.
	t.Unavailable = t.Muted
	return t, nil
}

// DefaultTheme is the aval theme on the default renderer, with Unicode glyphs
// and the dark variant. It never fails: aval is always registered. Call sites
// that know the terminal's capabilities should use NewTheme.
func DefaultTheme() Theme {
	r := lipgloss.DefaultRenderer()
	t, err := NewTheme(DefaultThemeName, caps.Caps{Profile: r.ColorProfile(), Unicode: true, Dark: true}, r)
	if err != nil {
		panic(err)
	}
	return t
}

func (t Theme) styles() []lipgloss.Style {
	return []lipgloss.Style{
		t.Text, t.Strong, t.Muted, t.Rule, t.OK, t.Warn, t.Fail, t.Key, t.Cursor,
		t.Title, t.Subtitle, t.Heading, t.Body, t.Selected, t.Marker, t.Checked,
		t.Unchecked, t.Success, t.Warning, t.Failure, t.Box, t.Bar, t.BarEmpty,
		t.Unavailable,
	}
}

// ProgressBar renders a fixed-width bar. Width is a parameter because the
// installer must degrade on a narrow terminal rather than wrap into nonsense.
func (t Theme) ProgressBar(done, total, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += t.Bar.Render(t.Glyphs.BarFull)
		} else {
			bar += t.BarEmpty.Render(t.Glyphs.BarEmpty)
		}
	}
	return bar
}

// StateKind selects the glyph and token of a state cell.
type StateKind int

const (
	StateOK StateKind = iota
	StateWarn
	StateFail
)

// State renders a state cell: the glyph in the state token, a space, then the
// word in the text token. Colour sits on the glyph only (DESIGN.md section 10).
// Under an Ascii profile it is plain "glyph word", with no escapes at all.
func (t Theme) State(k StateKind, word string) string {
	glyph := t.Glyphs.OK
	switch k {
	case StateWarn:
		glyph = t.Glyphs.Warn
	case StateFail:
		glyph = t.Glyphs.Fail
	}
	return t.StateWith(k, glyph, word)
}

// StateWith is State with another glyph in the state's token: the hub's
// `↑ 3` behind-upstream badge is a warn state drawn with an arrow.
func (t Theme) StateWith(k StateKind, glyph, word string) string {
	style := t.stateGlyph[k]
	if t.plain {
		return glyph + " " + word
	}
	return style.Render(glyph) + " " + t.Text.Render(word)
}
