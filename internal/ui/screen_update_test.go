package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	coreupdate "github.com/alvaroofernaandez/aval/internal/core/update"
)

// updateProbe records what the update view asked of the fake services.
type updateProbe struct {
	previews, pulls, reapplies int
	fetchCtx                   context.Context
}

const npmCommand = "npm update -g @alvaroofernaandez/aval"

func behindPreview() coreupdate.Preview {
	return coreupdate.Preview{
		Status: coreupdate.Status{Current: "a1b2c3d", Latest: "d4e5f6a", Behind: 3},
		Stat: " config/zsh/.zshrc         | 12 +++--\n" +
			" skills/plan-deck/SKILL.md |  4 +-\n" +
			" 2 files changed, 10 insertions(+), 6 deletions(-)",
		Added: []string{"skills/new-skill -> .claude/skills/new-skill"},
	}
}

func upToDatePreview() coreupdate.Preview {
	return coreupdate.Preview{Status: coreupdate.Status{Current: "a1b2c3d", Latest: "a1b2c3d"}}
}

// dirtyPreview is what DryRun returns for a dirty tree: the status, with
// DirtyTree set, and the refusal.
func dirtyPreview() (coreupdate.Preview, error) {
	return coreupdate.Preview{Status: coreupdate.Status{Current: "a1b2c3d", DirtyTree: true}},
		errors.New("update: the repository has uncommitted changes; commit or stash them first")
}

func updateServices(t *testing.T, p *updateProbe, preview coreupdate.Preview, perr error) Services {
	return Services{
		tr:   testTr,
		Home: "/home/u",
		Repo: "/home/u/.aval",
		UpdatePreview: func(ctx context.Context) (coreupdate.Preview, error) {
			p.previews++
			p.fetchCtx = ctx
			return preview, perr
		},
		Pull: func(context.Context) (coreupdate.Status, error) {
			p.pulls++
			return coreupdate.Status{Current: "a1b2c3d", Latest: "d4e5f6a", Behind: 3, Updated: true}, nil
		},
		Reapply: func() (Model, error) {
			p.reapplies++
			return testModel(t), nil
		},
		CachedNotice: func() coreupdate.Notice { return coreupdate.Notice{Current: "0.3.0"} },
	}
}

func updatePhase(t *testing.T, s screen) opPhase {
	t.Helper()
	u, ok := s.(updateView)
	if !ok {
		t.Fatalf("screen is %T, want updateView", s)
	}
	return u.op.phase
}

// openUpdate sends the chrome and runs init, but holds the fetch result.
func openUpdate(t *testing.T, svc Services) (screen, []tea.Msg) {
	t.Helper()
	var s screen = newUpdateView(svc)
	s, _ = opStep(s, chromeMsg{testChrome(t, true)})
	return s, drainOp(s.init())
}

func hasMsg[T any](msgs []tea.Msg) (T, bool) {
	for _, m := range msgs {
		if v, ok := m.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func TestUpdateEscDuringTheFetchCancelsItsContext(t *testing.T) {
	p := &updateProbe{}
	s, _ := openUpdate(t, updateServices(t, p, behindPreview(), nil))
	if s.busy() != busyCancellable {
		t.Fatalf("fetching: busy = %v, want cancellable", s.busy())
	}
	if p.fetchCtx == nil {
		t.Fatal("opening did not fetch")
	}
	_, out := opStep(s, stopMsg{})
	if !errors.Is(p.fetchCtx.Err(), context.Canceled) {
		t.Errorf("fetch context err = %v, want canceled", p.fetchCtx.Err())
	}
	if _, ok := hasMsg[popMsg](out); !ok {
		t.Errorf("esc during the fetch did not go back: %v", out)
	}
}

func TestUpdateStaleFetchResultIsDropped(t *testing.T) {
	p := &updateProbe{}
	s, held := openUpdate(t, updateServices(t, p, behindPreview(), nil))
	s, _ = opStep(s, stopMsg{})
	for _, m := range held {
		s, _ = opStep(s, m)
	}
	if got := updatePhase(t, s); got != phaseLoading {
		t.Errorf("a cancelled fetch result moved the phase to %v", got)
	}
	if strings.Contains(viewText(s), "Update and re-apply") {
		t.Errorf("a cancelled fetch result was shown:\n%s", viewText(s))
	}
}

func TestUpdatePreviewShowsWhatThePullBrings(t *testing.T) {
	p := &updateProbe{}
	s := loaded(t, newUpdateView(updateServices(t, p, behindPreview(), nil)))
	text := viewText(s)
	for _, want := range []string{
		"Update", "~/.aval",
		"3 commits behind origin", "a1b2c3d → d4e5f6a",
		"Managed paths that change", "config/zsh/.zshrc", "12 +++--", "skills/plan-deck/SKILL.md",
		"New manifest entries", "+ skills/new-skill -> ~/.claude/skills/new-skill",
		"› Update and re-apply", "pull with --ff-only, then link what is new",
		"  Back", "change nothing",
		"Files in the way move to ~/.aval-backup first · undo: aval restore latest",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "2 files changed") {
		t.Errorf("the diffstat summary line leaked into the paths:\n%s", text)
	}
	if p.pulls != 0 || p.reapplies != 0 {
		t.Errorf("opening wrote: pulls=%d reapplies=%d", p.pulls, p.reapplies)
	}
}

func TestUpdateDirtyTreeOffersOnlyBack(t *testing.T) {
	p := &updateProbe{}
	prev, err := dirtyPreview()
	s := loaded(t, newUpdateView(updateServices(t, p, prev, err)))
	text := viewText(s)
	for _, want := range []string{"× the repository has uncommitted changes", "commit or stash them in ~/.aval"} {
		if !strings.Contains(text, want) {
			t.Errorf("dirty tree lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "re-apply") || strings.Contains(text, "Re-apply") {
		t.Errorf("dirty tree still offers an action:\n%s", text)
	}
	if km := s.keys(); len(km) != 0 {
		t.Errorf("dirty tree has keys %v, want none (esc back only)", km)
	}
	if _, out := opStep(s, key("enter")); p.pulls != 0 || p.reapplies != 0 || len(out) != 0 {
		t.Errorf("enter on a dirty tree acted: pulls=%d reapplies=%d msgs=%v", p.pulls, p.reapplies, out)
	}
}

func TestUpdateUpToDateOffersReapply(t *testing.T) {
	p := &updateProbe{}
	s := loaded(t, newUpdateView(updateServices(t, p, upToDatePreview(), nil)))
	text := viewText(s)
	for _, want := range []string{"√ up to date (a1b2c3d)", "› Re-apply links anyway", "  Back"} {
		if !strings.Contains(text, want) {
			t.Errorf("up to date lacks %q:\n%s", want, text)
		}
	}
	_, out := opStep(s, key("enter"))
	if p.pulls != 0 {
		t.Errorf("re-apply anyway pulled %d times, want 0", p.pulls)
	}
	if p.reapplies != 1 {
		t.Errorf("re-apply anyway built %d installers, want 1", p.reapplies)
	}
	if _, ok := hasMsg[pushMsg](out); !ok {
		t.Errorf("re-apply anyway pushed nothing: %v", out)
	}
}

func TestUpdatePullHandsOverToReapplying(t *testing.T) {
	p := &updateProbe{}
	s := loaded(t, newUpdateView(updateServices(t, p, behindPreview(), nil)))
	s, out := opStep(s, key("enter")) // the cursor is on the action
	if p.pulls != 1 || p.reapplies != 1 {
		t.Fatalf("pulls=%d reapplies=%d, want 1 and 1", p.pulls, p.reapplies)
	}
	if len(out) != 1 {
		t.Fatalf("hand-over msgs = %v, want one push", out)
	}
	pm, ok := out[0].(pushMsg)
	if !ok {
		t.Fatalf("hand-over msg %T, want pushMsg", out[0])
	}
	f, ok := pm.s.(flow)
	if !ok {
		t.Fatalf("pushed %T, want the hosted installer flow", pm.s)
	}
	if !f.m.hosted || f.m.Screen() != ScreenInstall {
		t.Errorf("hosted=%v screen=%v, want a hosted run on the progress screen", f.m.hosted, f.m.Screen())
	}
	if !strings.Contains(f.frame(), "Re-applying") || strings.Contains(f.frame(), "Installing") {
		t.Errorf("progress is not titled Re-applying:\n%s", f.frame())
	}
	m := drain(t, f.m, f.init())
	if m.Screen() != ScreenDone {
		t.Fatalf("the re-apply did not finish: screen %v", m.Screen())
	}
	if want := "Updated a1b2c3d → d4e5f6a. 2 linked, 0 already in place."; !strings.Contains(m.View(), want) {
		t.Errorf("result lacks %q:\n%s", want, m.View())
	}
	// When the flow pops, the update view leaves too: the hub is next.
	if _, out := opStep(s, resumeMsg{}); len(out) != 1 {
		t.Errorf("resumed after the re-apply: msgs %v, want one pop", out)
	} else if _, ok := out[0].(popMsg); !ok {
		t.Errorf("resumed after the re-apply: %T, want popMsg", out[0])
	}
}

func TestUpdatePullErrorStaysInTheView(t *testing.T) {
	p := &updateProbe{}
	svc := updateServices(t, p, behindPreview(), nil)
	svc.Pull = func(context.Context) (coreupdate.Status, error) {
		p.pulls++
		return coreupdate.Status{}, errors.New("fatal: Not possible to fast-forward, aborting.")
	}
	s := loaded(t, newUpdateView(svc))
	s, out := opStep(s, key("enter"))
	if p.reapplies != 0 || len(out) != 0 {
		t.Errorf("a failed pull went on: reapplies=%d msgs=%v", p.reapplies, out)
	}
	if !strings.Contains(viewText(s), "× fatal: Not possible to fast-forward") {
		t.Errorf("pull error not shown:\n%s", viewText(s))
	}
}

func TestUpdateNpmShowsItsCommandAndNeverRunsIt(t *testing.T) {
	p := &updateProbe{}
	svc := updateServices(t, p, upToDatePreview(), nil)
	svc.CachedNotice = func() coreupdate.Notice {
		return coreupdate.Notice{Current: "0.3.0", Latest: "0.3.1", Command: npmCommand}
	}
	s := loaded(t, newUpdateView(svc))
	text := viewText(s)
	for _, want := range []string{"! aval 0.3.1 available", npmCommand} {
		if !strings.Contains(text, want) {
			t.Errorf("npm install lacks %q:\n%s", want, text)
		}
	}
	// The only action is the repo's re-apply; nothing execs the command.
	_, out := opStep(s, key("enter"))
	for _, m := range out {
		if strings.Contains(strings.ToLower(fmt.Sprintf("%T", m)), "exec") {
			t.Errorf("the view ran a process: %T", m)
		}
	}
	if !strings.Contains(fmt.Sprint(s.keys()), "enter") || strings.Contains(text, "› "+npmCommand) {
		t.Errorf("the npm command is offered as an action:\n%s", text)
	}
}

func TestUpdateCurrentBinaryHidesTheCommand(t *testing.T) {
	svc := updateServices(t, &updateProbe{}, upToDatePreview(), nil)
	svc.CachedNotice = func() coreupdate.Notice {
		return coreupdate.Notice{Current: "0.3.0", Latest: "0.3.0", Command: npmCommand}
	}
	if text := viewText(loaded(t, newUpdateView(svc))); strings.Contains(text, npmCommand) {
		t.Errorf("a current binary still names the npm command:\n%s", text)
	}
}

func init() {
	type golden = struct {
		name  string
		build func(t *testing.T, unicode bool) tea.Model
	}
	goldenViews = append(goldenViews,
		golden{"update", func(t *testing.T, unicode bool) tea.Model {
			return goldenOpViewApp(t, unicode, newUpdateView(updateServices(t, &updateProbe{}, behindPreview(), nil)), false)
		}},
		golden{"update_dirty", func(t *testing.T, unicode bool) tea.Model {
			prev, err := dirtyPreview()
			return goldenOpViewApp(t, unicode, newUpdateView(updateServices(t, &updateProbe{}, prev, err)), false)
		}},
		golden{"update_current", func(t *testing.T, unicode bool) tea.Model {
			return goldenOpViewApp(t, unicode, newUpdateView(updateServices(t, &updateProbe{}, upToDatePreview(), nil)), false)
		}},
	)
}

// motionChrome is the test chrome with the resolved motion setting.
func motionChrome(t *testing.T, motion bool) chrome {
	c := testChrome(t, true)
	c.Motion = motion
	return c
}

func hasSpinner(text, label string) bool {
	for _, f := range spinFrames {
		if strings.Contains(text, f+" "+label) {
			return true
		}
	}
	return false
}

// openUpdateWith is openUpdate with the chrome's motion setting; it returns
// the command the chrome message produced and the held fetch result.
func openUpdateWith(t *testing.T, svc Services, motion bool) (screen, tea.Cmd, []tea.Msg) {
	t.Helper()
	var s screen = newUpdateView(svc)
	s, cmd := s.update(chromeMsg{motionChrome(t, motion)})
	return s, cmd, drainOp(s.init())
}

func TestUpdateFetchWithMotionOffShowsStaticText(t *testing.T) {
	s, cmd, _ := openUpdateWith(t, updateServices(t, &updateProbe{}, behindPreview(), nil), false)
	text := viewText(s)
	if !strings.Contains(text, "Fetching origin") || hasSpinner(text, "Fetching origin") {
		t.Errorf("motion off: want static Fetching origin, got:\n%s", text)
	}
	if cmd != nil {
		t.Error("motion off: the chrome message scheduled a tick")
	}
	if next, tick := s.update(opTickMsg{}); tick != nil || viewText(next) != text {
		t.Error("motion off: a tick still animates the fetch")
	}
}

func TestUpdateFetchWithMotionOnSpins(t *testing.T) {
	s, cmd, _ := openUpdateWith(t, updateServices(t, &updateProbe{}, behindPreview(), nil), true)
	if !hasSpinner(viewText(s), "Fetching origin") {
		t.Errorf("motion on: no spinner beside the fetch:\n%s", viewText(s))
	}
	if cmd == nil {
		t.Error("motion on: the chrome message scheduled no tick")
	}
}

func TestUpdatePullWithMotionOffShowsStaticText(t *testing.T) {
	s := loaded(t, newUpdateView(updateServices(t, &updateProbe{}, behindPreview(), nil))) // motion off
	s, _ = s.update(key("enter"))
	text := viewText(s)
	if !strings.Contains(text, "Pulling") || hasSpinner(text, "Pulling") {
		t.Errorf("motion off: want static Pulling, got:\n%s", text)
	}
	if next, tick := s.update(opTickMsg{}); tick != nil || viewText(next) != text {
		t.Error("motion off: a tick still animates the pull")
	}
}

func TestUpdatePullWithMotionOnSpins(t *testing.T) {
	var s screen = newUpdateView(updateServices(t, &updateProbe{}, behindPreview(), nil))
	s, _ = opStep(s, chromeMsg{motionChrome(t, true)})
	for _, m := range drainOp(s.init()) {
		s, _ = opStep(s, m)
	}
	s, _ = s.update(key("enter"))
	if !hasSpinner(viewText(s), "Pulling") {
		t.Errorf("motion on: no spinner beside the pull:\n%s", viewText(s))
	}
}

func TestNetworkWaitFollowsTheChromeMotion(t *testing.T) {
	s := loaded(t, testOperation(t, opNetwork, confirmInline, &opProbe{})) // chrome: motion off
	s, _ = s.update(key("enter"))
	if hasSpinner(viewText(s), "Removing links") {
		t.Errorf("chrome motion off but the operation spins:\n%s", viewText(s))
	}
}

func TestNewHubCarriesTheMotionSetting(t *testing.T) {
	for _, motion := range []bool{true, false} {
		a := NewHub(goldenTheme(t, true), "macOS (arm64)", Services{}, motion, "").(App)
		if a.chrome.Motion != motion {
			t.Errorf("NewHub(motion=%v): chrome.Motion = %v", motion, a.chrome.Motion)
		}
	}
}

// reapplyFlow runs the update view's hand-over and returns the pushed flow.
func reapplyFlowOf(t *testing.T, svc Services) flow {
	t.Helper()
	s := loaded(t, newUpdateView(svc))
	_, out := opStep(s, key("enter"))
	for _, m := range out {
		if pm, ok := m.(pushMsg); ok {
			return pm.s.(flow)
		}
	}
	t.Fatalf("no hand-over: %v", out)
	return flow{}
}

func TestClosingTheReapplyFinishesItsRunExactlyOnce(t *testing.T) {
	calls := 0
	svc := updateServices(t, &updateProbe{}, behindPreview(), nil)
	svc.FinishReapply = func() error { calls++; return nil }
	f := reapplyFlowOf(t, svc)
	f.m = drain(t, f.m, f.init())
	a, _ := hostedApp(t, f)

	// Two closes before the result lands: the second must not finish again.
	next, cmd1 := a.Update(key("enter"))
	a = next.(App)
	next, cmd2 := a.Update(key("enter"))
	a = next.(App)
	for _, cmd := range []tea.Cmd{cmd1, cmd2} {
		for _, msg := range drainCmd(cmd) {
			a, _ = routerStep(a, msg)
		}
	}
	if calls != 1 {
		t.Errorf("FinishReapply ran %d times, want 1", calls)
	}
	if a.depth() != 1 {
		t.Errorf("depth = %d after closing the re-apply, want 1", a.depth())
	}
}

func TestClosingAStoppedReapplyStillFinishesItsRun(t *testing.T) {
	calls := 0
	svc := updateServices(t, &updateProbe{}, behindPreview(), nil)
	svc.FinishReapply = func() error { calls++; return nil }
	f := reapplyFlowOf(t, svc)
	// Pushed without running its init, so the first step stays in flight.
	a := testApp(t, newFake("hub"))
	a.stack = append(a.stack, f)

	inFlight := f.init()
	a, _ = routerStep(a, key("q"))
	for _, msg := range drainCmd(inFlight) {
		a, _ = routerStep(a, msg)
	}
	fm := a.top().(flow).m
	if fm.Screen() != ScreenDone || !fm.Stopped() {
		t.Fatalf("fixture: screen %v stopped %v, want a stopped done screen", fm.Screen(), fm.Stopped())
	}
	next, cmd := a.Update(key("enter"))
	a = next.(App)
	for _, msg := range drainCmd(cmd) {
		a, _ = routerStep(a, msg)
	}
	if calls != 1 || a.depth() != 1 {
		t.Errorf("calls %d depth %d, want 1 and 1", calls, a.depth())
	}
}

func TestReapplyFinishErrorReachesTheHub(t *testing.T) {
	boom := errors.New("boom")
	svc := updateServices(t, &updateProbe{}, behindPreview(), nil)
	svc.FinishReapply = func() error { return boom }
	f := reapplyFlowOf(t, svc)
	f.m = drain(t, f.m, f.init())
	a, hub := hostedApp(t, f)
	next, cmd := a.Update(key("enter"))
	a = next.(App)
	for _, msg := range drainCmd(cmd) {
		a, _ = routerStep(a, msg)
	}
	if !hub.received(installFinishedMsg{err: boom}) {
		t.Errorf("the hub never got the error; it received %v", *hub.log)
	}
}
