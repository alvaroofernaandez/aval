package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeScreen records every message it receives. Its log is shared by
// pointer, so copies returned from update keep writing to the same record.
type fakeScreen struct {
	name  string
	state busyKind
	text  bool
	// next is pushed on enter.
	next screen
	log  *[]tea.Msg
	t    ticket
}

// startMsg asks the fake to start an async job; its result is jobDone.
type startMsg struct{ id int }
type jobDone struct{ id int }

func newFake(name string) fakeScreen {
	return fakeScreen{name: name, log: &[]tea.Msg{}}
}

func (f fakeScreen) init() tea.Cmd { return nil }

func (f fakeScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	*f.log = append(*f.log, msg)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" && f.next != nil {
			return f, push(f.next)
		}
	case startMsg:
		id := msg.id
		return f, f.t.issue(func() tea.Msg { return jobDone{id} })
	}
	if inner, ok := f.t.accept(msg); ok {
		*f.log = append(*f.log, inner)
	}
	return f, nil
}

func (f fakeScreen) keys() keyMap {
	return keyMap{{keys: []string{"enter"}, action: actInstall, label: "enter", desc: "open " + f.name, core: true}}
}

func (f fakeScreen) view() view {
	return view{body: func(l layout, h int) []string { return []string{"screen " + f.name} }}
}

func (f fakeScreen) busy() busyKind { return f.state }
func (f fakeScreen) typing() bool   { return f.text }

func (f fakeScreen) received(want tea.Msg) bool {
	for _, m := range *f.log {
		if reflect.DeepEqual(m, want) {
			return true
		}
	}
	return false
}

func testApp(t *testing.T, root screen) App {
	t.Helper()
	return newApp(chrome{theme: goldenTheme(t, true), version: "0.2.0", platform: "macOS (arm64)", width: 80, height: 24}, Services{}, root)
}

// step sends msg and feeds back every message its command produces, the way
// the tea runtime would, except tea.Quit, which is reported.
func routerStep(a App, msg tea.Msg) (App, bool) {
	next, cmd := a.Update(msg)
	a = next.(App)
	quit := false
	for _, m := range drainCmd(cmd) {
		if _, ok := m.(tea.QuitMsg); ok {
			quit = true
			continue
		}
		var q bool
		a, q = routerStep(a, m)
		quit = quit || q
	}
	return a, quit
}

// drain runs cmd and flattens batches.
func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, drainCmd(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func ctrlC() tea.KeyMsg  { return tea.KeyMsg{Type: tea.KeyCtrlC} }
func escKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }

func TestRouterEnterPushesAScreen(t *testing.T) {
	root := newFake("root")
	root.next = newFake("child")
	a, _ := routerStep(testApp(t, root), key("enter"))
	if a.depth() != 2 {
		t.Fatalf("depth = %d after enter, want 2", a.depth())
	}
	if v := a.View(); !strings.Contains(v, "screen child") {
		t.Errorf("the pushed screen is not on top:\n%s", v)
	}
}

func TestRouterEscPopsAScreen(t *testing.T) {
	root := newFake("root")
	root.next = newFake("child")
	a, _ := routerStep(testApp(t, root), key("enter"))
	a, quit := routerStep(a, escKey())
	if a.depth() != 1 || quit {
		t.Fatalf("depth = %d quit = %v after esc, want 1 and no quit", a.depth(), quit)
	}
	if v := a.View(); !strings.Contains(v, "screen root") {
		t.Errorf("esc did not return to the root:\n%s", v)
	}
}

func TestRouterEscAtTheRootDoesNothing(t *testing.T) {
	a := testApp(t, newFake("root"))
	before := a.View()
	a, quit := routerStep(a, escKey())
	if a.depth() != 1 || quit || a.Quitting() {
		t.Errorf("esc at the root: depth %d quit %v", a.depth(), quit)
	}
	if a.View() != before {
		t.Errorf("esc at the root changed the view")
	}
}

func TestRouterQuitsWhenIdle(t *testing.T) {
	for _, k := range []tea.KeyMsg{key("q"), ctrlC()} {
		a, quit := routerStep(testApp(t, newFake("root")), k)
		if !quit || !a.Quitting() {
			t.Errorf("%s while idle: quit = %v, Quitting = %v, want both", k, quit, a.Quitting())
		}
	}
}

func TestRouterStopsWhenBusy(t *testing.T) {
	for _, k := range []tea.KeyMsg{key("q"), ctrlC(), escKey()} {
		f := newFake("root")
		f.state = busyCancellable
		a, quit := routerStep(testApp(t, f), k)
		if quit || a.Quitting() {
			t.Errorf("%s while busy quit the program", k)
		}
		if !f.received(stopMsg{}) {
			t.Errorf("%s while busy did not stop the operation", k)
		}
	}
}

func TestRouterRefusesToQuitWhileAtomic(t *testing.T) {
	root := newFake("root")
	child := newFake("child")
	child.state = busyAtomic
	root.next = child
	a, _ := routerStep(testApp(t, root), key("enter"))
	for _, k := range []tea.KeyMsg{key("q"), ctrlC(), escKey()} {
		var quit bool
		a, quit = routerStep(a, k)
		if quit || a.Quitting() {
			t.Errorf("%s quit during an atomic operation", k)
		}
		if a.depth() != 2 {
			t.Errorf("%s left an atomic operation (depth %d)", k, a.depth())
		}
		if child.received(stopMsg{}) {
			t.Errorf("%s stopped an atomic operation", k)
		}
	}
}

func TestRouterBroadcastsSizeToEveryScreen(t *testing.T) {
	root := newFake("root")
	child := newFake("child")
	root.next = child
	a, _ := routerStep(testApp(t, root), key("enter"))
	size := tea.WindowSizeMsg{Width: 100, Height: 30}
	a, _ = routerStep(a, size)
	for _, f := range []fakeScreen{root, child} {
		if !f.received(size) {
			t.Errorf("screen %s did not receive the size", f.name)
		}
	}
	if a.width != 100 || a.height != 30 {
		t.Errorf("chrome size = %dx%d, want 100x30", a.width, a.height)
	}
}

func TestRouterDropsStaleGenerations(t *testing.T) {
	f := newFake("root")
	a := testApp(t, f)
	next, first := a.Update(startMsg{1})
	a = next.(App)
	next, second := a.Update(startMsg{2})
	a = next.(App)
	stale, fresh := drainCmd(first), drainCmd(second)
	if len(stale) != 1 || len(fresh) != 1 {
		t.Fatalf("issue returned %d and %d messages, want one each", len(stale), len(fresh))
	}
	a, _ = routerStep(a, stale[0])
	a, _ = routerStep(a, fresh[0])
	if f.received(jobDone{1}) {
		t.Errorf("the stale result reached the screen")
	}
	if !f.received(jobDone{2}) {
		t.Errorf("the current result was dropped")
	}
}

func TestRouterHelpListsScreenAndGlobalKeys(t *testing.T) {
	root := newFake("root")
	root.next = newFake("child")
	a, _ := routerStep(testApp(t, root), key("enter"))
	a, _ = routerStep(a, key("?"))
	v := a.View()
	for _, want := range []string{"Keys", "open child", "back", "help", "quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("help does not list %q:\n%s", want, v)
		}
	}
	a, _ = routerStep(a, escKey())
	if v := a.View(); strings.Contains(v, "Keys") || a.depth() != 2 {
		t.Errorf("esc did not close the help first (depth %d):\n%s", a.depth(), v)
	}
}

func TestRouterHintsMergeGlobalKeys(t *testing.T) {
	root := newFake("root")
	root.next = newFake("child")
	a, _ := routerStep(testApp(t, root), key("enter"))
	hints := a.keys().hints(a.theme.Glyphs)
	var got []string
	for _, h := range hints {
		got = append(got, h.key+" "+h.desc)
	}
	want := []string{"enter open child", "esc back", "? help", "q quit"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("hints = %q, want %q", got, want)
	}
	if v := a.View(); !strings.Contains(v, "esc back") {
		t.Errorf("the hint line does not show the global keys:\n%s", v)
	}
}

func TestRouterTextFieldTypesQAndQuestionMark(t *testing.T) {
	f := newFake("root")
	f.text = true
	a := testApp(t, f)
	for _, k := range []string{"q", "?"} {
		var quit bool
		a, quit = routerStep(a, key(k))
		if quit || a.Quitting() || a.help {
			t.Errorf("%q with a text field open acted globally (quit %v help %v)", k, quit, a.help)
		}
		if !f.received(key(k)) {
			t.Errorf("%q did not reach the text field", k)
		}
	}
	if _, quit := routerStep(a, ctrlC()); !quit {
		t.Errorf("ctrl+c with a text field open did not quit")
	}
}
