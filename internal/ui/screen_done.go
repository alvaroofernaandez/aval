package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/install"
)

// The done screen: what happened and where the backup is. The counts
// headline, the undo line, next steps and the dry-run and stopped wording
// belong here (T10); the status line is free for the undo command.

func (m Model) doneAction(act action) (tea.Model, tea.Cmd) {
	if act == actClose {
		m.Quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) doneView() view {
	return view{body: m.doneBody}
}

func (m Model) doneBody(l layout, _ int) []string {
	counts := map[install.Status]int{}
	backups := 0
	for _, r := range m.results {
		counts[r.Status]++
		if r.Backup != "" {
			backups++
		}
	}

	var out []string
	switch {
	case len(m.errs) > 0:
		out = append(out, m.theme.Failure.Render(m.fit(fmt.Sprintf("Finished with %d error(s)", len(m.errs)), l.width)))
	case m.DryRun:
		out = append(out, m.theme.Warning.Render(m.fit("Dry run complete: nothing was written", l.width)))
	default:
		out = append(out, m.theme.Success.Render("Done"))
	}
	out = append(out, "")

	for _, s := range []install.Status{install.StatusLinked, install.StatusCopied, install.StatusAlready, install.StatusWould} {
		if counts[s] > 0 {
			out = append(out, m.theme.Body.Render(m.fit(fmt.Sprintf("  %-18s %d", s.String(), counts[s]), l.width)))
		}
	}

	// Named explicitly, because it is the one thing the user may need to act
	// on: their previous config is in there, not gone.
	if backups > 0 {
		out = append(out, "",
			m.theme.Warning.Render(m.fit(fmt.Sprintf("  %d existing file(s) moved to:", backups), l.width)),
			"  "+m.theme.Muted.Render(m.fitMiddle(m.runner.BackupRoot(), l.width-2)))
	}

	if len(m.errs) > 0 {
		out = append(out, "")
	}
	for _, err := range m.errs {
		out = append(out, "  "+m.theme.Failure.Render(m.fit(err.Error(), l.width-2)))
	}
	return out
}
