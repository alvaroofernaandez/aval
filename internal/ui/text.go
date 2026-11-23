package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Truncation helpers. Labels end with the glyph tier's ellipsis; paths cut
// from the middle so the root and the file name both stay readable.

// fit shortens plain text to w columns, ending with the glyph tier's ellipsis.
func (m Model) fit(s string, w int) string {
	if w < 1 {
		w = 1
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	ell := m.theme.Glyphs.Ellipsis
	budget := w - lipgloss.Width(ell)
	if budget < 0 {
		// Not even the ellipsis fits: cut it rather than overflow.
		return m.fitRunes(ell, w)
	}
	return strings.TrimRight(m.fitRunes(s, budget), " ") + ell
}

// fitRunes cuts s to at most w columns with no ellipsis.
func (m Model) fitRunes(s string, w int) string {
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String()
}

// fitMiddle shortens a path to w columns by cutting from the middle, so both
// the root and the file name stay readable.
func (m Model) fitMiddle(s string, w int) string {
	if w < 1 {
		w = 1
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	ell := m.theme.Glyphs.Ellipsis
	keep := w - lipgloss.Width(ell)
	if keep < 2 {
		return m.fit(s, w)
	}
	r := []rune(s)
	head := keep / 2
	tail := keep - head
	for lipgloss.Width(string(r[:head]))+lipgloss.Width(string(r[len(r)-tail:])) > keep && tail > 1 {
		tail--
	}
	return string(r[:head]) + ell + string(r[len(r)-tail:])
}

// pad fits s into exactly w columns, padding with spaces on the right.
func (m Model) pad(s string, w int) string {
	if w < 1 {
		w = 1
	}
	s = m.fit(s, w)
	if gap := w - lipgloss.Width(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}
