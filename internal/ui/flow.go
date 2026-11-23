package ui

import tea "github.com/charmbracelet/bubbletea"

// flow hosts the existing installer or settings Model on the router's stack
// (plan step 3, "Host Model, do not dissolve it"). The Model keeps its own
// key map, help overlay and frame: the router hands the flow every key and
// shows the Model's View unchanged (see selfFramed in router.go). Hosted, the
// Model's exits pop back to the hub instead of ending the program.
//
// Closing the done screen of a completed install runs finish once, before the
// pop; quitting at select or welcome, or closing a stopped run, never does.
// A re-apply flow (reapplyFlow) also finishes when stopped.
type flow struct {
	m Model
	// finish is Services.FinishInstall; nil for settings, or when the
	// install needs no post-install steps.
	finish func(Model) ([]string, error)
	// finishStopped runs finish when a stopped run closes too: a re-apply's
	// backup run must be committed whatever happened, so it can be undone.
	finishStopped bool
	// finishing is set while finish runs: keys are inert and the flow pops
	// when installFinishedMsg arrives.
	finishing bool
}

// installFinishedMsg is what FinishInstall returned for a hosted install,
// sent once when its done screen closes. The router broadcasts it to the
// whole stack, bottom first, so the hub reads it before the flow pops: the
// hub shows the warnings (and err, the step that failed) in its status line
// and refreshes its panel. warnings are lines like "could not record the
// repository location: ...", without a "warning:" prefix.
type installFinishedMsg struct {
	warnings []string
	err      error
}

// newFlow hosts m. finish runs after a completed install; pass nil for the
// settings screen.
func newFlow(m Model, finish func(Model) ([]string, error)) flow {
	m.hosted = true
	return flow{m: m, finish: finish}
}

// reapplyFlow hosts the re-apply: finish only closes the run's backup (the
// service's FinishReapply), completed or stopped. Without the service it is a
// plain flow.
func reapplyFlow(m Model, finish func() error) flow {
	f := newFlow(m, nil)
	if finish != nil {
		f.finish = func(Model) ([]string, error) { return nil, finish() }
		f.finishStopped = true
	}
	return f
}

// installFlow builds the hosted installer from the services: select when a
// repository is known, welcome otherwise.
func installFlow(svc Services) (flow, error) {
	m, err := svc.Installer()
	if err != nil {
		return flow{}, err
	}
	return newFlow(m, svc.FinishInstall), nil
}

func (f flow) init() tea.Cmd { return f.m.Init() }

func (f flow) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		// The router's frame: its size and theme (a settings save adopts a
		// new one). The Model keeps its own lockup labels.
		f.m.width, f.m.height = msg.c.width, msg.c.height
		f.m = f.m.WithTheme(msg.c.theme)
		return f, nil
	case installFinishedMsg:
		if !f.finishing {
			return f, nil
		}
		f.finishing = false
		return f, pop()
	case tea.KeyMsg:
		if f.finishing {
			return f, nil
		}
	}
	left := f.m.Quitting
	next, cmd := f.m.Update(msg)
	f.m = next.(Model)
	closed := f.m.Completed() || (f.finishStopped && f.m.Screen() == ScreenDone)
	if !left && f.m.Quitting && closed && f.finish != nil {
		// The Model asked to pop; the flow pops once finish has answered.
		f.finishing = true
		finish, m := f.finish, f.m
		return f, func() tea.Msg {
			w, err := finish(m)
			return installFinishedMsg{warnings: w, err: err}
		}
	}
	return f, cmd
}

// keys is nil: the Model routes its own keys and draws its own hint line.
func (f flow) keys() keyMap { return nil }

// view is unused while the flow is on top: the router shows frame instead.
func (f flow) view() view {
	return view{body: func(layout, int) []string { return nil }}
}

// frame is the Model's View, unchanged.
func (f flow) frame() string { return f.m.View() }

// busy maps the Model's state onto the router's kinds. Installing is a
// stepped run (a stop finishes the step in flight) and the welcome clone a
// network wait, both cancellable; the post-install steps are atomic.
func (f flow) busy() busyKind {
	switch {
	case f.finishing:
		return busyAtomic
	case f.m.screen == ScreenInstall:
		return busyCancellable
	case f.m.screen == ScreenWelcome && f.m.welcome.mode == modeCloning:
		return busyCancellable
	}
	return busyIdle
}

// typing reports the welcome path field open: q and ? are typed there.
func (f flow) typing() bool {
	return !f.m.help && f.m.screen == ScreenWelcome && f.m.welcome.mode == modePath
}
