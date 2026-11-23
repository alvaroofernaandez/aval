package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
)

// The backups view (plan section 3, Backups and restore): the runs under
// ~/.aval-backup newest first, what the cursor's run holds, `p` to pin, and
// enter to preview a restore. The restore itself is the operation pattern:
// preview, a destructive confirm on Cancel that names the run's .displaced
// folder, an atomic run, the result.

func init() { registerView(viewBackups, func(svc Services) screen { return newBackups(svc) }) }

const (
	actBackupsRestore action = -140 // enter: preview the restore
	actBackupsPin     action = -141 // p: pin or unpin the run

	backupsTimeLayout        = "2006-01-02 15:04"
	backupsCompactTimeLayout = "01-02 15:04"
	backupsGap               = 3
	backupsCursorW           = 2

	// restoreSafety is the safety net in full, in the confirm's prompt; the
	// status line has 76 columns, so it carries restoreSafetyShort.
	restoreSafety      = "Anything aval did not write moves to the run's .displaced folder, not deleted."
	restoreSafetyShort = "Anything aval did not write moves to .displaced, not deleted"
)

var errRestoreNoRepo = errors.New("Restoring needs the aval repository, to tell aval's links from yours, and none was found. Set it up with: aval install")

// runEntries is what one run holds, read once per run.
type runEntries struct {
	list []backup.Entry
	err  error
}

type backups struct {
	svc Services
	c   chrome

	runs    []backup.Run
	listErr error
	cursor  int
	// entries caches RunEntries by run id; a reload starts a fresh map.
	entries map[string]runEntries
	// note replaces the status line after a failed pin.
	note string
}

func newBackups(svc Services) backups {
	b := backups{svc: svc}
	b.load("")
	return b
}

// load reads the runs, newest first whatever order the service returns, and
// keeps the cursor on keep when it is still listed.
func (b *backups) load(keep string) {
	b.runs, b.listErr, b.entries = nil, nil, map[string]runEntries{}
	if b.svc.Backups != nil {
		b.runs, b.listErr = b.svc.Backups()
	}
	sort.SliceStable(b.runs, func(i, j int) bool {
		if !b.runs[i].Time.Equal(b.runs[j].Time) {
			return b.runs[i].Time.After(b.runs[j].Time)
		}
		return b.runs[i].ID > b.runs[j].ID
	})
	b.cursor = 0
	for i, r := range b.runs {
		if r.ID == keep {
			b.cursor = i
		}
	}
	b.readEntries()
}

// readEntries loads what the cursor's run holds, once. Legacy and pending
// runs have no list to show.
func (b *backups) readEntries() {
	r, ok := b.current()
	if !ok || r.Legacy || r.Pending || b.svc.RunEntries == nil {
		return
	}
	if _, done := b.entries[r.ID]; done {
		return
	}
	list, err := b.svc.RunEntries(r)
	b.entries[r.ID] = runEntries{list, err}
}

func (b backups) current() (backup.Run, bool) {
	if b.cursor < 0 || b.cursor >= len(b.runs) {
		return backup.Run{}, false
	}
	return b.runs[b.cursor], true
}

// restorable: the run has a manifest and finished, and this build can
// preview a restore. Without a repository enter still opens, and the
// preview explains what is missing.
func (b backups) restorable() bool {
	r, ok := b.current()
	return ok && !r.Legacy && !r.Pending && b.svc.RestorePreview != nil && b.svc.Restore != nil
}

func (b backups) init() tea.Cmd { return nil }

func (b backups) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		b.c = msg.c
	case resumeMsg:
		r, _ := b.current()
		b.load(r.ID)
	case tea.KeyMsg:
		act, ok := b.keys().lookup(msg.String())
		if !ok {
			return b, nil
		}
		switch act {
		case actUp:
			b.cursor = max(b.cursor-1, 0)
			b.readEntries()
		case actDown:
			b.cursor = min(b.cursor+1, max(len(b.runs)-1, 0))
			b.readEntries()
		case actBackupsRestore:
			r, _ := b.current()
			return b, push(newRestore(b.svc, r))
		case actBackupsPin:
			return b.pin(), nil
		}
	}
	return b, nil
}

// pin flips the cursor's run and reloads, so the row shows what is on disk.
func (b backups) pin() backups {
	r, _ := b.current()
	b.note = ""
	if err := b.svc.Pin(r.ID, !r.Pinned); err != nil {
		verb := "pin"
		if r.Pinned {
			verb = "unpin"
		}
		b.note = fmt.Sprintf("Could not %s %s: %v. Try: aval restore --%s %s", verb, r.ID, err, verb, r.ID)
		return b
	}
	b.load(r.ID)
	return b
}

func (b backups) keys() keyMap {
	var km keyMap
	if len(b.runs) > 1 {
		km = append(km,
			binding{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: "move"},
			binding{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: "move"},
		)
	}
	if b.restorable() {
		km = append(km, binding{keys: []string{"enter"}, action: actBackupsRestore, label: "enter", desc: "preview restore", core: true})
	}
	if len(b.runs) > 0 && b.svc.Pin != nil {
		km = append(km, binding{keys: []string{"p"}, action: actBackupsPin, label: "p", desc: "pin"})
	}
	return km
}

func (b backups) busy() busyKind { return busyIdle }

func (b backups) view() view {
	status := fmt.Sprintf("The newest %d unpinned runs are kept", backup.Retain)
	if b.note != "" {
		status = b.note
	}
	return view{body: b.body, status: status}
}

// storePath is the backup root as the heading shows it.
func (b backups) storePath() string {
	if b.svc.Home == "" {
		return "~/.aval-backup"
	}
	return tildeUnder(b.svc.Home, filepath.Join(b.svc.Home, ".aval-backup"))
}

func (b backups) tilde(p string) string { return tildeUnder(b.svc.Home, p) }

// body is the heading, the runs and the cursor's run. When h runs short the
// lower pane gives way first, then the list scrolls with the cursor visible.
func (b backups) body(l layout, h int) []string {
	c, t := b.c, b.c.theme
	count := fmt.Sprintf("%d runs", len(b.runs))
	if len(b.runs) == 1 {
		count = "1 run"
	}
	out := c.headingRow(l, "Backups", t.Strong, b.storePath()+c.sep()+count, t.Muted)
	out = append(out, "")

	switch {
	case b.listErr != nil:
		return append(out, failureLines(b.c, l, "could not read "+b.storePath(), b.listErr.Error(), "aval restore --list")...)
	case len(b.runs) == 0:
		for _, s := range c.wrap("No backup runs yet. Install, sync and uninstall make one before they write.", l.width) {
			out = append(out, t.Text.Render(s))
		}
		return out
	}

	// The pane gives way first: it shrinks to its header and an overflow
	// line, then goes, and only then does the list scroll.
	pane := b.pane(l)
	room := h - len(out)
	rows := min(len(b.runs), max(room, 1))
	switch paneRoom := room - rows - 1; {
	case paneRoom < 2:
		pane = nil
	case paneRoom < len(pane):
		hidden := len(pane) - (paneRoom - 1)
		more := fmt.Sprintf("%s %d more", t.Glyphs.Ellipsis, hidden)
		pane = append(pane[:paneRoom-1], "  "+t.Muted.Render(c.fit(more, max(l.width-2, 1))))
	}
	start, end := scrollWindow(len(b.runs), b.cursor, rows)
	cols := b.columns(l)
	for i := start; i < end; i++ {
		out = append(out, b.row(l, cols, b.runs[i], i == b.cursor))
	}
	if len(pane) > 0 {
		out = append(out, "")
		out = append(out, pane...)
	}
	return out
}

// runCols are the list's columns for the current width.
type runCols struct {
	timeLayout     string
	gap            int
	origin, digits int
	countW         int
	// short drops the word after the entry count and shortens the flags.
	short bool
}

// columns picks the widest layout in which every row fits with its flags:
// the full one, then single gaps, then the year dropped, then short counts
// and flags. A row that still does not fit is cut with the ellipsis.
func (b backups) columns(l layout) runCols {
	base := runCols{timeLayout: backupsTimeLayout, gap: backupsGap}
	for _, r := range b.runs {
		base.origin = max(base.origin, lipgloss.Width(originOf(r)))
		base.digits = max(base.digits, len(fmt.Sprint(r.Entries)))
	}
	tiers := []func(runCols) runCols{
		func(c runCols) runCols { return c },
		func(c runCols) runCols { c.gap = 1; return c },
		func(c runCols) runCols { c.timeLayout = backupsCompactTimeLayout; return c },
		func(c runCols) runCols { c.short = true; return c },
	}
	cols := base
	for _, tier := range tiers {
		cols = tier(cols)
		cols.countW = 0
		for _, r := range b.runs {
			cols.countW = max(cols.countW, lipgloss.Width(b.countCell(cols, r)))
		}
		fits := true
		for _, r := range b.runs {
			cells, flags, _ := b.cells(cols, r)
			if backupsCursorW+lipgloss.Width(rowText(cols, cells, flags)) > l.width {
				fits = false
				break
			}
		}
		if fits {
			break
		}
	}
	return cols
}

func originOf(r backup.Run) string {
	switch {
	case r.Origin != "":
		return r.Origin
	case r.Legacy:
		return "legacy"
	}
	return "unknown"
}

func (b backups) countCell(cols runCols, r backup.Run) string {
	switch {
	case r.Legacy:
		return ""
	case cols.short:
		return fmt.Sprintf("%*d", cols.digits, r.Entries)
	case r.Entries == 1:
		return fmt.Sprintf("%*d entry", cols.digits, r.Entries)
	}
	return fmt.Sprintf("%*d entries", cols.digits, r.Entries)
}

// cells is the row's plain columns, then its flags (pinned, interrupted,
// failed) both plain and styled.
func (b backups) cells(cols runCols, r backup.Run) (cells string, plain, styled []string) {
	c, t, g := b.c, b.c.theme, b.c.theme.Glyphs
	gap := strings.Repeat(" ", cols.gap)
	cells = r.Time.Format(cols.timeLayout) + gap + c.pad(originOf(r), cols.origin) + gap + c.pad(b.countCell(cols, r), cols.countW)
	pending, failed := "interrupted", "failed, rolled back"
	if cols.short {
		failed = "failed"
	}
	if r.Pinned {
		plain, styled = append(plain, "pinned"), append(styled, t.Text.Render("pinned"))
	}
	if r.Pending {
		plain, styled = append(plain, g.Warn+" "+pending), append(styled, t.State(StateWarn, pending))
	}
	if r.Failed {
		plain, styled = append(plain, g.Fail+" "+failed), append(styled, t.State(StateFail, failed))
	}
	return cells, plain, styled
}

// rowText joins the cells and the flags, without trailing padding.
func rowText(cols runCols, cells string, flags []string) string {
	if len(flags) == 0 {
		return strings.TrimRight(cells, " ")
	}
	gap := strings.Repeat(" ", cols.gap)
	return cells + gap + strings.Join(flags, gap)
}

// row is one run: absolute time, origin, entry count and its flags. Colour
// sits on the state glyphs only; the cursor row reverses its cells.
func (b backups) row(l layout, cols runCols, r backup.Run, selected bool) string {
	c, t := b.c, b.c.theme
	cells, plain, styled := b.cells(cols, r)
	marker := strings.Repeat(" ", backupsCursorW)
	cellStyle := t.Text
	if selected {
		marker = t.Marker.Render(c.pad(t.Glyphs.Cursor, backupsCursorW))
		cellStyle = t.Selected
	}
	room := max(l.width-backupsCursorW, 1)
	if text := rowText(cols, cells, plain); lipgloss.Width(text) > room {
		return marker + cellStyle.Render(c.fit(text, room))
	}
	if len(styled) == 0 {
		return marker + cellStyle.Render(strings.TrimRight(cells, " "))
	}
	gap := strings.Repeat(" ", cols.gap)
	return marker + cellStyle.Render(cells) + gap + strings.Join(styled, gap)
}

// pane is what the cursor's run holds, or why it has nothing to preview.
func (b backups) pane(l layout) []string {
	c, t := b.c, b.c.theme
	r, ok := b.current()
	if !ok {
		return nil
	}
	w := max(l.width-2, 1)
	note := func(k StateKind, s string) []string {
		var out []string
		for i, line := range c.wrap(s, max(w-2, 1)) {
			if i == 0 {
				out = append(out, "  "+t.State(k, line))
				continue
			}
			out = append(out, "    "+t.Text.Render(line))
		}
		return out
	}
	switch {
	case r.Legacy:
		return append([]string{t.Text.Render(c.fit(r.ID+" has no manifest", l.width))},
			note(StateWarn, "restore by hand from "+b.tilde(r.Dir))...)
	case r.Pending:
		return append([]string{t.Text.Render(c.fit(r.ID+" did not finish", l.width))},
			note(StateWarn, "interrupted, the next sync restores it")...)
	}
	out := []string{t.Text.Render(c.fit(r.ID+" holds", l.width))}
	e, read := b.entries[r.ID]
	switch {
	case !read:
		return append(out, "  "+t.Muted.Render(c.fit("entries unavailable here", w)))
	case e.err != nil:
		return append(out, failureLines(b.c, l, "could not read "+r.ID, e.err.Error(), "aval restore --list")...)
	case len(e.list) == 0:
		return append(out, "  "+t.Muted.Render(c.fit("no entries", w)))
	}
	kindW := 0
	pathW := 0
	for _, en := range e.list {
		kindW = max(kindW, lipgloss.Width(entryKind(en)))
		pathW = max(pathW, lipgloss.Width(b.tilde(en.Path)))
	}
	pathW = min(pathW, w-2-kindW)
	for _, en := range e.list {
		if pathW < 8 {
			out = append(out, "  "+t.Text.Render(c.fitMiddle(b.tilde(en.Path), w)))
			continue
		}
		out = append(out, "  "+t.Text.Render(c.pad(c.fitMiddle(b.tilde(en.Path), pathW), pathW))+"  "+t.Muted.Render(entryKind(en)))
	}
	return out
}

// entryKind is what a run saved for a path: the kind it had, or that aval
// made it.
func entryKind(e backup.Entry) string {
	switch {
	case e.CreatedDir:
		return "folder created by aval"
	case !e.Existed:
		return "created by aval"
	case e.Kind == backup.KindSymlink:
		return "link"
	case e.Kind == backup.KindDir:
		return "folder"
	}
	return "file"
}

// failureLines is an error as the design asks: what and where on the state
// line, the cause, then the command that helps.
func failureLines(c chrome, l layout, what, cause, command string) []string {
	t := c.theme
	var out []string
	for i, s := range c.wrap(what, max(l.width-2, 1)) {
		if i == 0 {
			out = append(out, t.State(StateFail, s))
			continue
		}
		out = append(out, "  "+t.Text.Render(s))
	}
	for _, s := range c.wrap(cause, max(l.width-2, 1)) {
		out = append(out, "  "+t.Text.Render(s))
	}
	if command != "" {
		for _, s := range c.wrap("Try: "+command, max(l.width-2, 1)) {
			out = append(out, "  "+t.Muted.Render(s))
		}
	}
	return out
}

// restoreFailure is a restore that stopped at path.
type restoreFailure struct {
	path string
	err  error
}

func (f restoreFailure) Error() string { return f.err.Error() }

// newRestore is the restore operation for run: preview with RestorePreview,
// confirm on Cancel, then Restore once, atomically.
func newRestore(svc Services, run backup.Run) screen {
	home := svc.Home
	tilde := func(p string) string { return tildeUnder(home, p) }
	o, err := newOperation(operationSpec[[]backup.Action]{
		Kind: opAtomic,
		Mode: confirmDestructive,
		Confirm: confirmSpec{
			ActionNote: "put the saved state back",
			Prompt:     fmt.Sprintf("This puts back what run %s saved. %s", run.ID, restoreSafety),
			Safety:     restoreSafetyShort,
		},
		ActionFor: func(p []backup.Action) string {
			if n := restoreWrites(p); n != 1 {
				return fmt.Sprintf("Restore %d paths", n)
			}
			return "Restore 1 path"
		},
		Ready: func(p []backup.Action) bool { return restoreWrites(p) > 0 },
		Preview: func(context.Context) ([]backup.Action, error) {
			if svc.Repo == "" {
				return nil, errRestoreNoRepo
			}
			return svc.RestorePreview(run)
		},
		Run: func(_ context.Context, _ []backup.Action) (opResult, error) {
			acts, err := svc.Restore(run)
			if err != nil {
				f := restoreFailure{err: err}
				if len(acts) > 0 {
					f.path = acts[len(acts)-1].Path
				}
				return opResult{}, f
			}
			return opResult{
				Summary: fmt.Sprintf("Restored backup %s. Anything aval did not write is in %s.", run.ID, tilde(filepath.Join(run.Dir, ".displaced"))),
				Counts:  restoreCounts(acts, true),
			}, nil
		},
		PreviewBody: func(c chrome, l layout, p []backup.Action) []string {
			return restorePreviewLines(c, l, run, p, tilde)
		},
		PreviewStatus: "Nothing has changed yet",
		Loading:       "Reading run " + run.ID,
		Running:       "Restoring " + run.ID,
		Result: func(c chrome, l layout, r opResult, err error) []string {
			if err == nil {
				return defaultResultLines(c, l, r)
			}
			what := "could not restore backup " + run.ID
			var f restoreFailure
			if errors.As(err, &f) && f.path != "" {
				what = "could not restore " + tilde(f.path)
			}
			cause := strings.ReplaceAll(err.Error(), home+string(filepath.Separator), "~"+string(filepath.Separator))
			if home == "" {
				cause = err.Error()
			}
			return failureLines(c, l, what, cause, "aval restore "+run.ID)
		},
	})
	if err != nil {
		// The spec above always names its safety net and its action.
		panic(err)
	}
	return o
}

// defaultResultLines is the operation's own result rendering, for a view
// whose Result override only handles the failure.
func defaultResultLines(c chrome, l layout, r opResult) []string {
	o := operation[struct{}]{c: c, result: r}
	return o.resultLines(l)
}

// restoreWrites counts the paths a restore would write.
func restoreWrites(p []backup.Action) int {
	n := 0
	for _, a := range p {
		if a.Kind == backup.ActRestore || a.Kind == backup.ActRemove {
			n++
		}
	}
	return n
}

// restoreKinds is the display order of the action kinds, with the preview
// row's word, the preview count's label and the result count's label.
var restoreKinds = []struct {
	kind                backup.ActionKind
	state               StateKind
	preview, toDo, done string
}{
	{backup.ActRestore, StateWarn, "would restore", "to restore", "restored"},
	{backup.ActRemove, StateWarn, "would remove", "to remove", "removed"},
	{backup.ActKept, StateWarn, "kept", "kept", "kept"},
	{backup.ActUnchanged, StateOK, "unchanged", "unchanged", "unchanged"},
	{backup.ActGone, StateOK, "already gone", "already gone", "already gone"},
}

func restoreCounts(p []backup.Action, done bool) []opCount {
	var out []opCount
	for _, k := range restoreKinds {
		n := 0
		for _, a := range p {
			if a.Kind == k.kind {
				n++
			}
		}
		if n == 0 {
			continue
		}
		label := k.toDo
		if done {
			label = k.done
		}
		out = append(out, opCount{n, label})
	}
	return out
}

// restorePreviewLines is the preview: the run, each path with what a restore
// would do to it, then the counts.
func restorePreviewLines(c chrome, l layout, run backup.Run, p []backup.Action, tilde func(string) string) []string {
	t := c.theme
	right := originOf(run) + c.sep() + run.Time.Format(backupsTimeLayout)
	out := c.headingRow(l, "Restore "+run.ID, t.Strong, right, t.Muted)
	out = append(out, "")
	if restoreWrites(p) == 0 {
		for _, s := range c.wrap("nothing to restore: every path is as the run saved it", max(l.width-2, 1)) {
			if len(out) == 2 {
				out = append(out, t.State(StateOK, s))
				continue
			}
			out = append(out, "  "+t.Text.Render(s))
		}
		return out
	}
	wordW := 0
	for _, k := range restoreKinds {
		wordW = max(wordW, lipgloss.Width(t.Glyphs.OK+" "+k.preview))
	}
	for _, k := range restoreKinds {
		for _, a := range p {
			if a.Kind != k.kind {
				continue
			}
			cell := t.State(k.state, k.preview)
			pad := strings.Repeat(" ", max(wordW-lipgloss.Width(cell), 0)+2)
			pathW := l.width - wordW - 2
			if pathW < 8 {
				out = append(out, cell, "  "+t.Text.Render(c.fitMiddle(tilde(a.Path), max(l.width-2, 1))))
				continue
			}
			out = append(out, cell+pad+t.Text.Render(c.fitMiddle(tilde(a.Path), pathW)))
		}
	}
	parts := make([]string, 0, len(restoreKinds))
	for _, n := range restoreCounts(p, false) {
		parts = append(parts, fmt.Sprintf("%d %s", n.N, n.Label))
	}
	out = append(out, "")
	if line := strings.Join(parts, c.sep()); lipgloss.Width(line) <= l.width {
		return append(out, t.Text.Render(line))
	}
	for _, s := range parts {
		out = append(out, t.Text.Render(c.fit(s, l.width)))
	}
	return out
}
