package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

// The select screen: choose groups before anything is written. Every group
// says what would happen at each of its destinations, read once from
// Runner.Preview when the model is built, so View never touches the disk.

// The select screen's own actions. They are negative so they can never
// collide with the shared iota block in keys.go.
const (
	actExpand   action = -1 // show the cursor group's destinations
	actCollapse action = -2 // hide them again
)

// emptySelection is the status line after enter with nothing selected.
const emptySelection = "Select at least one group."

// selectState is what the select screen keeps beyond the cursor.
type selectState struct {
	// states holds Preview's answer per group, per action, in plan order.
	states [][]install.State
	// expanded is the group whose destinations are listed, or -1.
	expanded int
	// notice replaces the status line until the next action.
	notice string
}

func newSelectState(p *plan.Plan, r *install.Runner) selectState {
	s := selectState{expanded: -1}
	if p == nil {
		return s
	}
	s.states = make([][]install.State, len(p.Groups))
	for i, g := range p.Groups {
		s.states[i] = make([]install.State, len(g.Actions))
		if r == nil {
			continue
		}
		for j, a := range g.Actions {
			s.states[i][j] = r.Preview(a)
		}
	}
	return s
}

// stateCounts tallies new, linked and replaces for one group.
type stateCounts [3]int

func (c *stateCounts) add(o stateCounts) {
	for i := range c {
		c[i] += o[i]
	}
}

func (m Model) groupCounts(i int) stateCounts {
	var c stateCounts
	if i < len(m.sel.states) {
		for _, st := range m.sel.states[i] {
			if int(st) >= 0 && int(st) < len(c) {
				c[st]++
			}
		}
	}
	return c
}

func (m Model) selectAction(act action) (tea.Model, tea.Cmd) {
	m.sel.notice = ""
	switch act {
	case actUp:
		if m.cursor > 0 {
			m.cursor--
			m.sel.expanded = -1
		}
	case actDown:
		if m.cursor < len(m.plan.Groups)-1 {
			m.cursor++
			m.sel.expanded = -1
		}
	case actExpand:
		if len(m.plan.Groups) > 0 {
			m.sel.expanded = m.cursor
		}
	case actCollapse:
		m.sel.expanded = -1
	case actToggle:
		if len(m.plan.Groups) > 0 {
			m.plan.Groups[m.cursor].Selected = !m.plan.Groups[m.cursor].Selected
		}
	case actAll:
		for i := range m.plan.Groups {
			m.plan.Groups[i].Selected = true
		}
	case actClear:
		for i := range m.plan.Groups {
			m.plan.Groups[i].Selected = false
		}
	case actInstall:
		m.queue = m.plan.SelectedActions()
		// Starting an empty run would end on an empty summary that reads
		// like a failure. Stay put and say what is missing.
		if len(m.queue) == 0 {
			m.sel.notice = emptySelection
			return m, nil
		}
		m.screen = ScreenInstall
		m.index = 0
		return m, m.applyNext()
	}
	return m, nil
}

// installStyle says how files land, in UI copy.
func (m Model) installStyle() string {
	if m.Symlinks {
		return "symlinked: repo edits apply at once"
	}
	return "copied: run aval install again after repo edits"
}

func (m Model) selectView() view {
	return view{body: m.selectBody, status: m.selectStatus(m.layout())}
}

// selectStatus totals the selection. It keeps the longest form that fits:
// regular spells every count out, narrow keeps the backup count, the one
// number that means an existing file moves, and compact keeps the ratio.
func (m Model) selectStatus(l layout) string {
	if m.sel.notice != "" {
		return m.sel.notice
	}
	selected, total := 0, 0
	var c stateCounts
	for i, g := range m.plan.Groups {
		total += len(g.Actions)
		if g.Selected {
			selected += len(g.Actions)
			c.add(m.groupCounts(i))
		}
	}
	sep := m.sep()
	moved := "files"
	if c[install.StateReplaces] == 1 {
		moved = "file"
	}
	short := fmt.Sprintf("%d/%d selected", selected, total)
	if l.bp == bpCompact {
		return short
	}
	for _, s := range []string{
		fmt.Sprintf("%d of %d items selected%s%d new%s%d linked%s%d %s moved to backup",
			selected, total, sep, c[install.StateNew], sep, c[install.StateLinked], sep, c[install.StateReplaces], moved),
		fmt.Sprintf("%d of %d selected%s%d new%s%d linked%s%d to backup",
			selected, total, sep, c[install.StateNew], sep, c[install.StateLinked], sep, c[install.StateReplaces]),
		fmt.Sprintf("%d of %d selected%s%d to backup", selected, total, sep, c[install.StateReplaces]),
	} {
		if l.bp == bpNarrow && strings.Contains(s, "items") {
			continue
		}
		if lipgloss.Width(s) <= l.width {
			return s
		}
	}
	return short
}

// stateSummary is "8 new · 3 linked · 1 replaces a file", zeros left out.
func (m Model) stateSummary(c stateCounts) string {
	var parts []string
	for _, st := range []install.State{install.StateNew, install.StateLinked, install.StateReplaces} {
		if c[st] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c[st], st))
		}
	}
	return strings.Join(parts, m.sep())
}

func (m Model) selectBody(l layout, h int) []string {
	right, rightStyle := m.installStyle(), m.theme.Muted
	if l.bp == bpCompact {
		right = ""
	}
	if m.DryRun {
		right, rightStyle = "dry run: nothing will be written", m.theme.Warning
	}
	head := m.headingRow(l, "Choose what to install", m.theme.Heading, right, rightStyle)
	list, cur, curEnd := m.selectLines(l, true)
	// The cursor's detail line is the first thing a short terminal gives up;
	// destinations the user asked for with the right arrow stay.
	if len(head)+len(list) > h {
		list, cur, curEnd = m.selectLines(l, false)
	}

	out := append([]string{}, head...)
	switch {
	case len(head)+1+len(list) <= h:
		out = append(out, "")
		fallthrough
	case len(head)+len(list) <= h:
		return append(out, list...)
	}

	// Short terminal: the list scrolls. The window holds the cursor row and as
	// much of its detail and destinations as fits; rows above it show only
	// when there is room left.
	listH := h - len(head)
	if listH < 1 {
		out, listH = nil, h
	}
	start := 0
	if curEnd > listH {
		start = min(curEnd-listH, cur)
	}
	start = min(start, max(len(list)-listH, 0))
	end := min(start+listH, len(list))
	win := append([]string{}, list[start:end]...)
	if hidden := curEnd - end; hidden > 0 && len(win) > 1 {
		win[len(win)-1] = "      " + m.theme.Muted.Render(m.fit(fmt.Sprintf("%d more", hidden+1), l.width-6))
	}
	return append(out, win...)
}

// selectLines is the list as lines, with the cursor row's index and the end
// of its block (the row, its detail and any expanded destinations).
func (m Model) selectLines(l layout, withDetail bool) (lines []string, cur, curEnd int) {
	g := m.theme.Glyphs
	compact := l.bp == bpCompact

	// Numbers right-align in their own column; the noun follows.
	digits := 1
	for _, grp := range m.plan.Groups {
		digits = max(digits, len(fmt.Sprint(len(grp.Actions))))
	}
	count := func(n int) string {
		num := fmt.Sprintf("%*d", digits, n)
		switch {
		case compact:
			return num
		case n == 1:
			return num + " item "
		}
		return num + " items"
	}
	countW := lipgloss.Width(count(0))
	boxW := lipgloss.Width(g.Checked)
	// marker (2) + box + gap + label + gap + count.
	labelW := l.width - 2 - boxW - 1 - 1 - countW
	if !compact {
		// The label column is as wide as the longest label, capped at 24, so
		// short labels leave room for the state counts.
		longest := 1
		for _, grp := range m.plan.Groups {
			longest = max(longest, lipgloss.Width(grp.Label))
		}
		for _, u := range m.Unavailable {
			longest = max(longest, lipgloss.Width(u.Label))
		}
		labelW = min(labelW, 24, longest)
	}
	// What is left after the count, less a two-column gap, holds the states.
	statesW := l.width - 2 - boxW - 1 - labelW - 1 - countW - 2

	for i, grp := range m.plan.Groups {
		box := m.theme.Unchecked.Render(g.Unchecked)
		if grp.Selected {
			box = m.theme.Checked.Render(g.Checked)
		}
		marker := "  "
		if i == m.cursor {
			marker = m.theme.Marker.Render(g.Cursor + " ")
			cur = len(lines)
		}
		line := box + " " + m.pad(grp.Label, labelW) + " " + m.theme.Muted.Render(count(len(grp.Actions)))
		if s := m.stateSummary(m.groupCounts(i)); s != "" && !compact && statesW >= 8 {
			line += "  " + m.theme.Muted.Render(m.fit(s, statesW))
		}
		if i == m.cursor {
			line = m.theme.Selected.Render(line)
		}
		lines = append(lines, marker+line)
		if i != m.cursor {
			continue
		}
		if grp.Detail != "" && !compact && withDetail {
			lines = append(lines, "      "+m.theme.Muted.Render(m.fit(grp.Detail, l.width-6)))
		}
		if m.sel.expanded == i {
			lines = append(lines, m.destinationLines(l, i)...)
		}
		curEnd = len(lines)
	}

	for _, u := range m.Unavailable {
		text := g.Partial + " " + m.pad(u.Label, max(labelW, 1)) + " " + u.Reason
		lines = append(lines, "  "+m.theme.Unavailable.Render(m.fit(text, l.width-2)))
	}
	return lines, cur, curEnd
}

// destinationLines lists one group's destinations with their state: a glyph
// and its word for linked and replaces, the word alone for new, aligned to
// the same column.
func (m Model) destinationLines(l layout, gi int) []string {
	g := m.theme.Glyphs
	glyphW := max(lipgloss.Width(g.OK), lipgloss.Width(g.Warn))
	const indent = 6
	wordW := glyphW + 1 + lipgloss.Width(install.StateReplaces.String())
	pathW := l.width - indent - 2 - wordW
	longest := 1
	for _, a := range m.plan.Groups[gi].Actions {
		longest = max(longest, lipgloss.Width(a.Label))
	}
	pathW = min(pathW, longest)
	var out []string
	for j, a := range m.plan.Groups[gi].Actions {
		st := install.StateNew
		if j < len(m.sel.states[gi]) {
			st = m.sel.states[gi][j]
		}
		var cell string
		switch st {
		case install.StateLinked:
			cell = m.theme.State(StateOK, st.String())
		case install.StateReplaces:
			cell = m.theme.State(StateWarn, st.String())
		default:
			cell = strings.Repeat(" ", glyphW+1) + m.theme.Text.Render(st.String())
		}
		if pathW < 4 {
			out = append(out, strings.Repeat(" ", indent)+m.fit(a.Label+" "+st.String(), l.width-indent))
			continue
		}
		path := m.fitMiddle(a.Label, pathW)
		if gap := pathW - lipgloss.Width(path); gap > 0 {
			path += strings.Repeat(" ", gap)
		}
		out = append(out, strings.Repeat(" ", indent)+path+"  "+cell)
	}
	return out
}
