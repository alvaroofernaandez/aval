package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/install"
)

// The install screen: one action per update cycle, so the bar moves. Inline
// failures and the graceful ctrl+c stop belong here (T9): installKeys binds
// q and ctrl+c to actHold today.

// applyNext applies one action per cycle, so the UI repaints between them and
// the progress bar actually moves. Doing the whole run in one command would
// freeze the frame until it finished.
func (m Model) applyNext() tea.Cmd {
	if m.index >= len(m.queue) {
		return func() tea.Msg { return doneMsg{} }
	}
	action := m.queue[m.index]
	runner := m.runner
	return func() tea.Msg {
		res, err := runner.Apply(action)
		return appliedMsg{result: res, err: err}
	}
}

func (m Model) installView() view {
	status := "nothing failed so far"
	if n := len(m.errs); n > 0 {
		status = fmt.Sprintf("%d failed so far", n)
	}
	return view{body: m.installBody, status: status}
}

func (m Model) installBody(l layout, h int) []string {
	done, total := m.index, len(m.queue)
	out := m.headingRow(l, "Installing", m.theme.Heading, fmt.Sprintf("%d of %d", done, total), m.theme.Muted)
	out = append(out, m.theme.ProgressBar(done, total, l.width))

	rows := m.installRows(l)
	room := h - len(out)
	if room > len(rows) {
		// Room to spare: a gap between the bar and the log.
		out = append(out, "")
		room--
	}
	// The newest rows only: a full log would push the bar off the top.
	if room < len(rows) {
		rows = rows[len(rows)-max(room, 0):]
	}
	return append(out, rows...)
}

// installRows is the log: "√ linked  ~/.config/yazi", then the step running now.
func (m Model) installRows(l layout) []string {
	g := m.theme.Glyphs
	type cell struct {
		state string
		style lipgloss.Style
		label string
	}
	var cells []cell
	for _, r := range m.results {
		c := cell{state: g.OK + " " + r.Status.String(), style: m.theme.Success, label: r.Action.Label}
		switch {
		case r.Backup != "":
			c.state, c.style = g.Warn+" backed up", m.theme.Warning
		case r.Status == install.StatusAlready:
			c.style = m.theme.Muted
		case r.Status == install.StatusWould:
			c.state, c.style = g.Warn+" "+r.Status.String(), m.theme.Warning
		}
		cells = append(cells, c)
	}
	if m.index < len(m.queue) {
		verb := "copying"
		switch {
		case m.DryRun:
			verb = "checking"
		case m.Symlinks:
			verb = "linking"
		}
		cells = append(cells, cell{state: g.Cursor + " " + verb, style: m.theme.Key, label: m.queue[m.index].Label})
	}

	stateW := 0
	for _, c := range cells {
		stateW = max(stateW, lipgloss.Width(c.state))
	}
	var rows []string
	for _, c := range cells {
		pathW := l.width - 2 - stateW - 2
		if pathW < 4 {
			// No room for a path: the state is the part that matters.
			rows = append(rows, "  "+c.style.Render(m.fit(c.state, l.width-2)))
			continue
		}
		state := c.state + strings.Repeat(" ", stateW-lipgloss.Width(c.state))
		rows = append(rows, "  "+c.style.Render(state)+"  "+m.fitMiddle(c.label, pathW))
	}
	return rows
}
