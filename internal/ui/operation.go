package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// operation is the operation pattern (DESIGN.md section 10) as one reusable
// screen: preview (nothing written), confirm, run, result. Views embed it, or
// push it as is. P is whatever the preview found, handed back to the run.
//
// Async work (the preview load and the run) goes through one generation
// ticket, so a cancelled or superseded result is dropped.

// opKind is how the run may end (DESIGN.md section 10, Navigation).
type opKind int

const (
	// opStepped (install, re-apply): a stop cancels the context and the run
	// ends after its current step, still reporting a result.
	opStepped opKind = iota
	// opNetwork (fetch, pull, clone): the spinner shows, and a stop cancels
	// the context and returns to the preview at once.
	opNetwork
	// opAtomic (sync, restore, uninstall, a doctor fix): keys are inert and
	// the program cannot quit until the result lands.
	opAtomic
)

// opPhase is where the operation is.
type opPhase int

const (
	phaseLoading opPhase = iota // the preview func is running
	phasePreview                // the preview; inline mode shows the rows under it
	phaseConfirm                // the destructive confirm step
	phaseRun                    // the run func is running
	phaseResult                 // the run returned
)

// opCount is one count of a result, rendered "2 linked".
type opCount struct {
	N     int
	Label string
}

// opResult is what a run reports: counts, the backup id and the undo
// command. Never "Success".
type opResult struct {
	// Summary is the headline, for example "Updated a1b2c3d → d4e5f6a."
	Summary  string
	Counts   []opCount
	BackupID string
	// Undo is the command that reverts the run, shown in the status line.
	Undo string
}

// operationSpec configures an operation. Preview, Run and Confirm.Safety are
// required.
type operationSpec[P any] struct {
	Kind opKind
	Mode confirmMode
	// Confirm is the confirm step's text. ActionFor, when set, names the
	// action from the preview ("Remove 2 links").
	Confirm   confirmSpec
	ActionFor func(p P) string
	// NoteFor, when set, names the action's note from the preview.
	NoteFor func(p P) string
	// Ready reports whether the preview leaves anything to do; when false
	// there is no confirm and only esc back. Nil means always ready.
	Ready func(p P) bool

	// Preview reads what the run would do and writes nothing. Run writes.
	// Both get a context the operation cancels on stop.
	Preview func(ctx context.Context) (P, error)
	Run     func(ctx context.Context, p P) (opResult, error)

	// PreviewBody renders the preview.
	PreviewBody func(c chrome, l layout, p P) []string
	// PreviewRows renders the preview as a head that always shows and rows
	// that scroll when they do not fit (up, down, "N more below"). It is
	// used instead of PreviewBody.
	PreviewRows func(c chrome, l layout, p P) (head, rows []string)
	// PreviewStatus is the destructive preview's status line, for example
	// "Nothing has changed yet".
	PreviewStatus string
	// Loading and Running label the waits, without the ellipsis.
	Loading, Running string
	// Result overrides the default result rendering.
	Result func(c chrome, l layout, r opResult, err error) []string
}

type operation[P any] struct {
	spec operationSpec[P]
	c    chrome

	phase   opPhase
	preview P
	loadErr error
	confirm confirm
	// top is the first scrolled row of a PreviewRows preview.
	top    int
	result opResult
	runErr error
	// stopping: a stepped run was asked to stop and is finishing its step.
	stopping bool

	// job is the generation of the preview load or the run in flight; spin
	// the spinner's tick chain.
	job, spin ticket
	frame     int
	cancel    context.CancelFunc
	// start is the first load, issued at construction so init can return it.
	start tea.Cmd
}

type (
	opPreviewMsg[P any] struct {
		p   P
		err error
	}
	opDoneMsg struct {
		r   opResult
		err error
	}
	opTickMsg struct{}
)

var (
	errOpNoPreview = errors.New("operation: a preview func is required")
	errOpNoRun     = errors.New("operation: a run func is required")
)

// newOperation validates spec and starts in the loading phase; init returns
// the load. A confirm without a safety line is an error here, before any
// view can show it.
func newOperation[P any](spec operationSpec[P]) (operation[P], error) {
	if spec.Preview == nil {
		return operation[P]{}, errOpNoPreview
	}
	if spec.Run == nil {
		return operation[P]{}, errOpNoRun
	}
	cs := spec.Confirm
	if spec.ActionFor != nil && strings.TrimSpace(cs.Action) == "" {
		cs.Action = "-" // validated per preview; only the safety line matters here
	}
	if _, err := newConfirm(spec.Mode, cs); err != nil {
		return operation[P]{}, err
	}
	o := operation[P]{spec: spec, phase: phaseLoading}
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = cancel
	load := spec.Preview
	o.start = tea.Batch(
		o.job.issue(func() tea.Msg {
			p, err := load(ctx)
			return opPreviewMsg[P]{p, err}
		}),
	)
	return o, nil
}

func (o operation[P]) init() tea.Cmd { return o.start }

// spinning reports whether the current wait shows the spinner: network
// waits only, and only with the chrome's motion on.
func (o operation[P]) spinning() bool {
	return o.c.Motion && o.spec.Kind == opNetwork && (o.phase == phaseLoading || o.phase == phaseRun)
}

// tick issues the next spinner frame when the wait spins.
func (o *operation[P]) tick() tea.Cmd {
	if !o.spinning() {
		o.spin.void()
		return nil
	}
	return o.spin.issue(tea.Tick(spinInterval, func(time.Time) tea.Msg { return opTickMsg{} }))
}

func (o operation[P]) ready() bool {
	return o.loadErr == nil && (o.spec.Ready == nil || o.spec.Ready(o.preview))
}

// newConfirmFor builds the confirm for the loaded preview, cursor reset.
func (o operation[P]) newConfirmFor() confirm {
	cs := o.spec.Confirm
	if o.spec.ActionFor != nil {
		cs.Action = o.spec.ActionFor(o.preview)
	}
	if o.spec.NoteFor != nil {
		cs.ActionNote = o.spec.NoteFor(o.preview)
	}
	c, err := newConfirm(o.spec.Mode, cs)
	if err != nil {
		// newOperation checked the safety line; only an empty ActionFor
		// label gets here, and that is a programming error in the view.
		panic(err)
	}
	return c
}

func (o operation[P]) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		motionOn := msg.c.Motion && !o.c.Motion
		o.c = msg.c
		if motionOn {
			// The first tick waits for the chrome: only it knows motion is on.
			return o, o.tick()
		}
		return o, nil
	case tea.KeyMsg:
		return o.key(msg)
	case stopMsg:
		return o.stop()
	}
	if inner, ok := o.spin.accept(msg); ok {
		if _, tick := inner.(opTickMsg); tick {
			o.frame = (o.frame + 1) % len(spinFrames)
			return o, o.tick()
		}
	}
	inner, ok := o.job.accept(msg)
	if !ok {
		return o, nil
	}
	o.job.void()
	switch inner := inner.(type) {
	case opPreviewMsg[P]:
		o.phase, o.preview, o.loadErr = phasePreview, inner.p, inner.err
		if o.ready() {
			o.confirm = o.newConfirmFor()
		}
	case opDoneMsg:
		o.phase, o.result, o.runErr, o.stopping = phaseResult, inner.r, inner.err, false
		o.release()
	}
	o.spin.void()
	return o, nil
}

// release cancels the context of the finished or abandoned job.
func (o *operation[P]) release() {
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
}

func (o operation[P]) key(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch o.phase {
	case phasePreview:
		if !o.ready() {
			return o, nil
		}
		if o.spec.Mode == confirmInline {
			return o.choose(msg)
		}
		act, ok := o.keys().lookup(msg.String())
		switch {
		case !ok:
		case act == actOpContinue:
			o.phase, o.confirm = phaseConfirm, o.newConfirmFor()
		case act == actUp:
			o.top = max(o.top-1, 0)
		case act == actDown:
			n, room := o.scrollRows()
			o.top = min(o.top+1, max(n-room, 0))
		}
	case phaseConfirm:
		return o.choose(msg)
	case phaseResult:
		if act, ok := o.keys().lookup(msg.String()); ok && act == actOpClose {
			return o, pop()
		}
	}
	// Loading and run take no keys: the router turns esc, q and ctrl+c into
	// a stopMsg when the wait is cancellable, and drops them when atomic.
	return o, nil
}

// choose hands a key to the confirm rows and acts on the choice.
func (o operation[P]) choose(msg tea.KeyMsg) (screen, tea.Cmd) {
	var ch confirmChoice
	o.confirm, ch = o.confirm.update(msg)
	switch ch {
	case choiceAction:
		return o.run()
	case choiceCancel:
		if o.spec.Mode == confirmInline {
			return o, pop()
		}
		o.phase = phasePreview
	}
	return o, nil
}

// run starts the run func under a fresh generation and context.
func (o operation[P]) run() (screen, tea.Cmd) {
	o.phase, o.stopping, o.frame = phaseRun, false, 0
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = cancel
	run, p := o.spec.Run, o.preview
	cmd := o.job.issue(func() tea.Msg {
		r, err := run(ctx, p)
		return opDoneMsg{r, err}
	})
	return o, tea.Batch(cmd, o.tick())
}

// stop is the router's stopMsg: esc, q or ctrl+c during a cancellable wait.
func (o operation[P]) stop() (screen, tea.Cmd) {
	switch {
	case o.phase == phaseLoading:
		// The preview writes nothing, so cancelling it simply leaves.
		o.release()
		o.job.void()
		o.spin.void()
		return o, pop()
	case o.phase != phaseRun || o.spec.Kind == opAtomic:
		return o, nil
	case o.spec.Kind == opStepped:
		// The step in flight finishes and the run still reports.
		o.stopping = true
		if o.cancel != nil {
			o.cancel()
		}
		return o, nil
	}
	o.release()
	o.job.void()
	o.spin.void()
	o.phase = phasePreview
	if o.ready() {
		o.confirm = o.newConfirmFor()
	}
	return o, nil
}

var (
	opContinueKeys = keyMap{{keys: []string{"enter"}, action: actOpContinue, label: "enter", desc: "continue", core: true}}
	opResultKeys   = keyMap{{keys: []string{"enter"}, action: actOpClose, label: "enter", desc: "close", core: true}}
)

var opScrollKeys = keyMap{
	{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: "scroll"},
	{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: "scroll"},
}

// scrollRows is how many rows a PreviewRows preview has and how many it
// shows at the frame's height; room is 0 when the preview is not a scrolling
// list or the size is not known yet.
func (o operation[P]) scrollRows() (n, room int) {
	if o.spec.PreviewRows == nil || o.c.height <= 0 {
		return 0, 0
	}
	l := o.c.layout()
	head, rows := o.spec.PreviewRows(o.c, l, o.preview)
	v := view{body: o.previewLines, status: o.spec.PreviewStatus}
	budget := o.c.framePlan(v, opContinueKeys, l).budget
	if len(head)+len(rows) <= budget {
		return len(rows), len(rows)
	}
	return len(rows), max(budget-len(head)-1, 0)
}

// scrolls reports whether the preview has rows below or above the window.
func (o operation[P]) scrolls() bool {
	n, room := o.scrollRows()
	return n > room
}

func (o operation[P]) keys() keyMap {
	switch o.phase {
	case phasePreview:
		switch {
		case !o.ready():
			return nil
		case o.spec.Mode == confirmInline:
			return o.confirm.keys()
		}
		if o.scrolls() {
			return append(append(keyMap(nil), opContinueKeys...), opScrollKeys...)
		}
		return opContinueKeys
	case phaseConfirm:
		return o.confirm.keys()
	case phaseResult:
		return opResultKeys
	}
	return nil
}

func (o operation[P]) busy() busyKind {
	switch o.phase {
	case phaseLoading:
		return busyCancellable
	case phaseRun:
		if o.spec.Kind == opAtomic {
			return busyAtomic
		}
		return busyCancellable
	}
	return busyIdle
}

func (o operation[P]) view() view {
	c := o.c
	switch o.phase {
	case phaseLoading:
		return view{body: func(l layout, h int) []string { return o.waitLines(l, o.spec.Loading, "Loading") }}
	case phaseRun:
		v := view{body: func(l layout, h int) []string { return o.waitLines(l, o.spec.Running, "Running") }}
		if o.stopping {
			v.status = "Stopping after the current step" + c.theme.Glyphs.Ellipsis
		}
		return v
	case phaseConfirm:
		return view{body: func(l layout, h int) []string { return o.confirm.lines(c, l) }, status: o.confirm.status()}
	case phaseResult:
		v := view{body: func(l layout, h int) []string { return o.resultLines(l) }}
		if o.result.Undo != "" {
			v.status = "undo: " + o.result.Undo
		}
		return v
	}
	v := view{body: o.previewLines, status: o.spec.PreviewStatus}
	if o.spec.Mode == confirmInline && o.ready() {
		v.status = o.confirm.status()
	}
	return v
}

// previewLines is the preview, with the inline rows under it. When h runs
// short the preview gives up lines so the rows stay visible.
func (o operation[P]) previewLines(l layout, h int) []string {
	c := o.c
	var body []string
	switch {
	case o.loadErr != nil:
		body = o.failLines(l, o.loadErr)
	case o.spec.PreviewRows != nil:
		head, rows := o.spec.PreviewRows(c, l, o.preview)
		body = o.scrollList(l, h, head, rows)
	case o.spec.PreviewBody != nil:
		body = o.spec.PreviewBody(c, l, o.preview)
	}
	if o.spec.Mode != confirmInline || !o.ready() {
		return body
	}
	rows := o.confirm.lines(c, l)
	if keep := h - len(rows) - 1; len(body) > keep {
		body = body[:max(keep, 0)]
	}
	if len(body) > 0 {
		body = append(body, "")
	}
	return append(body, rows...)
}

// scrollList is head plus the rows that fit under it from o.top, with a line
// saying how many are out of sight when some are.
func (o operation[P]) scrollList(l layout, h int, head, rows []string) []string {
	if len(head)+len(rows) <= h {
		return append(head, rows...)
	}
	room := max(h-len(head)-1, 0)
	start, end := scrollWindow(len(rows), o.top+room-1, room)
	var parts []string
	if start > 0 {
		parts = append(parts, fmt.Sprintf("%d above", start))
	}
	if end < len(rows) {
		parts = append(parts, fmt.Sprintf("%d more below", len(rows)-end))
	}
	out := append(head, rows[start:end]...)
	return append(out, o.c.theme.Muted.Render(o.c.fit(strings.Join(parts, o.c.sep()), l.width)))
}

// waitLines is the one spinner beside label, or static text with motion off.
func (o operation[P]) waitLines(l layout, label, fallback string) []string {
	if label == "" {
		label = fallback
	}
	text := label + o.c.theme.Glyphs.Ellipsis
	if o.spinning() {
		text = spinFrames[o.frame%len(spinFrames)] + " " + text
	}
	return []string{o.c.theme.Text.Render(o.c.fit(text, l.width))}
}

func (o operation[P]) failLines(l layout, err error) []string {
	var out []string
	for i, line := range o.c.wrap(err.Error(), max(l.width-2, 1)) {
		if i == 0 {
			out = append(out, o.c.theme.State(StateFail, line))
			continue
		}
		out = append(out, "  "+o.c.theme.Text.Render(line))
	}
	return out
}

// resultLines is the result: a failure first, the summary, the counts and
// the backup id. The undo command goes in the status line.
func (o operation[P]) resultLines(l layout) []string {
	c := o.c
	if o.spec.Result != nil {
		return o.spec.Result(c, l, o.result, o.runErr)
	}
	var out []string
	if o.runErr != nil {
		out = append(out, o.failLines(l, o.runErr)...)
	}
	for _, s := range c.wrap(o.result.Summary, l.width) {
		out = append(out, c.theme.Text.Render(s))
	}
	if len(o.result.Counts) > 0 {
		parts := make([]string, len(o.result.Counts))
		for i, n := range o.result.Counts {
			parts[i] = fmt.Sprintf("%d %s", n.N, n.Label)
		}
		for _, s := range c.wrap(strings.Join(parts, c.sep()), l.width) {
			out = append(out, c.theme.Text.Render(s))
		}
	}
	if o.result.BackupID != "" {
		out = append(out, c.theme.Muted.Render(c.fit("backup "+o.result.BackupID, l.width)))
	}
	return out
}
