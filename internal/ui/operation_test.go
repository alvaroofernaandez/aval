package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fakePreview is what the test operations preview: two links to remove.
type fakePreview struct{ links []string }

// opProbe records what the run func saw.
type opProbe struct {
	runs int
	ctx  context.Context
}

func testOperation(t *testing.T, kind opKind, mode confirmMode, p *opProbe) operation[fakePreview] {
	t.Helper()
	o, err := newOperation(operationSpec[fakePreview]{
		Kind:    kind,
		Mode:    mode,
		Confirm: confirmSpec{Action: "Remove 2 links", ActionNote: "back up, then unlink", Safety: testSafety},
		Preview: func(context.Context) (fakePreview, error) {
			return fakePreview{links: []string{"~/.claude/skills/old-skill", "~/.claude/skills/retired"}}, nil
		},
		Run: func(ctx context.Context, _ fakePreview) (opResult, error) {
			p.runs++
			p.ctx = ctx
			return opResult{
				Summary:  "Removed 2 links.",
				Counts:   []opCount{{2, "removed"}, {55, "already in place"}},
				BackupID: "20261123-101402",
				Undo:     "aval restore latest",
			}, nil
		},
		PreviewBody: func(c chrome, l layout, p fakePreview) []string {
			out := []string{c.theme.Strong.Render("Uninstall")}
			for _, s := range p.links {
				out = append(out, "  "+c.theme.State(StateWarn, "would remove")+"  "+c.fitMiddle(s, l.width-20))
			}
			return out
		},
		PreviewStatus: "Nothing has changed yet",
		Loading:       "Reading the links",
		Running:       "Removing links",
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// opStep sends msg to s and feeds back what its command produces, except
// commands listed in hold, which are returned unrun. It never runs ticks.
func opStep(s screen, msg tea.Msg) (screen, []tea.Msg) {
	next, cmd := s.update(msg)
	var out []tea.Msg
	for _, m := range drainOp(cmd) {
		switch m.(type) {
		case popMsg, pushMsg, quitMsg:
			out = append(out, m)
			continue
		}
		var more []tea.Msg
		next, more = opStep(next, m)
		out = append(out, more...)
	}
	return next, out
}

// drainOp is drainCmd without spinner ticks, which would sleep and loop.
func drainOp(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	for _, m := range drainCmd(cmd) {
		if g, ok := m.(genMsg); ok {
			if _, tick := g.msg.(opTickMsg); tick {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

// loaded runs the operation's init and delivers the preview.
func loaded(t *testing.T, o screen) screen { return loadedWith(t, o, false) }

// loadedWith is loaded with the chrome's motion setting.
func loadedWith(t *testing.T, o screen, motion bool) screen {
	t.Helper()
	ch := testChrome(t, true)
	ch.Motion = motion
	o, _ = opStep(o, chromeMsg{ch})
	for _, m := range drainOp(o.init()) {
		o, _ = opStep(o, m)
	}
	return o
}

func phaseOf(t *testing.T, s screen) opPhase {
	t.Helper()
	o, ok := s.(operation[fakePreview])
	if !ok {
		t.Fatalf("update returned %T, want the operation", s)
	}
	return o.phase
}

func TestOperationRequiresASafetyLine(t *testing.T) {
	_, err := newOperation(operationSpec[fakePreview]{
		Confirm: confirmSpec{Action: "Remove"},
		Preview: func(context.Context) (fakePreview, error) { return fakePreview{}, nil },
		Run:     func(context.Context, fakePreview) (opResult, error) { return opResult{}, nil },
	})
	if err == nil {
		t.Fatal("newOperation without a safety line = nil error, want one")
	}
}

func TestDestructiveOperationConfirmsOnItsOwnStepOnCancel(t *testing.T) {
	p := &opProbe{}
	s := loaded(t, testOperation(t, opAtomic, confirmDestructive, p))
	if got := phaseOf(t, s); got != phasePreview {
		t.Fatalf("after load phase = %v, want preview", got)
	}
	if strings.Contains(viewText(s), "Cancel") {
		t.Errorf("the destructive preview shows the confirm rows:\n%s", viewText(s))
	}
	s, _ = opStep(s, key("enter"))
	if got := phaseOf(t, s); got != phaseConfirm {
		t.Fatalf("enter on the preview: phase = %v, want confirm", got)
	}
	if !strings.Contains(viewText(s), "› Cancel") {
		t.Errorf("the confirm step does not open on Cancel:\n%s", viewText(s))
	}
	if got := s.view().status; got != testSafety {
		t.Errorf("confirm status = %q, want the safety line", got)
	}
}

func TestEnterOnCancelRunsNothing(t *testing.T) {
	p := &opProbe{}
	s := loaded(t, testOperation(t, opAtomic, confirmDestructive, p))
	s, _ = opStep(s, key("enter"))
	s, _ = opStep(s, key("enter"))
	if p.runs != 0 {
		t.Fatalf("enter on Cancel called run %d times", p.runs)
	}
	if got := phaseOf(t, s); got != phasePreview {
		t.Errorf("enter on Cancel: phase = %v, want the preview", got)
	}
}

func TestEnterOnTheActionRunsOnce(t *testing.T) {
	p := &opProbe{}
	s := loaded(t, testOperation(t, opAtomic, confirmDestructive, p))
	s, _ = opStep(s, key("enter"))
	s, _ = opStep(s, key("up"))
	s, _ = opStep(s, key("enter"))
	if p.runs != 1 {
		t.Fatalf("enter on the action called run %d times, want 1", p.runs)
	}
	if got := phaseOf(t, s); got != phaseResult {
		t.Errorf("phase = %v, want result", got)
	}
}

func TestInlineOperationRunsFromThePreview(t *testing.T) {
	p := &opProbe{}
	s := loaded(t, testOperation(t, opAtomic, confirmInline, p))
	if !strings.Contains(viewText(s), "› Remove 2 links") {
		t.Errorf("inline rows want the cursor on the action under the preview:\n%s", viewText(s))
	}
	s, _ = opStep(s, key("enter"))
	if p.runs != 1 {
		t.Fatalf("inline enter called run %d times, want 1", p.runs)
	}
}

func TestInlineBackPopsTheView(t *testing.T) {
	p := &opProbe{}
	s := loaded(t, testOperation(t, opAtomic, confirmInline, p))
	s, _ = opStep(s, key("down"))
	_, out := opStep(s, key("enter"))
	if p.runs != 0 || len(out) != 1 || out[0] != (popMsg{}) {
		t.Errorf("Back: runs %d, messages %v; want no run and a pop", p.runs, out)
	}
}

func TestResultCarriesCountsBackupAndUndo(t *testing.T) {
	p := &opProbe{}
	s := loaded(t, testOperation(t, opAtomic, confirmInline, p))
	s, _ = opStep(s, key("enter"))
	got := viewText(s)
	for _, want := range []string{"Removed 2 links.", "2 removed", "55 already in place", "20261123-101402", "aval restore latest"} {
		if !strings.Contains(got, want) {
			t.Errorf("result lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(strings.ToLower(got), "success") {
		t.Errorf("result says success:\n%s", got)
	}
}

func TestRunErrorShowsAFailure(t *testing.T) {
	o := testOperation(t, opAtomic, confirmInline, &opProbe{})
	o.spec.Run = func(context.Context, fakePreview) (opResult, error) {
		return opResult{BackupID: "20261123-101402", Undo: "aval restore latest"}, errors.New("rolled back: doctor failed")
	}
	s := loaded(t, o)
	s, _ = opStep(s, key("enter"))
	got := viewText(s)
	if !strings.Contains(got, "× rolled back: doctor failed") || !strings.Contains(got, "20261123-101402") {
		t.Errorf("failed run view:\n%s", got)
	}
}

// startRun moves a loaded inline operation into its run and returns the run
// command unrun, with the probe holding the run's context once it runs.
func startRun(t *testing.T, kind opKind, motion bool, p *opProbe) (screen, []tea.Msg) {
	t.Helper()
	s := loadedWith(t, testOperation(t, kind, confirmInline, p), motion)
	next, cmd := s.update(key("enter"))
	if got := phaseOf(t, next); got != phaseRun {
		t.Fatalf("enter: phase = %v, want run", got)
	}
	return next, drainOp(cmd)
}

func TestCancellableRunReturnsToThePreviewOnStop(t *testing.T) {
	p := &opProbe{}
	s, held := startRun(t, opNetwork, false, p)
	if s.busy() != busyCancellable {
		t.Fatalf("network run busy = %v, want cancellable", s.busy())
	}
	if p.ctx == nil {
		t.Fatal("run was not called")
	}
	s, _ = opStep(s, stopMsg{})
	if got := phaseOf(t, s); got != phasePreview {
		t.Errorf("after stop phase = %v, want preview", got)
	}
	if !errors.Is(p.ctx.Err(), context.Canceled) {
		t.Errorf("run context err = %v, want canceled", p.ctx.Err())
	}
	// The cancelled run's result arrives late and is dropped.
	for _, m := range held {
		s, _ = opStep(s, m)
	}
	if got := phaseOf(t, s); got != phasePreview {
		t.Errorf("a stale result moved the phase to %v", got)
	}
}

func TestStaleOperationResultsAreDropped(t *testing.T) {
	p := &opProbe{}
	s, held := startRun(t, opNetwork, false, p)
	s, _ = opStep(s, stopMsg{})
	// Start a second run; the first one's result must not finish it.
	next, cmd := s.update(key("enter"))
	s = next
	for _, m := range held {
		s, _ = opStep(s, m)
	}
	if got := phaseOf(t, s); got != phaseRun {
		t.Fatalf("a stale result ended the second run: phase %v", got)
	}
	for _, m := range drainOp(cmd) {
		s, _ = opStep(s, m)
	}
	if got := phaseOf(t, s); got != phaseResult {
		t.Errorf("the current result was not accepted: phase %v", got)
	}
}

func TestSteppedRunFinishesTheStepOnStop(t *testing.T) {
	p := &opProbe{}
	s, held := startRun(t, opStepped, false, p)
	s, _ = opStep(s, stopMsg{})
	if got := phaseOf(t, s); got != phaseRun {
		t.Fatalf("a stepped stop left the run early: phase %v", got)
	}
	if !strings.Contains(viewText(s), "Stopping after the current step") {
		t.Errorf("stepped stop does not say so:\n%s", viewText(s))
	}
	for _, m := range held {
		s, _ = opStep(s, m)
	}
	if got := phaseOf(t, s); got != phaseResult {
		t.Errorf("a stepped run's result after stop: phase %v, want result", got)
	}
}

func TestAtomicRunReportsBusyAtomicAndIgnoresKeys(t *testing.T) {
	p := &opProbe{}
	s, _ := startRun(t, opAtomic, false, p)
	if s.busy() != busyAtomic {
		t.Fatalf("atomic run busy = %v, want atomic", s.busy())
	}
	a := testApp(t, s)
	for _, k := range append(candidateKeys(), "ctrl+c") {
		msg := key(k)
		switch k {
		case "esc":
			msg = escKey()
		case "ctrl+c":
			msg = ctrlC()
		}
		var quit bool
		a, quit = routerStep(a, msg)
		if quit || a.Quitting() {
			t.Fatalf("key %q quit during an atomic run", k)
		}
		if a.help {
			a, _ = routerStep(a, key("?"))
		}
		if got := phaseOf(t, a.top()); got != phaseRun {
			t.Fatalf("key %q moved an atomic run to %v", k, got)
		}
	}
	if next, _ := opStep(s, stopMsg{}); phaseOf(t, next) != phaseRun {
		t.Error("stopMsg ended an atomic run")
	}
}

func TestNetworkWaitShowsTheSpinner(t *testing.T) {
	s, _ := startRun(t, opNetwork, true, &opProbe{})
	if got := viewText(s); !strings.Contains(got, spinFrames[0]+" Removing links") {
		t.Errorf("motion on: no spinner beside the running label:\n%s", got)
	}
}

func TestNetworkWaitWithMotionOffShowsStaticText(t *testing.T) {
	s, _ := startRun(t, opNetwork, false, &opProbe{})
	got := viewText(s)
	if !strings.Contains(got, "Removing links") {
		t.Fatalf("no running label:\n%s", got)
	}
	for _, f := range spinFrames {
		if strings.Contains(got, f+" Removing links") {
			t.Errorf("motion off shows spinner frame %q:\n%s", f, got)
		}
	}
	if next, cmd := s.update(opTickMsg{}); cmd != nil || viewText(next) != got {
		t.Error("motion off: a tick still animates")
	}
}

// viewText renders a screen's body and status at 80 columns.
func viewText(s screen) string {
	v := s.view()
	l := layout{bp: bpRegular, margin: 2, width: 76}
	return strings.Join(append(v.body(l, 1000), v.status), "\n")
}

// goldenConfirmApp is the confirm step as the program runs it: an App with a
// fake hub at the bottom and an operation on top, loaded and, when
// destructive, on its own confirm step (cursor on Cancel).
func goldenConfirmApp(t *testing.T, unicode bool, mode confirmMode) tea.Model {
	t.Helper()
	spec := confirmSpec{
		Action: "Remove 2 links", ActionNote: "back up, then unlink",
		Prompt: "This removes 2 links that point into ~/.aval.",
		Safety: "Files in the way move to ~/.aval-backup first · undo: aval restore latest",
	}
	if !unicode {
		spec.Safety = "Files in the way move to ~/.aval-backup first, undo: aval restore latest"
	}
	if mode == confirmInline {
		spec.Action, spec.ActionNote = "Apply sync", "back up, write, verify with doctor"
	}
	o, err := newOperation(operationSpec[fakePreview]{
		Kind:    opAtomic,
		Mode:    mode,
		Confirm: spec,
		Preview: func(context.Context) (fakePreview, error) {
			return fakePreview{links: []string{"~/.claude/skills/old-skill", "~/.claude/skills/retired"}}, nil
		},
		Run: func(context.Context, fakePreview) (opResult, error) { return opResult{}, nil },
		PreviewBody: func(c chrome, l layout, p fakePreview) []string {
			out := c.headingRow(l, "Uninstall", c.theme.Strong, "2 links", c.theme.Muted)
			out = append(out, "")
			for _, s := range p.links {
				cell := c.theme.State(StateWarn, "would remove") + "  "
				out = append(out, cell+c.theme.Text.Render(c.fitMiddle(s, max(l.width-lipgloss.Width(cell), 1))))
			}
			return out
		},
		PreviewStatus: "Nothing has changed yet",
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := chrome{theme: goldenTheme(t, unicode), version: "0.2.0", platform: "macOS (arm64)", width: 80, height: 40}
	var s screen = o
	s, _ = opStep(s, chromeMsg{ch})
	for _, m := range drainOp(s.init()) {
		s, _ = opStep(s, m)
	}
	if mode == confirmDestructive {
		s, _ = opStep(s, key("enter"))
	}
	a := newApp(ch, Services{}, newFake("hub"))
	a.stack = append(a.stack, s)
	return a
}
