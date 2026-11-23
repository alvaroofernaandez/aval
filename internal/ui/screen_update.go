package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	coreupdate "github.com/alvaroofernaandez/aval/internal/core/update"
)

// The update view is the operation pattern on update.DryRun, inline: opening
// fetches (a network wait with the spinner, esc cancels it) and lists what
// the pull would bring; the confirm rows sit under it with the cursor on the
// action. The run pulls (network, esc cancels and returns to the preview),
// then hands over to the hosted installer, titled "Re-applying", which links
// what is new exactly as `aval update` does.
//
// A dirty tree is refused with its fix and no action. Up to date still offers
// the re-apply, as the CLI always re-applies. When the binary itself is
// behind, the command that updates it (update.commandFor: npm for an npm
// install) is shown, never run.

const updateSafety = "Files in the way move to ~/.aval-backup first · undo: aval restore latest"

// updatePlan is what opening the view found.
type updatePlan struct {
	coreupdate.Preview
	// dirty: DryRun refused the uncommitted tree; there is nothing to run.
	dirty bool
	// binary is the cached notice for the binary itself, from the last
	// update check; never fetched here.
	binary coreupdate.Notice
}

// updateView hosts the operation and hands a finished pull over to the
// re-apply flow. Once handed over it pops itself when the flow returns, so
// esc from the re-apply's done screen lands on the hub.
type updateView struct {
	op     operation[updatePlan]
	svc    Services
	sep    string
	handed bool
}

func newUpdateView(svc Services) screen {
	o, err := newOperation(operationSpec[updatePlan]{
		Kind: opNetwork,
		Mode: confirmInline,
		Confirm: confirmSpec{
			Action: "Update and re-apply",
			Safety: updateSafety,
		},
		ActionFor: func(p updatePlan) string {
			if p.Behind > 0 {
				return "Update and re-apply"
			}
			return "Re-apply links anyway"
		},
		NoteFor: func(p updatePlan) string {
			if p.Behind > 0 {
				return "pull with --ff-only, then link what is new"
			}
			return "link what is missing, pull nothing"
		},
		Ready: func(p updatePlan) bool { return !p.dirty },
		Preview: func(ctx context.Context) (updatePlan, error) {
			return loadUpdate(ctx, svc)
		},
		Run: func(ctx context.Context, p updatePlan) (opResult, error) {
			return runUpdatePull(ctx, svc, p)
		},
		PreviewBody: func(c chrome, l layout, p updatePlan) []string {
			return updateBody(c, l, svc, p)
		},
		Loading: "Fetching origin",
		Running: "Pulling",
		Motion:  true,
	})
	if err != nil {
		panic(err)
	}
	return updateView{op: o, svc: svc}
}

// loadUpdate is the dry run. A dirty tree comes back from DryRun as an error
// with DirtyTree set; it is a state of the view, not a failure.
func loadUpdate(ctx context.Context, svc Services) (updatePlan, error) {
	if svc.UpdatePreview == nil {
		return updatePlan{}, errors.New("update is not available here: no repository was found")
	}
	var plan updatePlan
	if svc.CachedNotice != nil {
		plan.binary = svc.CachedNotice()
	}
	p, err := svc.UpdatePreview(ctx)
	plan.Preview = p
	if err != nil && p.DirtyTree {
		plan.dirty = true
		return plan, nil
	}
	return plan, err
}

// runUpdatePull pulls when the repo is behind. Its summary is the lead of the
// re-apply's done headline.
func runUpdatePull(ctx context.Context, svc Services, p updatePlan) (opResult, error) {
	if p.Behind == 0 {
		return opResult{Summary: "Re-applied."}, nil
	}
	if svc.Pull == nil {
		return opResult{}, errors.New("update is not available here: no repository was found")
	}
	s, err := svc.Pull(ctx)
	if err != nil {
		return opResult{}, err
	}
	if !s.Updated {
		return opResult{Summary: fmt.Sprintf("Already up to date (%s).", s.Current)}, nil
	}
	return opResult{Summary: fmt.Sprintf("Updated %s → %s.", s.Current, s.Latest)}, nil
}

func (u updateView) init() tea.Cmd { return u.op.init() }

func (u updateView) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		u.sep = msg.c.sep()
	case resumeMsg:
		if u.handed {
			return u, pop()
		}
	}
	next, cmd := u.op.update(msg)
	u.op = next.(operation[updatePlan])
	if u.op.phase != phaseResult || u.op.runErr != nil || u.handed {
		return u, cmd
	}
	// The pull landed: hand over to the re-apply.
	if u.svc.Reapply == nil {
		u.op.runErr = errors.New("re-apply is not available here: no repository was found")
		return u, cmd
	}
	m, err := u.svc.Reapply()
	if err != nil {
		u.op.runErr = err
		return u, cmd
	}
	u.handed = true
	lead := strings.ReplaceAll(u.op.result.Summary, "→", u.op.c.arrow())
	return u, tea.Batch(cmd, push(newFlow(m.reapplying(lead), nil)))
}

func (u updateView) keys() keyMap   { return u.op.keys() }
func (u updateView) busy() busyKind { return u.op.busy() }

func (u updateView) view() view {
	v := u.op.view()
	if u.sep != "" && u.sep != " · " {
		v.status = strings.ReplaceAll(v.status, " · ", u.sep)
	}
	return v
}

// arrow is "→", or "->" in the ASCII tier.
func (c chrome) arrow() string {
	if c.theme.Glyphs.Cursor == ASCIIGlyphs.Cursor {
		return "->"
	}
	return "→"
}

// updateBody is the heading, the repo's state and, when the binary itself is
// behind, the command that updates it.
func updateBody(c chrome, l layout, svc Services, p updatePlan) []string {
	repo := tildeUnder(svc.Home, svc.Repo)
	out := c.headingRow(l, "Update", c.theme.Strong, repo, c.theme.Muted)
	out = append(out, "")
	switch {
	case p.dirty:
		glyphW := lipgloss.Width(c.theme.Glyphs.Fail) + 1
		for i, s := range c.wrap("the repository has uncommitted changes", max(l.width-glyphW, 1)) {
			if i == 0 {
				out = append(out, c.theme.State(StateFail, s))
				continue
			}
			out = append(out, strings.Repeat(" ", glyphW)+c.theme.Text.Render(s))
		}
		for _, s := range c.wrap("commit or stash them in "+repo, max(l.width-2, 1)) {
			out = append(out, "  "+c.theme.Text.Render(s))
		}
	case p.Behind == 0:
		out = append(out, c.theme.State(StateOK, c.fit(fmt.Sprintf("up to date (%s)", p.Current), max(l.width-2, 1))))
	default:
		out = append(out, updateChanges(c, l, svc.Home, p)...)
	}
	if b := updateBinaryLines(c, l, p.binary); len(b) > 0 {
		out = append(append(out, ""), b...)
	}
	return out
}

// updateChanges is the behind count with the two SHAs, the managed paths
// that change with their diffstat, and the manifest entries added or removed.
func updateChanges(c chrome, l layout, home string, p updatePlan) []string {
	noun := "commits"
	if p.Behind == 1 {
		noun = "commit"
	}
	shas := p.Current + " " + c.arrow() + " " + p.Latest
	out := c.headingRow(l, fmt.Sprintf("%d %s behind origin", p.Behind, noun), c.theme.Text, shas, c.theme.Muted)
	if stat := parseDiffstat(p.Stat); len(stat) > 0 {
		out = append(out, "", c.theme.Strong.Render(c.fit("Managed paths that change", l.width)))
		out = append(out, diffstatLines(c, l, stat)...)
	} else {
		out = append(out, "", c.theme.Muted.Render(c.fit("No managed path changes", l.width)))
	}
	entries := func(title, mark string, list []string) {
		if len(list) == 0 {
			return
		}
		out = append(out, c.theme.Strong.Render(c.fit(title, l.width)))
		for _, e := range list {
			text := mark + " " + manifestEntry(home, e)
			out = append(out, "  "+c.theme.Text.Render(c.fitMiddle(text, max(l.width-2, 1))))
		}
	}
	entries("New manifest entries", "+", p.Added)
	entries("Removed manifest entries", "-", p.Removed)
	return out
}

// manifestEntry shows "source -> dest" with dest under ~, as linked.
func manifestEntry(home, e string) string {
	src, dest, ok := strings.Cut(e, " -> ")
	if !ok {
		return e
	}
	switch {
	case strings.HasPrefix(dest, "~"):
	case filepath.IsAbs(dest):
		dest = tildeUnder(home, dest)
	default:
		dest = "~/" + dest
	}
	return src + " -> " + dest
}

// statRow is one path of `git diff --stat`: the path and "12 +++--".
type statRow struct{ path, count, bars string }

// parseDiffstat reads `git diff --stat`, dropping the summary line.
func parseDiffstat(stat string) []statRow {
	var rows []statRow
	for _, line := range strings.Split(stat, "\n") {
		i := strings.LastIndex(line, " | ")
		if i < 0 {
			continue
		}
		path := strings.TrimSpace(line[:i])
		count, bars, _ := strings.Cut(strings.TrimSpace(line[i+3:]), " ")
		rows = append(rows, statRow{path, count, strings.TrimSpace(bars)})
	}
	return rows
}

// diffstatLines aligns the paths and right-aligns the counts, as git does.
// Paths give way first, truncated in the middle.
func diffstatLines(c chrome, l layout, rows []statRow) []string {
	pathW, countW, barsW := 0, 0, 0
	for _, r := range rows {
		pathW = max(pathW, lipgloss.Width(r.path))
		countW = max(countW, len(r.count))
		barsW = max(barsW, len(r.bars))
	}
	const lead = 2
	statW := countW + 1 + barsW
	room := l.width - lead - 2 - statW
	var out []string
	for _, r := range rows {
		if room < 8 {
			// Too narrow for columns: the path, then its stat under it.
			out = append(out, "  "+c.theme.Text.Render(c.fitMiddle(r.path, max(l.width-lead, 1))))
			out = append(out, "    "+c.theme.Muted.Render(c.fit(r.count+" "+r.bars, max(l.width-4, 1))))
			continue
		}
		w := min(pathW, room)
		cell := c.pad(c.fitMiddle(r.path, w), w)
		stat := fmt.Sprintf("%*s %s", countW, r.count, r.bars)
		out = append(out, strings.TrimRight("  "+c.theme.Text.Render(cell)+"  "+c.theme.Muted.Render(stat), " "))
	}
	return out
}

// updateBinaryLines is the notice for the binary itself, from the cache: its
// version and the command that updates it. The command is shown, never run.
// `aval update` is this view, so it is not named.
func updateBinaryLines(c chrome, l layout, n coreupdate.Notice) []string {
	if n.String() == "" {
		return nil
	}
	out := []string{c.theme.State(StateWarn, c.fit("aval "+n.Latest+" available", max(l.width-2, 1)))}
	if n.Command == "" || n.Command == "aval update" {
		return out
	}
	for _, s := range c.wrap("update the binary with "+n.Command, max(l.width-2, 1)) {
		out = append(out, "  "+c.theme.Text.Render(s))
	}
	return out
}

func init() { registerView(viewUpdate, newUpdateView) }
