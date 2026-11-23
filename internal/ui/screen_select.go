package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The select screen: choose groups before anything is written. Per-group state
// counts, details and the empty-selection message belong here (T8).

func (m Model) selectAction(act action) (tea.Model, tea.Cmd) {
	switch act {
	case actUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case actDown:
		if m.cursor < len(m.plan.Groups)-1 {
			m.cursor++
		}
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
		// Nothing selected is not an error, but starting an empty run and
		// showing an empty summary would be confusing. Stay put instead.
		if len(m.queue) == 0 {
			return m, nil
		}
		m.screen = ScreenInstall
		m.index = 0
		return m, m.applyNext()
	}
	return m, nil
}

// installStyle says how files land, in UI copy (no em dash, unlike the
// platform package's headless wording).
func (m Model) installStyle() string {
	if m.Symlinks {
		return "symlinked: repo edits apply at once"
	}
	return "copied: run aval install again after repo edits"
}

func (m Model) selectView() view {
	selected, total := 0, 0
	for _, g := range m.plan.Groups {
		total += len(g.Actions)
		if g.Selected {
			selected += len(g.Actions)
		}
	}
	status := fmt.Sprintf("%d of %d items selected", selected, total)
	if m.layout().bp == bpCompact {
		status = fmt.Sprintf("%d/%d items selected", selected, total)
	}
	return view{body: m.selectBody, status: status}
}

// selectRow is one list entry: the row itself and an optional detail line.
type selectRow struct {
	line, detail string
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
	rows := m.selectRows(l)

	full := 0
	for _, r := range rows {
		full++
		if r.detail != "" {
			full++
		}
	}

	out := append([]string{}, head...)
	switch {
	case len(head)+1+full <= h:
		out = append(out, "")
		fallthrough
	case len(head)+full <= h:
		for _, r := range rows {
			out = append(out, r.line)
			if r.detail != "" {
				out = append(out, r.detail)
			}
		}
		return out
	}

	// Short terminal: details go, and the rows scroll around the cursor.
	listH := h - len(head)
	if listH < 1 {
		out, listH = nil, h
	}
	start, end := scrollWindow(len(rows), m.cursor, listH)
	for _, r := range rows[start:end] {
		out = append(out, r.line)
	}
	return out
}

func (m Model) selectRows(l layout) []selectRow {
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
		labelW = min(labelW, 24)
	}

	var rows []selectRow
	for i, grp := range m.plan.Groups {
		box := m.theme.Unchecked.Render(g.Unchecked)
		if grp.Selected {
			box = m.theme.Checked.Render(g.Checked)
		}
		marker := "  "
		if i == m.cursor {
			marker = m.theme.Marker.Render(g.Cursor + " ")
		}
		line := box + " " + m.pad(grp.Label, labelW) + " " + m.theme.Muted.Render(count(len(grp.Actions)))
		if i == m.cursor {
			line = m.theme.Selected.Render(line)
		}
		r := selectRow{line: marker + line}
		if grp.Detail != "" && !compact {
			r.detail = "      " + m.theme.Muted.Render(m.fit(grp.Detail, l.width-6))
		}
		rows = append(rows, r)
	}

	for _, u := range m.Unavailable {
		text := g.Partial + " " + m.pad(u.Label, max(labelW, 1)) + " " + u.Reason
		rows = append(rows, selectRow{line: "  " + m.theme.Unavailable.Render(m.fit(text, l.width-2))})
	}
	return rows
}
