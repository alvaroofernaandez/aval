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
	// A cell is a state kind and its word; the running step is the one cell
	// that is not a state, so it renders through the key token instead.
	type cell struct {
		kind    StateKind
		word    string
		running bool
		label   string
	}
	render := func(c cell, word string) string {
		if c.running {
			return m.theme.Key.Render(g.Cursor + " " + word)
		}
		return m.theme.State(c.kind, word)
	}
	var cells []cell
	for _, r := range m.results {
		c := cell{kind: StateOK, word: r.Status.String(), label: r.Action.Label}
		switch {
		case r.Backup != "":
			c.kind, c.word = StateWarn, "backed up"
		case r.Status == install.StatusWould:
			c.kind = StateWarn
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
		cells = append(cells, cell{running: true, word: verb, label: m.queue[m.index].Label})
	}

	stateW := 0
	for _, c := range cells {
		stateW = max(stateW, lipgloss.Width(render(c, c.word)))
	}
	var rows []string
	for _, c := range cells {
		pathW := l.width - 2 - stateW - 2
		if pathW < 4 {
			// No room for a path: the state is the part that matters. The
			// word is cut, never the glyph.
			rows = append(rows, "  "+render(c, m.fit(c.word, l.width-2-lipgloss.Width(render(c, "")))))
			continue
		}
		cell := render(c, c.word)
		rows = append(rows, "  "+cell+strings.Repeat(" ", stateW-lipgloss.Width(cell))+"  "+m.fitMiddle(c.label, pathW))
	}
	return rows
}
