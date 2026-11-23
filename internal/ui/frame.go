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

// chrome is the shared frame state: the theme, the lockup labels and the
// terminal size. Model embeds it for the installer screens; App holds one
// for the screen stack, so every view draws the same frame.
type chrome struct {
	theme    Theme
	version  string
	platform string

	width, height int
}

// layout is the box a screen body draws into.
type layout struct {
	bp     breakpoint
	margin int
	// width is the usable content width, after margins and the cap.
	width int
	// hero is set when the view's hero rows replace the lockup.
	hero bool
}

func (c chrome) layout() layout {
	l := layout{bp: breakpointFor(c.width), margin: 2}
	if l.bp == bpCompact {
		l.margin = 1
	}
	l.width = min(c.width-2*l.margin, maxContent)
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
	// hero, when set, returns styled rows that replace the lockup (the hub
	// wordmark), or nil when they do not fit the width. The frame shows them
	// only when everything else fits at its natural height, so the hero is
	// the first thing to give way; body is then called with l.hero set.
	hero func(l layout, height int) []string
	// platform replaces the chrome's platform label when set (the sandbox
	// marker inside `aval dev`).
	platform string
}

// render lays v out inside the frame with the model's own key map.
func (m Model) render(v view) string { return m.renderWith(v, m.keyMap()) }

// hintLines renders the model's own key map; see chrome.hintLines.
func (m Model) hintLines(l layout, wrap bool) []string {
	return m.chrome.hintLines(l, wrap, m.keyMap())
}

// renderWith lays v out inside the frame at the chrome's size, with the hint
// line built from km.
func (c chrome) renderWith(v view, km keyMap) string {
	if v.platform != "" {
		c.platform = v.platform
	}
	l := c.layout()
	compact := l.bp == bpCompact
	if v.hero != nil && !compact {
		if out, ok := c.renderHero(v, km, l); ok {
			return out
		}
	}

	showTagline, showRules, showGaps, wrapHints := !compact, !compact, !compact, !compact
	fixed := func() int {
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
		return n + len(c.hintLines(l, wrapHints, km))
	}

	natural := len(v.body(l, math.MaxInt32))
	for _, drop := range []*bool{&showGaps, &showTagline, &wrapHints} {
		if natural <= c.height-fixed() {
			break
		}
		*drop = false
	}
	budget := max(c.height-fixed(), 1)

	var lines []string
	lines = append(lines, c.lockup(l))
	if showTagline {
		lines = append(lines, c.theme.Muted.Render(c.fit(tagline, l.width)))
	}
	if showRules {
		lines = append(lines, c.rule(l))
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
		lines = append(lines, c.rule(l))
	}
	if v.status != "" {
		lines = append(lines, c.theme.Text.Render(c.fit(v.status, l.width)))
	}
	lines = append(lines, c.hintLines(l, wrapHints, km)...)

	// Below the 8-row floor something has to go; the bottom of the frame
	// goes first, since the top says where you are.
	if c.height > 0 && len(lines) > c.height {
		lines = lines[:c.height]
	}
	pad := strings.Repeat(" ", l.margin)
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

// renderHero lays v out with its hero rows in place of the lockup, and the
// tagline beside the platform under them. It reports false when the hero
// does not fit with every gap, the tagline and the wrapped hints, so the
// caller falls back to the lockup.
func (c chrome) renderHero(v view, km keyMap, l layout) (string, bool) {
	hero := v.hero(l, c.height)
	if len(hero) == 0 {
		return "", false
	}
	l.hero = true
	body := v.body(l, math.MaxInt32)
	hints := c.hintLines(l, true, km)
	n := len(hero) + 1 + 2 + 2 + len(body) + len(hints)
	if v.status != "" {
		n++
	}
	if n > c.height {
		return "", false
	}
	lines := append([]string(nil), hero...)
	lines = append(lines, c.headingRow(l, tagline, c.theme.Muted, c.platform, c.theme.Muted)[0])
	lines = append(lines, c.rule(l), "")
	lines = append(lines, body...)
	lines = append(lines, "", c.rule(l))
	if v.status != "" {
		lines = append(lines, c.theme.Text.Render(c.fit(v.status, l.width)))
	}
	lines = append(lines, hints...)
	pad := strings.Repeat(" ", l.margin)
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n"), true
}

// lockup is "aval  <version>" with the platform right-aligned. Compact keeps
// "aval <version>" only.
func (c chrome) lockup(l layout) string {
	name := c.theme.Title.Render("aval")
	if c.version == "" {
		return name
	}
	gap := "  "
	if l.bp == bpCompact {
		gap = " "
	}
	ver := c.theme.Muted.Render(c.fit(c.version, l.width-lipgloss.Width(name)-len(gap)))
	left := name + gap + ver
	if l.bp == bpCompact || c.platform == "" {
		return left
	}
	if room := l.width - lipgloss.Width(left) - lipgloss.Width(c.platform); room >= 2 {
		return left + strings.Repeat(" ", room) + c.theme.Muted.Render(c.platform)
	}
	return left
}

func (c chrome) rule(l layout) string {
	return c.theme.Rule.Render(strings.Repeat(c.theme.Glyphs.BarEmpty, l.width))
}

// sep is the hint and list separator: " · " in Unicode, two spaces in ASCII.
func (c chrome) sep() string {
	if strings.TrimSpace(c.theme.Glyphs.Sep) == "" {
		return "  "
	}
	return " " + c.theme.Glyphs.Sep + " "
}

// hintLines renders km. With wrap, hints that do not fit
// start a new line. Without it (compact, or a short terminal) the line keeps
// every hint if they fit on one line and only the core ones otherwise, with
// `? help` on a line of its own when it does not fit beside them.
func (c chrome) hintLines(l layout, wrap bool, km keyMap) []string {
	all := km.hints(c.theme.Glyphs)
	if len(all) == 0 {
		return nil
	}
	sep := c.theme.Muted.Render(c.sep())
	part := func(h hint) string {
		return c.theme.Key.Render(h.key) + " " + c.theme.Muted.Render(h.desc)
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
		// `? help` is the way to every other key, so it always shows: on the
		// core line when it fits, on its own line otherwise.
		var line, help string
		var core []hint
		for _, h := range km.hintsWhere(c.theme.Glyphs, true) {
			if h.key == helpBinding.label {
				help = part(h)
			} else {
				core = append(core, h)
			}
		}
		for _, h := range core {
			next := part(h)
			if line != "" {
				next = line + sep + next
			}
			if lipgloss.Width(next) > l.width {
				break
			}
			line = next
		}
		switch {
		case help == "":
		case line == "":
			line = help
		case lipgloss.Width(line+sep+help) <= l.width:
			line += sep + help
		default:
			return []string{line, help}
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
			p = c.theme.Muted.Render(c.fit(h.key+" "+h.desc, l.width))
		}
		line += p
	}
	return append(lines, line)
}

// headingRow puts right beside left when both fit with a two-column gap, and
// on its own line below otherwise. Either side may be empty. Both come in
// plain and are styled here, so they can be fitted first.
func (c chrome) headingRow(l layout, left string, leftStyle lipgloss.Style, right string, rightStyle lipgloss.Style) []string {
	if right == "" {
		return []string{leftStyle.Render(c.fit(left, l.width))}
	}
	if room := l.width - lipgloss.Width(left) - lipgloss.Width(right); room >= 2 {
		return []string{leftStyle.Render(left) + strings.Repeat(" ", room) + rightStyle.Render(right)}
	}
	return []string{
		leftStyle.Render(c.fit(left, l.width)),
		rightStyle.Render(c.fit(right, l.width)),
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
