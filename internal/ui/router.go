package ui

import (
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// The router hosts the hub and its views as a stack (DESIGN.md section 10,
// Navigation). The hub is the bottom; enter pushes a view, esc pops one.
// Global keys live here and are merged into each screen's key map, so the
// hint line and the `?` overlay keep a single source.

// busyKind says whether a screen runs an operation, and how it may end.
type busyKind int

const (
	// busyIdle: nothing runs; esc goes back and q quits.
	busyIdle busyKind = iota
	// busyCancellable: a stepped or network operation. esc, q and ctrl+c send
	// the screen a stopMsg; a stepped run ends after the current step, a
	// network one cancels its context.
	busyCancellable
	// busyAtomic: a write that must not be cut (sync, restore, uninstall, a
	// doctor fix). Keys are inert and the program cannot quit.
	busyAtomic
)

// screen is one view on the router's stack. Screens are values: update
// returns the next state, as tea.Model does.
type screen interface {
	init() tea.Cmd
	update(msg tea.Msg) (screen, tea.Cmd)
	// keys is the screen's own map, without the global keys.
	keys() keyMap
	view() view
	busy() busyKind
}

// textField is implemented by screens with an inline text field. While it
// reports true every key but ctrl+c goes to the screen, so q and ? are typed
// and esc closes the field.
type textField interface{ typing() bool }

// selfFramed is a screen that draws its whole frame and routes its own keys:
// the hosted installer and settings Model (flow.go). The router hands it
// every key untouched and shows frame as is; its keys() is nil.
type selfFramed interface{ frame() string }

type (
	// pushMsg puts a screen on top of the stack.
	pushMsg struct{ s screen }
	// popMsg returns to the screen below; at the root it does nothing.
	popMsg struct{}
	// quitMsg ends the program.
	quitMsg struct{}
	// stopMsg is sent by the router to a busy screen: stop after the current
	// step, or cancel the network wait.
	stopMsg struct{}
	// chromeMsg hands a screen the current frame (theme, labels and size).
	// The router sends it on push and broadcasts it on every resize. A
	// screen sends it to the router to change the theme (a settings save):
	// the router adopts c's theme, keeps its own size and labels, and
	// broadcasts the result.
	chromeMsg struct{ c chrome }
	// resumeMsg tells a screen it is on top again after the one above it
	// popped, so it can refresh what the popped view may have changed.
	resumeMsg struct{}
)

func push(s screen) tea.Cmd { return func() tea.Msg { return pushMsg{s} } }
func pop() tea.Cmd          { return func() tea.Msg { return popMsg{} } }
func quit() tea.Cmd         { return func() tea.Msg { return quitMsg{} } }

// actPop is the router's esc: pop the top screen. Negative, like the other
// screen-local actions, so it never collides with the shared ones.
const actPop action = -30

// App is the router: a tea.Model holding the frame, the screen stack, the
// help overlay and the services the views reach the outside world through.
type App struct {
	chrome
	stack []screen
	// help is the `?` overlay over the top screen; helpTop its scroll.
	help    bool
	helpTop int

	services Services
	quitting bool
}

// newApp builds a router with root at the bottom of the stack.
func newApp(c chrome, svc Services, root screen) App {
	return App{chrome: c, stack: []screen{root}, services: svc}
}

// Quitting reports a deliberate exit (q, ctrl+c or a quitMsg).
func (a App) Quitting() bool { return a.quitting }

func (a App) depth() int  { return len(a.stack) }
func (a App) top() screen { return a.stack[len(a.stack)-1] }

func (a App) typing() bool {
	tf, ok := a.top().(textField)
	return ok && tf.typing()
}

func (a App) Init() tea.Cmd {
	return tea.Batch(a.top().init(), a.deliver(len(a.stack)-1, chromeMsg{a.chrome}))
}

// deliver sends msg to the screen at i and returns its command. It mutates
// the stack slot, so callers own a.
func (a App) deliver(i int, msg tea.Msg) tea.Cmd {
	next, cmd := a.stack[i].update(msg)
	a.stack[i] = next
	return cmd
}

// broadcast sends msg to every screen, bottom first.
func (a App) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(a.stack))
	for i := range a.stack {
		cmds[i] = a.deliver(i, msg)
	}
	return tea.Batch(cmds...)
}

// Update handles one message.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Copy the stack so earlier App values stay untouched.
	a.stack = append([]screen(nil), a.stack...)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.key(msg)
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a, tea.Batch(a.broadcast(msg), a.broadcast(chromeMsg{a.chrome}))
	case pushMsg:
		a.stack = append(a.stack, msg.s)
		a.help = false
		i := len(a.stack) - 1
		return a, tea.Batch(msg.s.init(), a.deliver(i, chromeMsg{a.chrome}))
	case popMsg:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
			a.help = false
			return a, a.deliver(len(a.stack)-1, resumeMsg{})
		}
		return a, nil
	case quitMsg:
		a.quitting = true
		return a, tea.Quit
	case chromeMsg:
		a.theme = msg.c.theme
		return a, a.broadcast(chromeMsg{a.chrome})
	}
	return a, a.broadcast(msg)
}

// key routes a key: the help overlay first, then an open text field, then
// the top screen's own map, then the global keys. A key no map lists does
// nothing.
func (a App) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if a.help {
		act, ok := a.helpKeys().lookup(k)
		if !ok {
			return a, nil
		}
		return a.helpAction(act)
	}
	if _, ok := a.top().(selfFramed); ok {
		return a, a.deliver(len(a.stack)-1, msg)
	}
	if a.typing() && k != "ctrl+c" {
		return a, a.deliver(len(a.stack)-1, msg)
	}
	if _, ok := a.top().keys().lookup(k); ok {
		return a, a.deliver(len(a.stack)-1, msg)
	}
	act, ok := a.globalKeys().lookup(k)
	if !ok {
		return a, nil
	}
	return a.global(act)
}

// global performs a router action.
func (a App) global(act action) (tea.Model, tea.Cmd) {
	switch act {
	case actPop:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
			return a, a.deliver(len(a.stack)-1, resumeMsg{})
		}
	case actHelp:
		a.help, a.helpTop = true, 0
	case actQuit:
		a.quitting = true
		return a, tea.Quit
	case actStop:
		return a, a.deliver(len(a.stack)-1, stopMsg{})
	}
	return a, nil
}

func (a App) helpAction(act action) (tea.Model, tea.Cmd) {
	switch act {
	case actHelp:
		a.help = false
	case actUp:
		a.helpTop = max(a.helpTop-1, 0)
	case actDown:
		a.helpTop = min(a.helpTop+1, max(len(a.keys())-1, 0))
	case actQuit, actStop:
		return a.global(act)
	}
	return a, nil
}

var (
	backBinding     = binding{keys: []string{"esc"}, action: actPop, label: "esc", desc: "back", core: true}
	cancelBinding   = binding{keys: []string{"esc"}, action: actStop, label: "esc", desc: "cancel", core: true}
	stopBinding     = binding{keys: []string{"q", "ctrl+c"}, action: actStop, label: "ctrl+c", desc: "stops after the current step"}
	ctrlCQuit       = binding{keys: []string{"ctrl+c"}, action: actQuit, desc: "quit"}
	atomicInert     = binding{keys: []string{"esc", "q", "ctrl+c"}, action: actNone, desc: "wait: this step cannot be interrupted"}
	helpCloseScroll = keyMap{
		{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: "scroll"},
		{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: "scroll"},
		{keys: []string{"esc", "?"}, action: actHelp, label: "esc", desc: "close", core: true},
	}
)

// globalKeys is the router's part of the key map, by state (the Navigation
// table in DESIGN.md section 10).
func (a App) globalKeys() keyMap {
	if a.typing() {
		return keyMap{ctrlCQuit}
	}
	switch a.top().busy() {
	case busyCancellable:
		return keyMap{cancelBinding, helpBinding, stopBinding}
	case busyAtomic:
		return keyMap{helpBinding, atomicInert}
	}
	var km keyMap
	if len(a.stack) > 1 {
		km = append(km, backBinding)
	}
	return append(km, helpBinding, quitBinding)
}

// keys is the merged map: the top screen's bindings, then the global ones.
// The hint line and the help overlay both read it.
func (a App) keys() keyMap {
	return append(append(keyMap{}, a.top().keys()...), a.globalKeys()...)
}

// helpKeys is the map while the overlay is open: close and scroll, plus
// ctrl+c with its meaning for the current state.
func (a App) helpKeys() keyMap {
	km := append(keyMap{}, helpCloseScroll...)
	switch a.top().busy() {
	case busyIdle:
		km = append(km, binding{keys: []string{"ctrl+c"}, action: actQuit})
	case busyCancellable:
		km = append(km, binding{keys: []string{"ctrl+c"}, action: actStop})
	}
	return km
}

// View renders the top screen, or the help overlay, inside the frame.
func (a App) View() string {
	if f, ok := a.top().(selfFramed); ok && !a.help {
		return f.frame()
	}
	if a.help {
		rows := a.keyRows(a.keys())
		top := a.helpTop
		return a.renderWith(view{
			body:   func(l layout, h int) []string { return a.keysBody(rows, top, l, h) },
			status: "help",
		}, a.helpKeys())
	}
	return a.renderWith(a.top().view(), a.keys())
}

// Async results carry a generation, so a result that arrives after its
// screen moved on (a cancelled fetch, a re-run, a popped view) is dropped.
// Generations come from one process-wide counter: no two requests share one,
// even across screens.

var lastGen atomic.Uint64

// genMsg is an async result stamped with the generation that asked for it.
// The router broadcasts it like any other non-key message.
type genMsg struct {
	gen uint64
	msg tea.Msg
}

// ticket is a screen's current async generation. A screen keeps one per
// operation slot:
//
//	cmd := s.fetch.issue(load)          // on start; older results go stale
//	if msg, ok := s.fetch.accept(msg)    // in update; false for stale ones
//	s.fetch.void()                       // on cancel
type ticket struct{ gen uint64 }

// issue starts a new generation and stamps cmd's result with it.
func (t *ticket) issue(cmd tea.Cmd) tea.Cmd {
	t.gen = lastGen.Add(1)
	if cmd == nil {
		return nil
	}
	gen := t.gen
	return func() tea.Msg { return genMsg{gen: gen, msg: cmd()} }
}

// void makes any outstanding result stale.
func (t *ticket) void() { t.gen = 0 }

// accept unwraps msg when it is a result of the current generation.
func (t ticket) accept(msg tea.Msg) (tea.Msg, bool) {
	g, ok := msg.(genMsg)
	if !ok || t.gen == 0 || g.gen != t.gen {
		return nil, false
	}
	return g.msg, true
}
