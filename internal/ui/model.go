package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// This file holds routing and state only. The shared chrome is in frame.go,
// the bindings in keys.go, and each view in its own screen_*.go file.

// Screen is which of the five views is showing.
type Screen int

const (
	ScreenSelect   Screen = iota // choose what to install
	ScreenInstall                // work in progress
	ScreenDone                   // summary
	ScreenSettings               // the config file, edited live
	ScreenWelcome                // find or clone the repository
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

	// Settings is where the settings screen reads and writes. An empty Path
	// leaves the `s` binding out of the installer.
	Settings SettingsEnv
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
	// chrome is the theme, the lockup labels and the terminal size.
	chrome

	plan   *plan.Plan
	runner *install.Runner

	Unavailable []UnavailableGroup
	Symlinks    bool
	DryRun      bool

	screen Screen
	cursor int
	// sel is the select screen's own state; see screen_select.go.
	sel selectState
	// settings is the settings screen's state; settingsEnv where it reads
	// and writes (see screen_settings.go).
	settings    settingsState
	settingsEnv SettingsEnv
	// welcome is the repository step; see screen_welcome.go.
	welcome welcomeState
	// caps is the detected terminal, the base for the settings preview.
	caps caps.Caps

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

	// Quitting records a deliberate exit, so main can tell "the user pressed q"
	// apart from "the install finished".
	Quitting bool

	// hosted is set by the flow screen: the Model runs inside the router, so
	// leaving it returns to the hub instead of ending the program.
	hosted bool

	// runTitle and doneLead retitle a re-apply (see reapplying): the progress
	// heading and the start of the done headline. Empty is the installer's.
	runTitle, doneLead string
}

// Fallback size until the terminal reports its own.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// newChrome builds the frame from the options: the named theme (the default
// when empty or unknown) and the detected size, or 80x24 until the terminal
// reports its own.
func newChrome(o Options) chrome {
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
	c := chrome{theme: th, version: o.Version, platform: o.Platform, width: defaultWidth, height: defaultHeight}
	if o.Caps.Width > 0 {
		c.width = o.Caps.Width
	}
	if o.Caps.Height > 0 {
		c.height = o.Caps.Height
	}
	return c
}

// NewModel builds the initial state.
func NewModel(p *plan.Plan, r *install.Runner, o Options) Model {
	return Model{
		chrome:   newChrome(o),
		plan:     p,
		runner:   r,
		Symlinks: o.Symlinks,
		DryRun:   o.DryRun,
		screen:   ScreenSelect,
		sel:      newSelectState(p, r),

		settingsEnv: o.Settings,
		caps:        o.Caps,
	}
}

// WithTheme returns m rendering with t. Tests use it to pin a deterministic
// theme; callers that know the terminal use it to apply their detected theme.
func (m Model) WithTheme(t Theme) Model {
	m.theme = t
	if m.screen == ScreenSettings {
		m.settings.preview = m.candidateTheme()
	}
	return m
}

// Screen exposes the current view, for tests and for main's exit code.
func (m Model) Screen() Screen { return m.screen }

// Results exposes what was applied.
func (m Model) Results() []install.Result { return m.results }

// Errs exposes failures. A non-empty slice means a non-zero exit.
func (m Model) Errs() []error { return m.errs }

// Init starts the run when the Model opens on the progress screen (a
// re-apply); the installer opens on select or welcome and waits for keys.
func (m Model) Init() tea.Cmd {
	if m.screen == ScreenInstall {
		return m.applyNext()
	}
	return nil
}

// reapplying turns m into a re-apply of its default selection, as `aval
// update` does after a pull: it opens on the progress screen titled
// "Re-applying" and its done headline starts with lead ("Updated a → b.").
func (m Model) reapplying(lead string) Model {
	m.runTitle, m.doneLead = "Re-applying", lead
	m.queue = m.plan.SelectedActions()
	m.screen, m.index = ScreenInstall, 0
	return m
}

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		// An open text field takes printable keys as text before any binding.
		if next, ok := m.typeInto(msg); ok {
			return next, nil
		}
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

	case clonedMsg:
		return m.cloned(msg.err), nil

	case spinTickMsg:
		return m.spun()

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
		return m, m.exit()
	case actStop:
		if m.screen == ScreenInstall {
			m.stopping = true
		}
		return m, nil
	case actSettings:
		return m.openSettings(), nil
	}
	switch m.screen {
	case ScreenSelect:
		return m.selectAction(act)
	case ScreenDone:
		return m.doneAction(act)
	case ScreenSettings:
		return m.settingsAction(act)
	case ScreenWelcome:
		return m.welcomeAction(act)
	}
	return m, nil
}

// exit is the one way out of the Model: standalone it ends the program, as
// it always has; hosted by the router (see flow.go) it pops back to the hub.
func (m Model) exit() tea.Cmd {
	if m.hosted {
		return pop()
	}
	return tea.Quit
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
	case ScreenSettings:
		return m.render(m.settingsView())
	case ScreenWelcome:
		return m.render(m.welcomeView())
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
