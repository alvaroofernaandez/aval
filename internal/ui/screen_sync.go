package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/txsync"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// The sync view is the operation pattern on txsync, inline: the dry run lists
// what would change and the confirm rows sit under it with the cursor on the
// action. The run is atomic (back up, write, verify with doctor, roll back on
// failure) and local, so nothing spins.

const (
	// syncStateW is the widest state cell ("! would change"); syncKindW the
	// widest kind ("settings").
	syncStateW = 14
	syncKindW  = 8
)

// syncRow is one line of the preview: a state, the kind of thing, and what.
type syncRow struct {
	state  StateKind
	word   string
	kind   string
	target string
}

// syncRows reads a report into rows. Agent-rules targets whose directory is
// absent are skipped by sync and left out here.
func syncRows(tr i18n.Translator, o *txsync.Outcome) []syncRow {
	if o == nil || o.Report == nil {
		return nil
	}
	var rows []syncRow
	s := o.Report.Settings
	switch {
	case s.Created:
		rows = append(rows, syncRow{StateWarn, tr.T("sync.would_create", nil), tr.T("sync.kind.settings", nil), "~/.claude/settings.json"})
	case len(s.Changed) > 0:
		rows = append(rows, syncRow{StateWarn, tr.T("sync.would_change", nil), tr.T("sync.kind.settings", nil), strings.Join(s.Changed, ", ")})
	default:
		rows = append(rows, syncRow{StateOK, tr.T("sync.up_to_date", nil), tr.T("sync.kind.settings", nil), "~/.claude/settings.json"})
	}
	for _, r := range o.Report.Rules {
		switch {
		case r.Skipped:
		case r.Changed:
			rows = append(rows, syncRow{StateWarn, tr.T("sync.would_update", nil), tr.T("sync.kind.rules", nil), r.Path})
		default:
			rows = append(rows, syncRow{StateOK, tr.T("sync.up_to_date", nil), tr.T("sync.kind.rules", nil), r.Path})
		}
	}
	for _, l := range o.Report.Links {
		if l.Status == "would link" {
			rows = append(rows, syncRow{StateWarn, tr.T("sync.would_link", nil), tr.T("sync.kind.overlay", nil), l.Dest})
			continue
		}
		rows = append(rows, syncRow{StateOK, tr.T("sync.linked", nil), tr.T("sync.kind.overlay", nil), l.Dest})
	}
	return rows
}

func syncDrift(rows []syncRow) int {
	n := 0
	for _, r := range rows {
		if r.state == StateWarn {
			n++
		}
	}
	return n
}

func newSyncView(svc Services) screen {
	tr := svc.tr
	o, err := newOperation(operationSpec[*txsync.Outcome]{
		Kind: opAtomic,
		Mode: confirmInline,
		Confirm: confirmSpec{
			Action:     tr.T("sync.action", nil),
			ActionNote: tr.T("sync.action_note", nil),
			Safety:     tr.T("sync.safety", nil),
		},
		Ready: func(o *txsync.Outcome) bool { return syncDrift(syncRows(tr, o)) > 0 },
		Preview: func(context.Context) (*txsync.Outcome, error) {
			if svc.SyncPreview == nil {
				return nil, errors.New(tr.T("sync.unavailable", nil))
			}
			return svc.SyncPreview()
		},
		Run: func(_ context.Context, _ *txsync.Outcome) (opResult, error) {
			return runSync(svc, tr)
		},
		PreviewBody: func(c chrome, l layout, o *txsync.Outcome) []string {
			return syncBody(c, l, svc.Home, o)
		},
		Result:  syncResult,
		Loading: tr.T("sync.loading", nil),
		Running: tr.T("sync.running", nil),
	})
	if err != nil {
		panic(err)
	}
	return glyphSepScreen{screen: o, sep: " · "}
}

// syncRolledBack is the error of a sync that was undone; it carries the
// outcome so the result can list the checks that failed.
type syncRolledBack struct {
	out *txsync.Outcome
	err error
}

func (e syncRolledBack) Error() string { return e.err.Error() }
func (e syncRolledBack) Unwrap() error { return e.err }

func runSync(svc Services, tr i18n.Translator) (opResult, error) {
	if svc.Sync == nil {
		return opResult{}, errors.New(tr.T("sync.unavailable", nil))
	}
	out, err := svc.Sync()
	if out != nil && out.RolledBack {
		if err == nil {
			err = errors.New(tr.T("sync.rolled_back_error", nil))
		}
		return opResult{BackupID: out.RunID}, syncRolledBack{out: out, err: err}
	}
	if err != nil {
		return opResult{}, err
	}
	rows := syncRows(tr, out)
	changed := syncDrift(rows)
	r := opResult{
		Summary:  tr.T("sync.synced", nil),
		Counts:   []opCount{{changed, tr.T("sync.count.changed", nil)}, {len(rows) - changed, tr.T("sync.count.already", nil)}},
		BackupID: "",
		Undo:     undoLatest,
	}
	if out != nil {
		r.BackupID = out.RunID
	}
	return r, nil
}

// syncBody is the heading, the rows and, with nothing to do, why.
func syncBody(c chrome, l layout, home string, o *txsync.Outcome) []string {
	out := c.headingRow(l, c.tr.T("sync.title", nil), c.theme.Strong, "~/.claude/settings.json", c.theme.Muted)
	out = append(out, "")
	rows := syncRows(c.tr, o)
	if syncDrift(rows) == 0 {
		return append(out, c.theme.State(StateOK, c.fit(c.tr.T("sync.nothing", nil), max(l.width-lipgloss.Width(c.theme.Glyphs.OK)-1, 1))))
	}
	lead := syncStateW + 2 + syncKindW + 2
	room := l.width - lead
	for _, r := range rows {
		state := c.theme.State(r.state, r.word)
		cell := state + strings.Repeat(" ", max(syncStateW-lipgloss.Width(state), 0)) + "  "
		target := tildeUnder(home, r.target)
		if room < 12 {
			// Too narrow for columns: the state and kind, then the target
			// under them.
			head := state
			if lipgloss.Width(state)+2+len(r.kind) <= l.width {
				head += "  " + c.theme.Muted.Render(r.kind)
			}
			out = append(out, head)
			out = append(out, "  "+c.theme.Text.Render(c.fitMiddle(target, max(l.width-2, 1))))
			continue
		}
		kind := c.theme.Muted.Render(c.pad(r.kind, syncKindW))
		out = append(out, cell+kind+"  "+c.theme.Text.Render(c.fitMiddle(target, room)))
	}
	return out
}

// syncResult renders the run: a rollback names the checks that failed and
// the backup id; any other error is shown as is; success is the summary.
func syncResult(c chrome, l layout, r opResult, err error) []string {
	var out []string
	var rb syncRolledBack
	switch {
	case errors.As(err, &rb):
		out = append(out, c.theme.State(StateFail, c.tr.T("sync.rolled_back", nil)))
		for _, s := range c.wrap(c.tr.T("sync.restored", nil), max(l.width-2, 1)) {
			out = append(out, "  "+c.theme.Text.Render(s))
		}
		if len(rb.out.Failed) == 0 {
			for _, s := range c.wrap(rb.err.Error(), max(l.width-2, 1)) {
				out = append(out, "  "+c.theme.Text.Render(s))
			}
		}
		for _, f := range rb.out.Failed {
			for i, s := range c.wrap(f.ID+"  "+f.Message, max(l.width-2, 1)) {
				if i == 0 {
					out = append(out, c.theme.State(StateFail, s))
					continue
				}
				out = append(out, "  "+c.theme.Text.Render(s))
			}
		}
	case err != nil:
		for i, s := range c.wrap(err.Error(), max(l.width-2, 1)) {
			if i == 0 {
				out = append(out, c.theme.State(StateFail, s))
				continue
			}
			out = append(out, "  "+c.theme.Text.Render(s))
		}
		return out
	default:
		out = append(out, c.theme.Text.Render(r.Summary))
		parts := make([]string, len(r.Counts))
		for i, n := range r.Counts {
			parts[i] = fmt.Sprintf("%d %s", n.N, n.Label)
		}
		for _, s := range c.wrap(strings.Join(parts, c.sep()), l.width) {
			out = append(out, c.theme.Text.Render(s))
		}
	}
	if r.BackupID != "" {
		out = append(out, c.theme.Muted.Render(c.fit(c.tr.T("op.backup", i18n.Args{"id": r.BackupID}), l.width)))
	}
	return out
}

func init() { registerView(viewSync, newSyncView) }
