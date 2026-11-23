package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/core/platform"
)

// Screen is which of the three views is showing.
type Screen int

const (
	ScreenSelect  Screen = iota // choose what to install
	ScreenInstall               // work in progress
	ScreenDone                  // summary
)

// UnavailableGroup is a manifest group that does not apply to this machine.
// They are shown rather than hidden: someone on Windows should see that the
// terminal environment exists and why it is not offered, instead of wondering
// what is missing.
type UnavailableGroup struct {
	Label  string
	Reason string
}

// appliedMsg reports one finished action back into the update loop.
type appliedMsg struct {
	result install.Result
	err    error
}

// doneMsg marks the end of the run.
type doneMsg struct{}

// Model is the installer UI.
type Model struct {
	theme    Theme
	platform platform.Platform
	plan     *plan.Plan
	runner   *install.Runner

	Unavailable []UnavailableGroup
	Symlinks    bool
	DryRun      bool

	screen Screen
	cursor int

	queue   []plan.Action
	index   int
	results []install.Result
	errs    []error

	width int
	// Quitting records a deliberate exit, so main can tell "the user pressed q"
	// apart from "the install finished".
	Quitting bool
}

// NewModel builds the initial state.
func NewModel(p *plan.Plan, pf platform.Platform, r *install.Runner, symlinks, dryRun bool) Model {
	return Model{
		theme:    DefaultTheme(),
		platform: pf,
		plan:     p,
		runner:   r,
		Symlinks: symlinks,
		DryRun:   dryRun,
		screen:   ScreenSelect,
		width:    80,
	}
}

// WithTheme returns m rendering with t. Tests use it to pin a deterministic
// theme; callers that know the terminal use it to apply their detected theme.
func (m Model) WithTheme(t Theme) Model {
	m.theme = t
	return m
}

// Screen exposes the current view, for tests and for main's exit code.
func (m Model) Screen() Screen { return m.screen }

// Results exposes what was applied.
func (m Model) Results() []install.Result { return m.results }

// Errs exposes failures. A non-empty slice means a non-zero exit.
func (m Model) Errs() []error { return m.errs }

func (m Model) Init() tea.Cmd { return nil }

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case appliedMsg:
		if msg.err != nil {
			m.errs = append(m.errs, msg.err)
		} else {
			m.results = append(m.results, msg.result)
		}
		m.index++
		return m, m.applyNext()

	case doneMsg:
		m.screen = ScreenDone
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		// Quitting mid-install would leave the run half-applied with no
		// summary, so it is only offered where it is safe.
		if m.screen != ScreenInstall {
			m.Quitting = true
			return m, tea.Quit
		}
		return m, nil
	}

	switch m.screen {
	case ScreenSelect:
		return m.handleSelectKey(msg)
	case ScreenDone:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.Quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) handleSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.plan.Groups)-1 {
			m.cursor++
		}
	case " ", "x":
		if len(m.plan.Groups) > 0 {
			m.plan.Groups[m.cursor].Selected = !m.plan.Groups[m.cursor].Selected
		}
	case "a":
		for i := range m.plan.Groups {
			m.plan.Groups[i].Selected = true
		}
	case "n":
		for i := range m.plan.Groups {
			m.plan.Groups[i].Selected = false
		}
	case "enter":
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

// View renders the current screen.
func (m Model) View() string {
	switch m.screen {
	case ScreenInstall:
		return m.viewInstall()
	case ScreenDone:
		return m.viewDone()
	default:
		return m.viewSelect()
	}
}

func (m Model) header() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("aval"))
	b.WriteString("\n")
	b.WriteString(m.theme.Subtitle.Render(m.fit(m.platform.Display(), m.width)))
	if m.DryRun {
		note := "  " + m.theme.Glyphs.Sep + "  dry run: nothing will be written"
		b.WriteString(m.theme.Warning.Render(m.fit(note, m.width-lipgloss.Width(m.platform.Display()))))
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Muted.Render(m.fit(m.platform.InstallStyle(m.Symlinks), m.width)))
	b.WriteString("\n\n")
	return b.String()
}

func (m Model) viewSelect() string {
	var b strings.Builder
	b.WriteString(m.header())

	selected := 0
	for _, g := range m.plan.Groups {
		if g.Selected {
			selected += len(g.Actions)
		}
	}

	b.WriteString(m.theme.Heading.Render("What to install"))
	b.WriteString("\n\n")

	for i, g := range m.plan.Groups {
		box := m.theme.Unchecked.Render(m.theme.Glyphs.Unchecked)
		if g.Selected {
			box = m.theme.Checked.Render(m.theme.Glyphs.Checked)
		}
		marker := "  "
		if i == m.cursor {
			marker = m.theme.Marker.Render(m.theme.Glyphs.Cursor + " ")
		}

		count := fmt.Sprintf("%d items", len(g.Actions))
		// marker (2) + box (3) + two spaces + count must fit beside the label.
		labelW := m.width - 2 - lipgloss.Width(box) - 2 - len(count)
		if labelW > 22 {
			labelW = 22
		}
		line := fmt.Sprintf("%s %s %s", box, m.pad(g.Label, labelW), m.theme.Muted.Render(count))
		if i == m.cursor {
			line = m.theme.Selected.Render(line)
		}
		b.WriteString(marker + line + "\n")
		if g.Detail != "" {
			b.WriteString("      " + m.theme.Muted.Render(m.fit(g.Detail, m.width-6)) + "\n")
		}
	}

	for _, u := range m.Unavailable {
		row := fmt.Sprintf("%s %-22s %s", m.theme.Glyphs.Partial, u.Label, u.Reason)
		b.WriteString("  " + m.theme.Unavailable.Render(m.fit(row, m.width-2)) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(m.theme.Body.Render(fmt.Sprintf("%d items selected", selected)))
	b.WriteString("\n\n")
	b.WriteString(m.help(
		"↑/↓", "move", "space", "toggle", "a", "all", "n", "none", "enter", "install", "q", "quit",
	))
	return b.String()
}

func (m Model) viewInstall() string {
	var b strings.Builder
	b.WriteString(m.header())

	done, total := m.index, len(m.queue)
	width := m.width - 20
	if width > 48 {
		width = 48
	}
	if width < 10 {
		width = 10
	}

	b.WriteString(m.theme.Heading.Render("Installing"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.ProgressBar(done, total, width))
	b.WriteString(m.theme.Body.Render(fmt.Sprintf("  %d/%d", done, total)))
	b.WriteString("\n\n")

	// The last few lines only: a full log scrolls the frame and hides the bar.
	start := len(m.results) - 6
	if start < 0 {
		start = 0
	}
	for _, r := range m.results[start:] {
		b.WriteString("  " + m.statusLine(r) + "\n")
	}
	return b.String()
}

func (m Model) viewDone() string {
	var b strings.Builder
	b.WriteString(m.header())

	counts := map[install.Status]int{}
	backups := 0
	for _, r := range m.results {
		counts[r.Status]++
		if r.Backup != "" {
			backups++
		}
	}

	if len(m.errs) > 0 {
		b.WriteString(m.theme.Failure.Render(m.fit(fmt.Sprintf("Finished with %d error(s)", len(m.errs)), m.width)))
	} else if m.DryRun {
		b.WriteString(m.theme.Warning.Render(m.fit("Dry run complete: nothing was written", m.width)))
	} else {
		b.WriteString(m.theme.Success.Render("Done"))
	}
	b.WriteString("\n\n")

	for _, s := range []install.Status{install.StatusLinked, install.StatusCopied, install.StatusAlready, install.StatusWould} {
		if counts[s] > 0 {
			b.WriteString(m.theme.Body.Render(m.fit(fmt.Sprintf("  %-18s %d", s.String(), counts[s]), m.width)) + "\n")
		}
	}

	// Named explicitly, because it is the one thing the user may need to act
	// on: their previous config is in there, not gone.
	if backups > 0 {
		b.WriteString("\n")
		b.WriteString(m.theme.Warning.Render(m.fit(fmt.Sprintf("  %d existing file(s) moved to:", backups), m.width)))
		b.WriteString("\n  " + m.theme.Muted.Render(m.fitMiddle(m.runner.BackupRoot(), m.width-2)) + "\n")
	}

	for _, err := range m.errs {
		b.WriteString("\n  " + m.theme.Failure.Render(m.fit(err.Error(), m.width-2)))
	}

	b.WriteString("\n\n")
	b.WriteString(m.help("enter", "close"))
	return b.String()
}

func (m Model) statusLine(r install.Result) string {
	style := m.theme.Success
	switch r.Status {
	case install.StatusAlready:
		style = m.theme.Muted
	case install.StatusWould:
		style = m.theme.Warning
	}
	status := r.Status.String()
	// Two columns of indent, one gap, then the status: the label gives way.
	labelW := m.width - 2 - 1 - lipgloss.Width(status)
	if labelW > 40 {
		labelW = 40
	}
	if labelW < 1 {
		// No room for a label at all: the status is the part that matters.
		return style.Render(m.fit(status, m.width-2))
	}
	line := fmt.Sprintf("%s %s", m.pad(r.Action.Label, labelW), style.Render(status))
	if r.Backup != "" {
		const note = "  (backed up)"
		if lipgloss.Width(line)+len(note)+2 <= m.width {
			line += m.theme.Warning.Render(note)
		}
	}
	return line
}

// help renders alternating key/description pairs.
func (m Model) help(pairs ...string) string {
	sep := m.theme.Muted.Render("  " + m.theme.Glyphs.Sep + "  ")
	var lines []string
	line, lineW := "", 0
	for i := 0; i+1 < len(pairs); i += 2 {
		part := m.theme.Key.Render(pairs[i]) + " " + m.theme.Muted.Render(pairs[i+1])
		w := lipgloss.Width(part)
		// A hint that does not fit starts a new line instead of running past
		// the edge; the width includes the separator that would precede it.
		if line != "" && lineW+lipgloss.Width(sep)+w > m.width {
			lines = append(lines, line)
			line, lineW = "", 0
		}
		if line != "" {
			line += sep
			lineW += lipgloss.Width(sep)
		}
		line += part
		lineW += w
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n")
}

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
