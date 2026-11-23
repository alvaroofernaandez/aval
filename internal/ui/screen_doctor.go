package ui

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/doctor"
)

// The doctor view (DESIGN.md section 10; plan section 3, Doctor): every check
// with problems first, a lower pane that explains the check under the
// cursor, and safe fixes through the operation pattern. Running the checks
// is read-only; a fix is a destructive confirm that opens on Cancel and names
// the backup, runs atomically, and the checks re-run when it closes.

func init() { registerView(viewDoctor, newDoctorView) }

// Doctor view actions, negative like the other screen-local ones.
const (
	actDoctorFix      action = -130
	actDoctorFixAll   action = -131
	actDoctorRerun    action = -132
	actDoctorSecurity action = -133
)

const (
	// doctorStateW is the state cell ("× fail"), padded so ids line up.
	doctorStateW = 7
	doctorGap    = 2
	// doctorIDMax caps the id column.
	doctorIDMax = 24
	// doctorFullFrom is the width from which a row carries its message and
	// the fix tag.
	doctorFullFrom = 60
	doctorFixTag   = "fix available"
	doctorSafety   = "Recorded in ~/.aval-backup first; undo with aval restore latest"
)

// doctorResultMsg carries one run of the checks.
type doctorResultMsg struct{ results []doctor.Result }

type doctorView struct {
	svc Services
	c   chrome

	// security adds the --security checks to every run.
	security bool
	checking bool
	run      ticket
	start    tea.Cmd

	// rows are the results, problems first; cursor indexes them.
	rows   []doctor.Result
	cursor int
}

func newDoctorView(svc Services) screen {
	d := doctorView{svc: svc}
	d.start = d.check()
	return d
}

// check starts a run under a new generation, so an earlier run's result is
// dropped when it lands.
func (d *doctorView) check() tea.Cmd {
	if d.svc.Doctor == nil {
		d.run.void()
		d.checking = false
		return nil
	}
	d.checking = true
	run, security := d.svc.Doctor, d.security
	return d.run.issue(func() tea.Msg { return doctorResultMsg{results: run(security)} })
}

func (d doctorView) init() tea.Cmd { return d.start }

func (d doctorView) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		d.c = msg.c
		return d, nil
	case resumeMsg:
		// Back from a fix, or from anything else that may have changed the
		// machine: re-run in place.
		return d, d.check()
	case tea.KeyMsg:
		return d.key(msg)
	}
	if inner, ok := d.run.accept(msg); ok {
		if r, ok := inner.(doctorResultMsg); ok {
			d.land(r.results)
		}
	}
	return d, nil
}

// land shows a run's results, keeping the cursor on the same check when it
// is still listed.
func (d *doctorView) land(results []doctor.Result) {
	prev := ""
	if d.cursor < len(d.rows) {
		prev = d.rows[d.cursor].ID
	}
	d.rows, d.checking = sortProblemsFirst(results), false
	d.cursor = 0
	for i, r := range d.rows {
		if r.ID == prev {
			d.cursor = i
			break
		}
	}
}

// sortProblemsFirst orders fail, then warn, then ok, keeping the CLI order
// within each state.
func sortProblemsFirst(rs []doctor.Result) []doctor.Result {
	rank := func(s doctor.Status) int {
		switch s {
		case doctor.Fail:
			return 0
		case doctor.Warn:
			return 1
		}
		return 2
	}
	out := append([]doctor.Result(nil), rs...)
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i].Status) < rank(out[j].Status) })
	return out
}

func (d doctorView) key(msg tea.KeyMsg) (screen, tea.Cmd) {
	act, ok := d.keys().lookup(msg.String())
	if !ok {
		return d, nil
	}
	switch act {
	case actUp:
		d.cursor = (d.cursor + len(d.rows) - 1) % len(d.rows)
	case actDown:
		d.cursor = (d.cursor + 1) % len(d.rows)
	case actDoctorRerun:
		return d, d.check()
	case actDoctorSecurity:
		d.security = !d.security
		return d, d.check()
	case actDoctorFix:
		return d, d.fix([]doctor.Result{d.rows[d.cursor]})
	case actDoctorFixAll:
		return d, d.fix(d.fixable())
	}
	return d, nil
}

// canFix reports whether the hub offers a fix for r.
func (d doctorView) canFix(r doctor.Result) bool {
	return d.svc.FixCheck != nil && r.Status != doctor.OK && r.Fix != doctor.FixNone
}

func (d doctorView) fixable() []doctor.Result {
	var out []doctor.Result
	for _, r := range d.rows {
		if d.canFix(r) {
			out = append(out, r)
		}
	}
	return out
}

func (d doctorView) keys() keyMap {
	var km keyMap
	if !d.checking && len(d.rows) > 0 {
		km = append(km,
			binding{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: "move"},
			binding{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: "move"},
		)
		if d.canFix(d.rows[d.cursor]) {
			km = append(km, binding{keys: []string{"f"}, action: actDoctorFix, label: "f", desc: "fix this", core: true})
		}
		if len(d.fixable()) > 0 {
			km = append(km, binding{keys: []string{"a"}, action: actDoctorFixAll, label: "a", desc: "fix all"})
		}
	}
	if d.svc.Doctor != nil {
		security := "security"
		if d.security {
			security = "hide security"
		}
		km = append(km,
			binding{keys: []string{"r"}, action: actDoctorRerun, label: "r", desc: "re-run", core: true},
			binding{keys: []string{"s"}, action: actDoctorSecurity, label: "s", desc: security},
		)
	}
	return km
}

func (d doctorView) busy() busyKind { return busyIdle }

func (d doctorView) view() view { return view{body: d.body, status: d.status()} }

func (d doctorView) counts() (ok, warn, fail int) {
	for _, r := range d.rows {
		switch r.Status {
		case doctor.OK:
			ok++
		case doctor.Warn:
			warn++
		case doctor.Fail:
			fail++
		}
	}
	return ok, warn, fail
}

// status is "3 of 12 need attention · 2 can be fixed here".
func (d doctorView) status() string {
	if d.checking || len(d.rows) == 0 {
		return ""
	}
	_, warn, fail := d.counts()
	if warn+fail == 0 {
		return fmt.Sprintf("All %d checks pass.", len(d.rows))
	}
	s := fmt.Sprintf("%d of %d need attention", warn+fail, len(d.rows))
	if n := len(d.fixable()); n > 0 {
		s += d.c.sep() + fmt.Sprintf("%d can be fixed here", n)
	}
	return s
}

// heading is "Doctor" with the counts, or checking, on the right.
func (d doctorView) heading(l layout) []string {
	t := d.c.theme
	right := "checking"
	switch {
	case d.svc.Doctor == nil:
		right = "not available"
	case !d.checking:
		ok, warn, fail := d.counts()
		right = strings.Join([]string{
			fmt.Sprintf("%d ok", ok), fmt.Sprintf("%d warn", warn), fmt.Sprintf("%d fail", fail),
		}, d.c.sep())
	}
	style := t.Muted
	if d.checking {
		style = t.Text
	}
	return d.c.headingRow(l, "Doctor", t.Strong, right, style)
}

// body is the heading, the list and the detail pane. When the height runs
// short the detail pane gives up lines first, then the list scrolls with
// the cursor visible.
func (d doctorView) body(l layout, h int) []string {
	head := append(d.heading(l), "")
	if d.checking || len(d.rows) == 0 {
		return head[:len(head)-1]
	}
	list := d.list(l)
	detail := d.detail(l)
	room := h - len(head)
	if h == math.MaxInt32 || len(list)+1+len(detail) <= room {
		return append(append(append(head, list...), ""), detail...)
	}
	if room < 1 {
		head, room = head[:min(len(head), max(h, 0))], 0
		if h > len(head) {
			room = h - len(head)
		}
	}
	listH := min(len(list), max(room-1-len(detail), min(len(list), max(room/2, 1))))
	listH = min(listH, room)
	start, end := scrollWindow(len(list), d.cursor, listH)
	out := append(head, list[start:end]...)
	if left := room - listH - 1; left > 0 && len(detail) > 0 {
		out = append(out, "")
		if len(detail) > left {
			more := len(detail) - left + 1
			detail = append(detail[:left-1:left-1], d.c.theme.Muted.Render(d.c.fit(fmt.Sprintf("+%d more lines", more), l.width)))
		}
		out = append(out, detail...)
	}
	return out
}

// stateCell is the glyph and word of r, padded to doctorStateW.
func (d doctorView) stateCell(r doctor.Result) string {
	k, word := StateOK, "ok"
	switch r.Status {
	case doctor.Warn:
		k, word = StateWarn, "warn"
	case doctor.Fail:
		k, word = StateFail, "fail"
	}
	s := d.c.theme.State(k, word)
	return s + strings.Repeat(" ", max(doctorStateW-lipgloss.Width(s), 0))
}

func (d doctorView) idWidth() int {
	w := 0
	for _, r := range d.rows {
		w = max(w, lipgloss.Width(r.ID))
	}
	return min(w, doctorIDMax) + doctorGap
}

// fullRows reports whether rows carry the message and the fix tag.
func (d doctorView) fullRows(l layout) bool { return l.width >= doctorFullFrom }

// list renders every row: cursor, state, id, and from doctorFullFrom up the
// message and `fix available`.
func (d doctorView) list(l layout) []string {
	t := d.c.theme
	idW := d.idWidth()
	lead := hubCursorW + doctorStateW + doctorGap
	lines := make([]string, len(d.rows))
	for i, r := range d.rows {
		marker := strings.Repeat(" ", hubCursorW)
		if i == d.cursor {
			marker = t.Marker.Render(d.c.pad(t.Glyphs.Cursor, hubCursorW))
		}
		room := max(l.width-lead, 1)
		var plain string
		switch {
		case !d.fullRows(l) || room < idW+10:
			plain = d.c.fit(r.ID, room)
		case d.canFix(r):
			tagW := lipgloss.Width(doctorFixTag) + doctorGap
			plain = d.c.pad(r.ID, idW) + d.c.pad(r.Message, room-idW-tagW) + strings.Repeat(" ", doctorGap) + doctorFixTag
		default:
			plain = d.c.pad(r.ID, idW) + d.c.fit(r.Message, room-idW)
		}
		style := t.Text
		if i == d.cursor {
			style = t.Selected
		}
		prefix := marker + d.stateCell(r)
		if l.width < lead+1 {
			prefix = marker
			plain = d.c.fit(r.ID, max(l.width-hubCursorW, 1))
		}
		lines[i] = prefix + strings.Repeat(" ", doctorGap*boolInt(l.width >= lead+1)) + style.Render(strings.TrimRight(plain, " "))
	}
	return lines
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// detail explains the check under the cursor: the title and why from
// doctor.Describe, the full Details list, and How to fix with the exact
// command when the hub has no fix for it.
func (d doctorView) detail(l layout) []string {
	t := d.c.theme
	r := d.rows[d.cursor]
	title, why, how := doctor.Describe(r.ID)
	if title == "" {
		title = r.ID
	}
	out := []string{t.Strong.Render(d.c.fit(title, l.width))}
	if !d.fullRows(l) && r.Message != "" {
		for _, s := range d.c.wrap(r.Message, l.width) {
			out = append(out, t.Text.Render(s))
		}
	}
	for _, s := range d.c.wrap(why, l.width) {
		out = append(out, t.Text.Render(s))
	}
	for _, item := range r.Details {
		out = append(out, "  "+t.Text.Render(d.c.fitMiddle(tildeUnder(d.svc.Home, item), max(l.width-2, 1))))
	}
	switch {
	case d.canFix(r):
		if !d.fullRows(l) {
			out = append(out, t.Muted.Render(d.c.fit(doctorFixTag+": f fixes it here", l.width)))
		}
	case r.Status != doctor.OK && how != "":
		out = append(out, t.Muted.Render(d.c.fit("How to fix", l.width)))
		for _, s := range d.c.wrap(how, max(l.width-2, 1)) {
			out = append(out, "  "+t.Text.Render(s))
		}
	}
	return out
}

// fix pushes the operation that repairs targets: a preview of what changes,
// a destructive confirm on Cancel naming the backup, an atomic run, and the
// result. Closing it resumes this view, which re-runs the checks.
func (d doctorView) fix(targets []doctor.Result) tea.Cmd {
	if len(targets) == 0 || d.svc.FixCheck == nil {
		return nil
	}
	fixCheck, home := d.svc.FixCheck, d.svc.Home
	action, note := fixAction(targets)
	op, err := newOperation(operationSpec[[]doctor.Result]{
		Kind: opAtomic,
		Mode: confirmDestructive,
		Confirm: confirmSpec{
			Action: action, ActionNote: note,
			Prompt: fixPrompt(targets),
			Safety: doctorSafety,
		},
		Preview: func(context.Context) ([]doctor.Result, error) { return targets, nil },
		Run: func(_ context.Context, p []doctor.Result) (opResult, error) {
			return runFixes(fixCheck, p)
		},
		PreviewBody: func(c chrome, l layout, p []doctor.Result) []string {
			return fixPreview(c, l, home, p)
		},
		PreviewStatus: "Nothing has changed yet",
		Loading:       "Reading the checks",
		Running:       "Fixing",
	})
	if err != nil {
		return nil
	}
	return push(op)
}

// fixAction names the confirm's action row from what it repairs.
func fixAction(targets []doctor.Result) (label, note string) {
	if len(targets) > 1 {
		return fmt.Sprintf("Fix %d checks", len(targets)), "back up, then repair each"
	}
	r := targets[0]
	if r.Fix == doctor.FixBrokenLinks {
		n := len(r.Details)
		switch n {
		case 0:
			return "Remove the dangling links", "back up, then unlink"
		case 1:
			return "Remove 1 link", "back up, then unlink"
		}
		return fmt.Sprintf("Remove %d links", n), "back up, then unlink"
	}
	return "Sync settings", "back up, write, verify with doctor"
}

// fixPrompt says what the confirm step repairs.
func fixPrompt(targets []doctor.Result) string {
	if len(targets) > 1 {
		ids := make([]string, len(targets))
		for i, r := range targets {
			ids[i] = r.ID
		}
		return fmt.Sprintf("Fix %d checks: %s.", len(targets), strings.Join(ids, ", "))
	}
	r := targets[0]
	if r.Fix == doctor.FixBrokenLinks {
		return fmt.Sprintf("Fix %s: remove %d dangling links from the agent directories.", r.ID, len(r.Details))
	}
	return "Fix " + r.ID + ": sync the repo-managed settings into the live config."
}

// fixPreview is what each fix changes: the links it removes, or the
// settings a sync rewrites. It writes nothing.
func fixPreview(c chrome, l layout, home string, p []doctor.Result) []string {
	t := c.theme
	title, right := "Fix all", fmt.Sprintf("%d checks", len(p))
	if len(p) == 1 {
		title, right = "Fix "+p[0].ID, ""
	}
	out := c.headingRow(l, title, t.Strong, right, t.Muted)
	for _, r := range p {
		out = append(out, "")
		lead := "Sync the repo-managed settings for " + r.ID + ":"
		if r.Fix == doctor.FixBrokenLinks {
			lead = "Remove these dangling links:"
		}
		for _, s := range c.wrap(lead, l.width) {
			out = append(out, t.Text.Render(s))
		}
		items := r.Details
		if len(items) == 0 {
			items = []string{r.Message}
		}
		for _, item := range items {
			out = append(out, "  "+t.Text.Render(c.fitMiddle(tildeUnder(home, item), max(l.width-2, 1))))
		}
	}
	return out
}

// runFixes applies each kind of fix once: the links fix for its check, and
// one sync for every drift check it repairs.
func runFixes(fixCheck func(string) ([]string, error), p []doctor.Result) (opResult, error) {
	var res opResult
	var summary []string
	total := 0
	synced := false
	for _, r := range p {
		if r.Fix == doctor.FixSync {
			if synced {
				continue
			}
			synced = true
		}
		changed, err := fixCheck(r.ID)
		total += len(changed)
		if err != nil {
			res.Summary = strings.Join(summary, " ")
			if total > 0 {
				res.Undo = "aval restore latest"
			}
			return res, fmt.Errorf("%s: %w", r.ID, err)
		}
		switch r.Fix {
		case doctor.FixBrokenLinks:
			summary = append(summary, fmt.Sprintf("Removed %d dangling links.", len(changed)))
		default:
			summary = append(summary, "Synced the repo-managed settings.")
		}
	}
	res.Summary = strings.Join(summary, " ")
	res.Counts = []opCount{{N: total, Label: "paths changed"}}
	res.Undo = "aval restore latest"
	return res, nil
}
