package ui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// The install screen: one action per update cycle, so the bar moves. Each
// finished action joins the log as it happens, failures included, and the
// step running now closes it. ctrl+c or q asks for a stop: the step in flight
// finishes and the rest are skipped.

// applyNext applies one action per cycle, so the UI repaints between them and
// the progress bar actually moves. Doing the whole run in one command would
// freeze the frame until it finished. A pending stop ends the run here, once
// the step in flight has reported back.
func (m Model) applyNext() tea.Cmd {
	if m.stopping || m.index >= len(m.queue) {
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
	status := m.tr.T("install.status.none_failed", nil)
	if n := len(m.errs); n > 0 {
		status = m.tr.T("install.status.failed", i18n.Args{"count": n})
	}
	if m.stopping {
		status += " · " + m.tr.T("install.status.stopping", nil)
	}
	return view{body: m.installBody, status: status}
}

func (m Model) installBody(l layout, h int) []string {
	done, total := m.index, len(m.queue)
	title := m.tr.T("install.title", nil)
	if m.runTitle != "" {
		title = m.runTitle
	}
	out := m.headingRow(l, title, m.theme.Heading, m.tr.T("install.progress", i18n.Args{"done": done, "total": total}), m.theme.Muted)
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

// installRows is the log, "√ linked  ~/.config/yazi" or
// "× failed  ~/.config/ghostty  source missing", then the step running now.
func (m Model) installRows(l layout) []string {
	g := m.theme.Glyphs
	// A cell is a state kind and its word; the running step is the one cell
	// that is not a state, so it renders through the key token instead.
	type cell struct {
		kind    StateKind
		word    string
		running bool
		label   string
		reason  string
	}
	render := func(c cell, word string) string {
		if c.running {
			return m.theme.Key.Render(g.Cursor + " " + word)
		}
		return m.theme.State(c.kind, word)
	}
	var cells []cell
	for _, s := range m.log {
		r := s.result
		c := cell{kind: StateOK, word: m.statusWord(r.Status), label: r.Action.Label}
		switch {
		case s.err != nil:
			c.kind, c.word, c.reason = StateFail, m.tr.T("install.word.failed", nil), failReason(s.err)
		case r.Backup != "":
			c.kind, c.word = StateWarn, m.tr.T("install.word.backed_up", nil)
		case r.Status == install.StatusWould:
			c.kind = StateWarn
		}
		cells = append(cells, c)
	}
	if m.index < len(m.queue) {
		verb := m.tr.T("install.word.copying", nil)
		switch {
		case m.DryRun:
			verb = m.tr.T("install.word.checking", nil)
		case m.Symlinks:
			verb = m.tr.T("install.word.linking", nil)
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
		state := render(c, c.word)
		rows = append(rows, "  "+state+strings.Repeat(" ", stateW-lipgloss.Width(state))+"  "+m.pathAndReason(c.label, c.reason, pathW))
	}
	return rows
}

// statusWord is the word for a completed step, in the language.
func (m Model) statusWord(s install.Status) string {
	switch s {
	case install.StatusLinked:
		return m.tr.T("install.status.linked", nil)
	case install.StatusCopied:
		return m.tr.T("install.status.copied", nil)
	case install.StatusAlready:
		return m.tr.T("install.status.already", nil)
	case install.StatusWould:
		return m.tr.T("install.status.would", nil)
	}
	return m.tr.T("install.status.unknown", nil)
}

// pathAndReason fits "path  reason" into w columns. The path keeps at least
// half the room; the reason is cut first, and dropped when nothing of it fits.
func (m Model) pathAndReason(path, reason string, w int) string {
	if reason == "" {
		return m.fitMiddle(path, w)
	}
	pathW := min(lipgloss.Width(path), max(w/2, w-2-lipgloss.Width(reason)))
	if reasonW := w - pathW - 2; reasonW >= 4 {
		return m.fitMiddle(path, pathW) + "  " + m.fit(reason, reasonW)
	}
	return m.fitMiddle(path, w)
}

// failReason is the short form of a failed action's error: install's own
// reason when it has one, the full message otherwise.
func failReason(err error) string {
	var ie *install.Error
	if errors.As(err, &ie) && ie.Reason != "" {
		return ie.Reason
	}
	return err.Error()
}
