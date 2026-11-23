package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The frame is the chrome every screen shares (DESIGN.md section 10):
//
//	lockup, tagline, rule, body, rule, status line, key hints
//
// No outer border and no painted canvas. Margins are 2 columns, 1 when
// compact, and content caps at maxContent. The frame is never wider or taller
// than the terminal: when height runs short it gives up, in order, the blank
// gaps, the tagline and the hint wrap, and only then does the body shrink.

const (
	// maxContent caps the content width; wide terminals get a margin, not a
	// longer line.
	maxContent = 76
	// compactBelow and regularFrom are the breakpoint edges in columns.
	compactBelow = 40
	regularFrom  = 80

	tagline = "Nothing passes without evidence."
)

type breakpoint int

const (
	bpCompact breakpoint = iota // under 40 columns
	bpNarrow                    // 40 to 79
	bpRegular                   // 80 and up
)

func breakpointFor(width int) breakpoint {
	switch {
	case width < compactBelow:
		return bpCompact
	case width < regularFrom:
		return bpNarrow
	}
	return bpRegular
}

// layout is the box a screen body draws into.
type layout struct {
	bp     breakpoint
	margin int
	// width is the usable content width, after margins and the cap.
	width int
}

func (m Model) layout() layout {
	l := layout{bp: breakpointFor(m.width), margin: 2}
	if l.bp == bpCompact {
		l.margin = 1
	}
	l.width = min(m.width-2*l.margin, maxContent)
	if l.width < 1 {
		l.width = 1
	}
	return l
}

// view is what a screen hands the frame. The hint line is not part of it: the
// frame builds that from the screen's key map.
type view struct {
	// body returns at most h lines, none wider than l.width. Screens with a
	// list scroll it so the cursor row stays visible. Called with a huge h,
	// it returns its natural height.
	body func(l layout, h int) []string
	// status is the plain-text line under the bottom rule; empty omits it.
	status string
}

// render lays v out inside the frame at the model's size.
func (m Model) render(v view) string {
	l := m.layout()
	compact := l.bp == bpCompact

	showTagline, showRules, showGaps, wrapHints := !compact, !compact, !compact, !compact
	chrome := func() int {
		n := 1 // lockup
		if showTagline {
			n++
		}
		if showRules {
			n += 2
		}
		if showGaps {
			n += 2
		}
		if v.status != "" {
			n++
		}
		return n + len(m.hintLines(l, wrapHints))
	}

	natural := len(v.body(l, math.MaxInt32))
	for _, drop := range []*bool{&showGaps, &showTagline, &wrapHints} {
		if natural <= m.height-chrome() {
			break
		}
		*drop = false
	}
	budget := max(m.height-chrome(), 1)

	var lines []string
	lines = append(lines, m.lockup(l))
	if showTagline {
		lines = append(lines, m.theme.Muted.Render(m.fit(tagline, l.width)))
	}
	if showRules {
		lines = append(lines, m.rule(l))
	}
	if showGaps {
		lines = append(lines, "")
	}
	body := v.body(l, budget)
	if len(body) > budget {
		body = body[:budget]
	}
	lines = append(lines, body...)
	if showGaps {
		lines = append(lines, "")
	}
	if showRules {
		lines = append(lines, m.rule(l))
	}
	if v.status != "" {
		lines = append(lines, m.theme.Text.Render(m.fit(v.status, l.width)))
	}
	lines = append(lines, m.hintLines(l, wrapHints)...)

	// Below the 8-row floor something has to go; the bottom of the frame
	// goes first, since the top says where you are.
	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	pad := strings.Repeat(" ", l.margin)
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

// lockup is "aval  <version>" with the platform right-aligned. Compact keeps
// "aval <version>" only.
func (m Model) lockup(l layout) string {
	name := m.theme.Title.Render("aval")
	if m.version == "" {
		return name
	}
	gap := "  "
	if l.bp == bpCompact {
		gap = " "
	}
	ver := m.theme.Muted.Render(m.fit(m.version, l.width-lipgloss.Width(name)-len(gap)))
	left := name + gap + ver
	if l.bp == bpCompact || m.platform == "" {
		return left
	}
	if room := l.width - lipgloss.Width(left) - lipgloss.Width(m.platform); room >= 2 {
		return left + strings.Repeat(" ", room) + m.theme.Muted.Render(m.platform)
	}
	return left
}

func (m Model) rule(l layout) string {
	return m.theme.Rule.Render(strings.Repeat(m.theme.Glyphs.BarEmpty, l.width))
}

// sep is the hint and list separator: " · " in Unicode, two spaces in ASCII.
func (m Model) sep() string {
	if strings.TrimSpace(m.theme.Glyphs.Sep) == "" {
		return "  "
	}
	return " " + m.theme.Glyphs.Sep + " "
}

// hintLines renders the current key map. With wrap, hints that do not fit
// start a new line. Without it (compact, or a short terminal) the line keeps
// every hint if they fit on one line and only the core ones otherwise.
func (m Model) hintLines(l layout, wrap bool) []string {
	km := m.keyMap()
	all := km.hints(m.theme.Glyphs)
	if len(all) == 0 {
		return nil
	}
	sep := m.theme.Muted.Render(m.sep())
	part := func(h hint) string {
		return m.theme.Key.Render(h.key) + " " + m.theme.Muted.Render(h.desc)
	}
	join := func(hs []hint) string {
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = part(h)
		}
		return strings.Join(parts, sep)
	}

	if !wrap {
		if l.bp != bpCompact {
			if line := join(all); lipgloss.Width(line) <= l.width {
				return []string{line}
			}
		}
		var line string
		for _, h := range km.hintsWhere(m.theme.Glyphs, true) {
			next := part(h)
			if line != "" {
				next = line + sep + next
			}
			if lipgloss.Width(next) > l.width {
				break
			}
			line = next
		}
		if line == "" {
			return nil
		}
		return []string{line}
	}

	var lines []string
	line := ""
	for _, h := range all {
		p := part(h)
		if line != "" && lipgloss.Width(line+sep+p) > l.width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += sep
		}
		// A single hint wider than the content is cut rather than overflow.
		if lipgloss.Width(p) > l.width {
			p = m.theme.Muted.Render(m.fit(h.key+" "+h.desc, l.width))
		}
		line += p
	}
	return append(lines, line)
}

// headingRow puts right beside left when both fit with a two-column gap, and
// on its own line below otherwise. Either side may be empty. Both come in
// plain and are styled here, so they can be fitted first.
func (m Model) headingRow(l layout, left string, leftStyle lipgloss.Style, right string, rightStyle lipgloss.Style) []string {
	if right == "" {
		return []string{leftStyle.Render(m.fit(left, l.width))}
	}
	if room := l.width - lipgloss.Width(left) - lipgloss.Width(right); room >= 2 {
		return []string{leftStyle.Render(left) + strings.Repeat(" ", room) + rightStyle.Render(right)}
	}
	return []string{
		leftStyle.Render(m.fit(left, l.width)),
		rightStyle.Render(m.fit(right, l.width)),
	}
}

// scrollWindow returns the [start, end) slice of n one-line rows that fits in
// h lines and keeps cursor visible. A cursor outside the rows pins the top.
func scrollWindow(n, cursor, h int) (int, int) {
	if h <= 0 {
		return 0, 0
	}
	if n <= h {
		return 0, n
	}
	start := 0
	if cursor >= h {
		start = cursor - h + 1
	}
	if start > n-h {
		start = n - h
	}
	return start, start + h
}

// PlatformLabel formats GOOS and GOARCH for the lockup: "macOS (arm64)",
// "Linux (amd64)", "Windows (amd64)". Unknown systems keep their GOOS name.
func PlatformLabel(goos, goarch string) string {
	name := goos
	switch goos {
	case "darwin":
		name = "macOS"
	case "linux":
		name = "Linux"
	case "windows":
		name = "Windows"
	}
	return fmt.Sprintf("%s (%s)", name, goarch)
}
