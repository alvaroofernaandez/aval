package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/install"
)

// The done screen: evidence of what happened (counts, each failure with its
// reason), the way back (the backup path and `aval restore latest`) and what
// to do next. Under a short terminal it gives up, in order: next steps, the
// counts, the undo line, and only then failures; the headline always stays.

func (m Model) doneAction(act action) (tea.Model, tea.Cmd) {
	if act == actClose {
		m.Quitting = true
		return m, m.exit()
	}
	return m, nil
}

func (m Model) doneView() view {
	return view{body: m.doneBody}
}

func (m Model) doneBody(l layout, h int) []string {
	return m.doneContent(l, h, m.Stopped())
}

// userShell names the user's login shell ("zsh", "bash", ...); a variable so
// tests do not depend on the machine.
var userShell = func() string {
	return filepath.Base(os.Getenv("SHELL"))
}

// reloadHint is the command that reloads the user's shell, or "" when the
// shell is not one we know how to reload.
func reloadHint() (cmd, desc string) {
	switch sh := userShell(); sh {
	case "zsh", "bash", "fish":
		return "exec " + sh, "reload your shell to pick up the new config"
	}
	return "", ""
}

// doneGroup is a block of lines kept or dropped as a unit; lines inside
// groups with a tier above their first are dropped one by one from the end.
type doneGroup struct {
	lines []string
}

func (m Model) doneContent(l layout, h int, stopped bool) []string {
	var linked, already, would, backups int
	for _, r := range m.results {
		switch r.Status {
		case install.StatusLinked, install.StatusCopied:
			linked++
		case install.StatusAlready:
			already++
		case install.StatusWould:
			would++
		}
		if r.Backup != "" {
			backups++
		}
	}
	failed := len(m.errs)
	applied, total := m.Progress()
	written := !m.DryRun && (linked > 0 || backups > 0 || (stopped && applied > 0))

	// Headline: bold ink. Colour belongs to state glyphs only (DESIGN.md 10).
	var headline string
	switch {
	case m.DryRun:
		headline = m.theme.Heading.Render(m.fit("Dry run complete: nothing was written.", l.width))
	case stopped:
		headline = m.theme.Heading.Render(m.fit(fmt.Sprintf("Stopped after %d of %d.", applied, total), l.width))
	case m.doneLead != "" && failed > 0:
		headline = m.theme.Heading.Render(m.fit(fmt.Sprintf("%s %d linked, %d failed.", m.doneLead, linked, failed), l.width))
	case m.doneLead != "":
		headline = m.theme.Heading.Render(m.fit(fmt.Sprintf("%s %d linked, %d already in place.", m.doneLead, linked, already), l.width))
	case failed > 0:
		headline = m.theme.Heading.Render(m.fit(fmt.Sprintf("Installed. %d linked, %d failed.", linked, failed), l.width))
	default:
		headline = m.theme.Heading.Render(m.fit(fmt.Sprintf("Installed. %d linked.", linked), l.width))
	}

	// Failures: the path and the reason, verbatim.
	var failures []string
	for _, err := range m.errs {
		failures = append(failures, "  "+m.theme.Failure.Render(m.fit(err.Error(), l.width-2)))
	}

	// Counts.
	row := func(label string, n int) string {
		return m.theme.Body.Render(m.fit(fmt.Sprintf("  %-17s %3d", label, n), l.width))
	}
	var counts []string
	switch {
	case m.DryRun:
		counts = append(counts, row("would change", would))
	default:
		counts = append(counts, row("linked", linked), row("already in place", already))
		if failed > 0 {
			counts = append(counts, row("failed", failed))
		}
	}

	// Moved files and the undo line travel together: the user needs to know
	// where the old files went and how to get back.
	var undo, moved []string
	if backups > 0 {
		noun := "files"
		if backups == 1 {
			noun = "file"
		}
		prefix := fmt.Sprintf("%d existing %s moved to ", backups, noun)
		// On one line when the whole path fits; otherwise the path gets its own
		// line and is cut from the middle.
		root := m.runner.BackupRoot()
		if avail := l.width - 2 - len(prefix); lipgloss.Width(root) <= avail {
			moved = append(moved, "  "+m.theme.Warning.Render(prefix)+m.theme.Muted.Render(root))
		} else {
			moved = append(moved,
				"  "+m.theme.Warning.Render(m.fit(strings.TrimSpace(prefix), l.width-2)),
				"  "+m.theme.Muted.Render(m.fitMiddle(root, l.width-2)))
		}
	}
	if written {
		const lead = "Undo everything with: "
		const cmd = "aval restore latest"
		if l.width >= len(lead)+len(cmd)+2 {
			undo = append(undo, "  "+m.theme.Text.Render(lead)+m.theme.Key.Render(cmd))
		} else {
			undo = append(undo, "  "+m.theme.Key.Render(m.fit(cmd, l.width-2)))
		}
	}

	// Next steps, only after a finished install.
	var next []string
	if !m.DryRun && !stopped {
		type step struct{ cmd, desc string }
		steps := []step{
			{"aval doctor", "check this machine against the repo"},
			{"aval config", "theme, glyphs and update checks"},
		}
		if c, d := reloadHint(); c != "" {
			steps = append(steps, step{c, d})
		}
		next = append(next, m.theme.Strong.Render("  Next"))
		for _, s := range steps {
			line := "    " + m.theme.Key.Render(m.pad(s.cmd, 13))
			if room := l.width - 4 - 13 - 1; room >= 8 {
				line += " " + m.theme.Muted.Render(m.fit(s.desc, room))
			}
			next = append(next, line)
		}
	}

	// Keep groups in priority order while they fit; blank lines separate the
	// shown blocks. Failures are taken line by line, and the rest all or
	// nothing.
	type block struct {
		failures, counts, moved, undo, next bool
		nFail                               int
	}
	var b block
	assemble := func(b block, gaps bool) []string {
		out := []string{headline}
		var blocks [][]string
		var main []string
		if b.counts {
			main = append(main, counts...)
		}
		if b.failures {
			main = append(main, failures[:b.nFail]...)
		}
		blocks = append(blocks, main)
		var rec []string
		if b.moved {
			rec = append(rec, moved...)
		}
		if b.undo {
			rec = append(rec, undo...)
		}
		blocks = append(blocks, rec)
		if b.next {
			blocks = append(blocks, next)
		}
		for _, blk := range blocks {
			if len(blk) > 0 {
				if gaps {
					out = append(out, "")
				}
				out = append(out, blk...)
			}
		}
		return out
	}
	// Blank lines between blocks go first when height is short.
	fits := func(b block) bool { return len(assemble(b, false)) <= h }

	for n := 1; n <= len(failures); n++ {
		try := b
		try.failures, try.nFail = true, n
		if !fits(try) {
			break
		}
		b = try
	}
	for _, step := range []func(block) block{
		func(b block) block { b.undo = len(undo) > 0; return b },
		func(b block) block { b.counts = true; return b },
		func(b block) block { b.moved = len(moved) > 0; return b },
		func(b block) block { b.next = len(next) > 0; return b },
	} {
		if try := step(b); fits(try) {
			b = try
		}
	}
	out := assemble(b, true)
	if len(out) > h {
		out = assemble(b, false)
	}
	if len(out) > h {
		out = out[:max(h, 1)]
	}
	return out
}
