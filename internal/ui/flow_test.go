package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// kinds reports which of popMsg and tea.QuitMsg a command produces.
func kinds(cmd tea.Cmd) (popped, quit bool) {
	for _, m := range drainCmd(cmd) {
		switch m.(type) {
		case popMsg:
			popped = true
		case tea.QuitMsg:
			quit = true
		}
	}
	return popped, quit
}

// doneFixture is the installer on its done screen after a completed run.
func doneFixture(t *testing.T) Model {
	t.Helper()
	m, cmd := startInstall(t, testModel(t))
	m = drain(t, m, cmd)
	if !m.Completed() {
		t.Fatal("fixture: the run did not complete")
	}
	return m
}

// counter is a FinishInstall fake that counts its calls.
type counter struct {
	calls    int
	warnings []string
}

func (c *counter) finish(Model) ([]string, error) {
	c.calls++
	return c.warnings, nil
}

// hostedApp is a router with a fake hub at the bottom and f pushed on it.
func hostedApp(t *testing.T, f flow) (App, fakeScreen) {
	t.Helper()
	hub := newFake("hub")
	a, _ := routerStep(testApp(t, hub), pushMsg{f})
	if a.depth() != 2 {
		t.Fatalf("fixture: depth = %d after push, want 2", a.depth())
	}
	return a, hub
}

func TestHostedModelPopsWhereItUsedToQuit(t *testing.T) {
	settings := func(t *testing.T) Model { m, _ := settingsModel(t, "", nil); return m }
	cases := []struct {
		name  string
		model func(t *testing.T) Model
		key   tea.KeyMsg
	}{
		{"q at select", testModel, key("q")},
		{"enter at done", doneFixture, key("enter")},
		{"settings save", settings, key("enter")},
		{"settings discard", settings, escKey()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := c.model(t)
			m.hosted = true
			_, cmd := m.Update(c.key)
			popped, quit := kinds(cmd)
			if quit || !popped {
				t.Errorf("hosted: popped=%v quit=%v, want a pop and no quit", popped, quit)
			}

			m.hosted = false
			_, cmd = m.Update(c.key)
			popped, quit = kinds(cmd)
			if !quit || popped {
				t.Errorf("standalone: popped=%v quit=%v, want a quit as before", popped, quit)
			}
		})
	}
}

func TestFlowShowsTheModelViewUnchanged(t *testing.T) {
	m := testModel(t)
	a, _ := hostedApp(t, newFlow(m, nil))
	got := a.View()
	want := a.top().(flow).m.View()
	if got != want {
		t.Errorf("the router reframed the hosted model:\n%s\nwant:\n%s", got, want)
	}
}

func TestClosingDoneCallsFinishInstallExactlyOnce(t *testing.T) {
	c := &counter{warnings: []string{"could not record the repository location"}}
	a, hub := hostedApp(t, newFlow(doneFixture(t), c.finish))

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
	if c.calls != 1 {
		t.Errorf("FinishInstall ran %d times, want 1", c.calls)
	}
	if a.depth() != 1 {
		t.Errorf("depth = %d after closing done, want 1 (back at the hub)", a.depth())
	}
	if !hub.received(installFinishedMsg{warnings: c.warnings}) {
		t.Errorf("the hub never got the warnings; it received %v", *hub.log)
	}
}

func TestFinishInstallErrorReachesTheHub(t *testing.T) {
	boom := errors.New("boom")
	a, hub := hostedApp(t, newFlow(doneFixture(t), func(Model) ([]string, error) { return nil, boom }))
	a, _ = routerStep(a, key("enter"))
	if a.depth() != 1 {
		t.Errorf("depth = %d, want 1", a.depth())
	}
	if !hub.received(installFinishedMsg{err: boom}) {
		t.Errorf("the hub never got the error; it received %v", *hub.log)
	}
}

func TestQuittingAtSelectNeverFinishes(t *testing.T) {
	c := &counter{}
	a, _ := hostedApp(t, newFlow(testModel(t), c.finish))
	a, quit := routerStep(a, key("q"))
	if quit {
		t.Fatal("q in a hosted installer ended the program")
	}
	if a.depth() != 1 {
		t.Errorf("depth = %d, want 1", a.depth())
	}
	if c.calls != 0 {
		t.Errorf("FinishInstall ran %d times after quitting at select, want 0", c.calls)
	}
}

func TestAStoppedRunNeverFinishes(t *testing.T) {
	c := &counter{}
	a, _ := hostedApp(t, newFlow(testModel(t), c.finish))

	// enter starts the run; q lands while the first step is in flight.
	next, inFlight := a.Update(key("enter"))
	a = next.(App)
	if a.top().busy() != busyCancellable {
		t.Errorf("busy = %v while installing, want cancellable", a.top().busy())
	}
	a, quit := routerStep(a, key("q"))
	if quit {
		t.Fatal("q mid-install ended the program")
	}
	for _, msg := range drainCmd(inFlight) {
		a, _ = routerStep(a, msg)
	}
	fm := a.top().(flow).m
	if fm.Screen() != ScreenDone || !fm.Stopped() {
		t.Fatalf("fixture: screen %v stopped %v, want a stopped done screen", fm.Screen(), fm.Stopped())
	}

	a, _ = routerStep(a, key("enter"))
	if a.depth() != 1 {
		t.Errorf("depth = %d after closing a stopped run, want 1", a.depth())
	}
	if c.calls != 0 {
		t.Errorf("FinishInstall ran %d times for a stopped run, want 0", c.calls)
	}
}

func TestSavingHostedSettingsChangesTheRouterTheme(t *testing.T) {
	m, _ := settingsModel(t, "", nil)
	hub := newFake("hub")
	root := testApp(t, hub)
	// A colour terminal: on a plain one every theme resolves to mono.
	root.theme = trueColorTheme(t, DefaultThemeName)
	a, _ := routerStep(root, pushMsg{newFlow(m, nil)})
	if strings.HasPrefix(a.theme.Name, "catppuccin") {
		t.Fatalf("fixture: router already on %q", a.theme.Name)
	}
	a, _ = routerStep(a, key("right"))
	a, quit := routerStep(a, key("enter"))
	if quit {
		t.Fatal("saving hosted settings ended the program")
	}
	if a.depth() != 1 {
		t.Errorf("depth = %d after save, want 1", a.depth())
	}
	if !strings.HasPrefix(a.theme.Name, "catppuccin") {
		t.Errorf("router theme after save = %q, want catppuccin", a.theme.Name)
	}
	var adopted bool
	for _, msg := range *hub.log {
		if cm, ok := msg.(chromeMsg); ok && strings.HasPrefix(cm.c.theme.Name, "catppuccin") {
			adopted = cm.c.width == a.width && cm.c.height == a.height
		}
	}
	if !adopted {
		t.Error("the hub was not handed the new theme at the router's size")
	}
}

func TestDiscardingHostedSettingsKeepsTheRouterTheme(t *testing.T) {
	m, _ := settingsModel(t, "", nil)
	a, _ := hostedApp(t, newFlow(m, nil))
	before := a.theme.Name
	a, _ = routerStep(a, key("right"))
	a, _ = routerStep(a, escKey())
	if a.depth() != 1 || a.theme.Name != before {
		t.Errorf("depth %d theme %q, want 1 and %q", a.depth(), a.theme.Name, before)
	}
}

func TestFlowBusyAndTypingFollowTheModel(t *testing.T) {
	m := testModel(t)
	f := newFlow(m, nil)
	if f.busy() != busyIdle || f.typing() {
		t.Errorf("select: busy %v typing %v, want idle and false", f.busy(), f.typing())
	}

	w := m
	w.screen, w.welcome.mode = ScreenWelcome, modePath
	if f := newFlow(w, nil); !f.typing() || f.busy() != busyIdle {
		t.Errorf("path field: busy %v typing %v, want idle and true", f.busy(), f.typing())
	}
	w.welcome.mode = modeCloning
	if f := newFlow(w, nil); f.busy() != busyCancellable || f.typing() {
		t.Errorf("cloning: busy %v typing %v, want cancellable and false", f.busy(), f.typing())
	}
	if f.keys() != nil {
		t.Error("the flow lists keys; the hosted model owns its key map")
	}
}

func TestInstallFlowHostsTheServicesInstaller(t *testing.T) {
	c := &counter{}
	f, err := installFlow(Services{
		Installer:     func() (Model, error) { return doneFixture(t), nil },
		FinishInstall: c.finish,
	})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := hostedApp(t, f)
	a, _ = routerStep(a, key("enter"))
	if c.calls != 1 || a.depth() != 1 {
		t.Errorf("calls %d depth %d, want 1 and 1", c.calls, a.depth())
	}

	boom := errors.New("no repo")
	if _, err := installFlow(Services{Installer: func() (Model, error) { return Model{}, boom }}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the installer's error", err)
	}
}
