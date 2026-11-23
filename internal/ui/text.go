package ui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Truncation helpers. Labels end with the glyph tier's ellipsis; paths cut
// from the middle so the root and the file name both stay readable.

// fit shortens plain text to w columns, ending with the glyph tier's ellipsis.
func (c chrome) fit(s string, w int) string {
	if w < 1 {
		w = 1
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	ell := c.theme.Glyphs.Ellipsis
	budget := w - lipgloss.Width(ell)
	if budget < 0 {
		// Not even the ellipsis fits: cut it rather than overflow.
		return c.fitRunes(ell, w)
	}
	return strings.TrimRight(c.fitRunes(s, budget), " ") + ell
}

// fitRunes cuts s to at most w columns with no ellipsis.
func (c chrome) fitRunes(s string, w int) string {
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
func (c chrome) fitMiddle(s string, w int) string {
	if w < 1 {
		w = 1
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	ell := c.theme.Glyphs.Ellipsis
	keep := w - lipgloss.Width(ell)
	if keep < 2 {
		return c.fit(s, w)
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
func (c chrome) pad(s string, w int) string {
	if w < 1 {
		w = 1
	}
	s = c.fit(s, w)
	if gap := w - lipgloss.Width(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

// wrap breaks plain text into lines of at most w columns at spaces. A word
// wider than w is cut with the ellipsis rather than overflow.
func (c chrome) wrap(s string, w int) []string {
	w = max(w, 1)
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if lipgloss.Width(word) > w {
			word = c.fit(word, w)
		}
		switch {
		case line == "":
			line = word
		case lipgloss.Width(line)+1+lipgloss.Width(word) <= w:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// tildeUnder shortens p to ~/... when it sits under home.
func tildeUnder(home, p string) string {
	if home == "" || p == "" {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return p
}
