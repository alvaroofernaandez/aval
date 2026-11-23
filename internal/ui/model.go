package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// This file holds routing and state only. The shared chrome is in frame.go,
// the bindings in keys.go, and each view in its own screen_*.go file.

// Screen is which of the three views is showing.
type Screen int

const (
	ScreenSelect  Screen = iota // choose what to install
	ScreenInstall               // work in progress
	ScreenDone                  // summary
)

// UnavailableGroup is a manifest group that does not apply to this machine.
// They are shown rather than hidden: someone on Windows should see that the
// terminal environment exists and why it is not offered, instead of wondering
// what is missing.
type UnavailableGroup struct {
	Label  string
	Reason string
}

// Options is what the model needs to know about the build and the terminal.
type Options struct {
	// Version is the build version shown in the lockup, for example "0.2.0".
	Version string
	// Platform is the right-aligned lockup label; see PlatformLabel.
	Platform string
	// Caps is the detected terminal. It picks the theme variant and glyph tier,
	// and its size is the starting size until the first WindowSizeMsg.
	Caps caps.Caps
	// Theme names the palette; empty means DefaultThemeName. An unknown name
	// falls back to the default.
	Theme string

	Symlinks bool
	DryRun   bool
}

// step is one finished action: its result, and err when it failed.
type step struct {
	result install.Result
	err    error
}

// appliedMsg reports one finished action back into the update loop.
type appliedMsg struct {
	result install.Result
	err    error
}

// doneMsg marks the end of the run.
type doneMsg struct{}

// Model is the installer UI.
type Model struct {
	theme    Theme
	version  string
	platform string
	plan     *plan.Plan
	runner   *install.Runner

	Unavailable []UnavailableGroup
	Symlinks    bool
	DryRun      bool

	screen Screen
	cursor int
	// sel is the select screen's own state; see screen_select.go.
	sel selectState

	queue   []plan.Action
	index   int
	results []install.Result
	errs    []error
	// log is every finished action in run order, failures included, so the
	// install screen shows a failure where it happened.
	log []step

	// stopping is set by ctrl+c or q mid-install: the action in flight
	// finishes and the rest are skipped. stopped is the outcome, set on the
	// done screen only when actions were actually left.
	stopping, stopped bool

	// help is the `?` overlay; helpTop is its scroll cursor (see help.go).
	help    bool
	helpTop int

	width, height int
	// Quitting records a deliberate exit, so main can tell "the user pressed q"
	// apart from "the install finished".
	Quitting bool
}

// Fallback size until the terminal reports its own.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// NewModel builds the initial state.
func NewModel(p *plan.Plan, r *install.Runner, o Options) Model {
	name := o.Theme
	if name == "" {
		name = DefaultThemeName
	}
	th, err := NewTheme(name, o.Caps, nil)
	if err != nil {
		if th, err = NewTheme(DefaultThemeName, o.Caps, nil); err != nil {
			th = DefaultTheme()
		}
	}
	m := Model{
		theme:    th,
		version:  o.Version,
		platform: o.Platform,
		plan:     p,
		runner:   r,
		Symlinks: o.Symlinks,
		DryRun:   o.DryRun,
		screen:   ScreenSelect,
		width:    defaultWidth,
		height:   defaultHeight,
		sel:      newSelectState(p, r),
	}
	if o.Caps.Width > 0 {
		m.width = o.Caps.Width
	}
	if o.Caps.Height > 0 {
		m.height = o.Caps.Height
	}
	return m
}

// WithTheme returns m rendering with t. Tests use it to pin a deterministic
// theme; callers that know the terminal use it to apply their detected theme.
func (m Model) WithTheme(t Theme) Model {
	m.theme = t
	return m
}

// Screen exposes the current view, for tests and for main's exit code.
func (m Model) Screen() Screen { return m.screen }

// Results exposes what was applied.
func (m Model) Results() []install.Result { return m.results }

// Errs exposes failures. A non-empty slice means a non-zero exit.
func (m Model) Errs() []error { return m.errs }

func (m Model) Init() tea.Cmd { return nil }

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		// The key map is the only way in: a key it does not list does nothing,
		// so the hint line can never drift from what is handled.
		act, ok := m.keyMap().lookup(msg.String())
		if !ok {
			return m, nil
		}
		return m.perform(act)

	case appliedMsg:
		m = m.record(msg.result, msg.err)
		return m, m.applyNext()

	case doneMsg:
		m.screen = ScreenDone
		m.stopped = m.stopping && m.index < len(m.queue)
		return m, nil
	}
	return m, nil
}

// perform routes one bound action to the screen that owns it.
func (m Model) perform(act action) (tea.Model, tea.Cmd) {
	if m.help && act != actStop {
		return m.helpAction(act)
	}
	switch act {
	case actHelp:
		m.help, m.helpTop = true, 0
		return m, nil
	case actQuit:
		m.Quitting = true
		return m, tea.Quit
	case actStop:
		if m.screen == ScreenInstall {
			m.stopping = true
		}
		return m, nil
	}
	switch m.screen {
	case ScreenSelect:
		return m.selectAction(act)
	case ScreenDone:
		return m.doneAction(act)
	}
	return m, nil
}

// View renders the current screen inside the shared frame.
func (m Model) View() string {
	if m.help {
		return m.render(m.helpView())
	}
	switch m.screen {
	case ScreenInstall:
		return m.render(m.installView())
	case ScreenDone:
		return m.render(m.doneView())
	default:
		return m.render(m.selectView())
	}
}

// record notes one finished action and moves to the next.
func (m Model) record(res install.Result, err error) Model {
	if err != nil {
		m.errs = append(m.errs, err)
	} else {
		m.results = append(m.results, res)
	}
	m.log = append(m.log, step{result: res, err: err})
	m.index++
	return m
}
