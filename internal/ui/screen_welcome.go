package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/core/repoloc"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// The welcome screen is step 1 of a first run: `aval install` found no
// repository, so it asks where one is. Two choices (Hick, DESIGN.md section
// 10), three when the working directory sits inside a checkout. Cloning is
// always an explicit choice, never a default action. Once a repository is
// chosen, Prepare builds the plan and the same program moves on to select.
//
// Nothing here writes outside the clone destination: the repository is
// recorded in the config file by main, and only after a completed install.

// Cloner fetches the repository into dest. The real one runs git; tests pass
// a fake, so no test ever touches the network.
type Cloner interface {
	Clone(dest string) error
}

// Prepared is what the installer needs once a repository is chosen.
type Prepared struct {
	Plan        *plan.Plan
	Runner      *install.Runner
	Unavailable []UnavailableGroup
	Symlinks    bool
}

// WelcomeEnv is what the welcome screen needs from the outside world.
type WelcomeEnv struct {
	// Home expands a typed "~/" and shortens paths for display.
	Home string
	// CloneDest is where the clone choice puts the repository (~/.aval).
	CloneDest string
	// CloneURL is shown while cloning; Cloner does the work.
	CloneURL string
	Cloner   Cloner
	// Candidate is a checkout found from the working directory, offered as
	// the first choice; empty offers none.
	Candidate string
	// Prepare builds the plan for a chosen repository. Its error stays on
	// the welcome screen.
	Prepare func(repo string) (Prepared, error)
}

type welcomeChoice int

const (
	choiceHere welcomeChoice = iota
	choiceClone
	choicePath
)

type welcomeMode int

const (
	modeChoose  welcomeMode = iota
	modePath                // the inline path field is open
	modeCloning             // the Cloner is running
)

// welcomeState is the screen's own state.
type welcomeState struct {
	env    WelcomeEnv
	cursor int
	mode   welcomeMode
	input  []rune
	// err is the inline error: under the field in path mode, under the
	// choices otherwise.
	err string
	// cloneFailed turns the clone choice into a retry.
	cloneFailed bool
	frame       int
	// repo is the repository chosen, once Prepare accepted it.
	repo string
}

// The welcome screen's actions; negative like the other screens' own.
const (
	actChoose action = -20 // enter on a choice
	actBack   action = -21 // esc out of the path field
	actErase  action = -22 // backspace in the path field
	actCheck  action = -23 // enter in the path field
)

var welcomeKeys = keyMap{
	{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: i18n.Key("key.move")},
	{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: i18n.Key("key.move")},
	{keys: []string{"enter"}, action: actChoose, label: "enter", desc: i18n.Key("key.choose"), core: true},
	helpBinding,
	quitBinding,
}

// welcomePathKeys leave every printable key to the field, `?` and q
// included: they are valid in a path. ctrl+c still quits.
var welcomePathKeys = keyMap{
	{keys: []string{"enter"}, action: actCheck, label: "enter", desc: i18n.Key("key.use_folder"), core: true},
	{keys: []string{"esc"}, action: actBack, label: "esc", desc: i18n.Key("key.back"), core: true},
	{keys: []string{"backspace"}, action: actErase, desc: i18n.Key("key.delete_char")},
	{keys: []string{"ctrl+c"}, action: actQuit, label: "ctrl+c", desc: i18n.Key("key.quit")},
}

var welcomeCloningKeys = keyMap{helpBinding, quitBinding}

func (m Model) welcomeKeys() keyMap {
	switch m.welcome.mode {
	case modePath:
		return welcomePathKeys
	case modeCloning:
		return welcomeCloningKeys
	}
	return welcomeKeys
}

// spinFrames is the one spinner (DESIGN.md section 10: network waits only).
// ASCII in both tiers, so it never needs a fallback.
var spinFrames = []string{"|", "/", "-", "\\"}

const spinInterval = 120 * time.Millisecond

type spinTickMsg struct{}

// clonedMsg reports the Cloner's result back into the update loop.
type clonedMsg struct{ err error }

func spinTick() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// NewWelcome builds a model that opens on the welcome screen. o carries the
// look and the install mode; the plan comes later, from env.Prepare.
func NewWelcome(o Options, env WelcomeEnv) Model {
	m := NewModel(&plan.Plan{}, nil, o)
	m.screen = ScreenWelcome
	m.welcome = welcomeState{env: env}
	return m
}

// Repo is the repository the welcome screen chose; empty when it was not
// shown or nothing was chosen.
func (m Model) Repo() string { return m.welcome.repo }

func (s welcomeState) choices() []welcomeChoice {
	if s.env.Candidate != "" {
		return []welcomeChoice{choiceHere, choiceClone, choicePath}
	}
	return []welcomeChoice{choiceClone, choicePath}
}

func (s welcomeState) indexOf(c welcomeChoice) int {
	for i, x := range s.choices() {
		if x == c {
			return i
		}
	}
	return 0
}

// typeInto adds typed or pasted text to the open path field. It runs before
// the key map: printable keys are text there, not bindings.
func (m Model) typeInto(msg tea.KeyMsg) (Model, bool) {
	if m.help || m.screen != ScreenWelcome || m.welcome.mode != modePath {
		return m, false
	}
	switch msg.Type {
	case tea.KeyRunes:
		m.welcome.input = append(append([]rune{}, m.welcome.input...), msg.Runes...)
	case tea.KeySpace:
		m.welcome.input = append(append([]rune{}, m.welcome.input...), ' ')
	default:
		return m, false
	}
	m.welcome.err = ""
	return m, true
}

func (m Model) welcomeAction(act action) (tea.Model, tea.Cmd) {
	s := &m.welcome
	switch act {
	case actUp:
		s.cursor = max(s.cursor-1, 0)
	case actDown:
		s.cursor = min(s.cursor+1, len(s.choices())-1)
	case actChoose:
		s.err = ""
		switch s.choices()[s.cursor] {
		case choiceHere:
			return m.useRepo(s.env.Candidate), nil
		case choiceClone:
			return m.startClone()
		case choicePath:
			s.mode, s.input = modePath, nil
		}
	case actBack:
		s.mode, s.err = modeChoose, ""
	case actErase:
		if n := len(s.input); n > 0 {
			s.input = s.input[:n-1]
		}
		s.err = ""
	case actCheck:
		path := m.expandHome(strings.TrimSpace(string(s.input)))
		root, err := repoloc.Check(path)
		if err != nil {
			s.err = m.shortenPaths(err.Error(), path)
			return m, nil
		}
		return m.useRepo(root), nil
	}
	return m, nil
}

// useRepo hands the chosen repository to Prepare and moves on to select.
func (m Model) useRepo(root string) Model {
	s := &m.welcome
	if s.env.Prepare == nil {
		s.err = m.tr.T("welcome.no_prepare", i18n.Args{"path": m.homeTilde(root)})
		return m
	}
	p, err := s.env.Prepare(root)
	if err != nil {
		s.err = err.Error()
		return m
	}
	s.mode, s.err, s.repo = modeChoose, "", root
	m.plan, m.runner = p.Plan, p.Runner
	if m.plan == nil {
		m.plan = &plan.Plan{}
	}
	m.Unavailable, m.Symlinks = p.Unavailable, p.Symlinks
	m.sel = newSelectState(m.plan, m.runner)
	m.cursor = 0
	m.screen = ScreenSelect
	return m
}

// startClone uses a checkout already at the destination, refuses anything
// else there, and otherwise runs the Cloner.
func (m Model) startClone() (tea.Model, tea.Cmd) {
	s := &m.welcome
	dest := s.env.CloneDest
	shown := m.homeTilde(dest)
	if fi, err := os.Stat(dest); err == nil {
		if fi.IsDir() && repoloc.Find(dest) == dest {
			return m.useRepo(dest), nil
		}
		s.err = m.tr.T("welcome.exists", i18n.Args{"path": shown, "manifest": repoloc.Manifest})
		return m, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		s.err = m.shortenPaths(err.Error(), dest)
		return m, nil
	}
	if s.env.Cloner == nil {
		s.err = m.tr.T("welcome.no_clone", nil)
		return m, nil
	}
	s.mode, s.frame = modeCloning, 0
	cloner := s.env.Cloner
	clone := func() tea.Msg { return clonedMsg{err: cloner.Clone(dest)} }
	if m.caps.Motion {
		return m, tea.Batch(clone, spinTick())
	}
	return m, clone
}

// cloned handles the Cloner's result.
func (m Model) cloned(err error) Model {
	s := &m.welcome
	s.mode = modeChoose
	if err != nil {
		s.cloneFailed = true
		s.cursor = s.indexOf(choiceClone)
		s.err = m.tr.T("welcome.clone_failed", i18n.Args{"error": strings.TrimSpace(err.Error())})
		return m
	}
	root, cerr := repoloc.Check(s.env.CloneDest)
	if cerr != nil {
		s.cloneFailed = true
		s.err = m.shortenPaths(cerr.Error(), s.env.CloneDest)
		return m
	}
	s.cloneFailed = false
	return m.useRepo(root)
}

// spun advances the spinner while a clone is in flight with motion on.
func (m Model) spun() (Model, tea.Cmd) {
	if m.screen != ScreenWelcome || m.welcome.mode != modeCloning || !m.caps.Motion {
		return m, nil
	}
	m.welcome.frame = (m.welcome.frame + 1) % len(spinFrames)
	return m, spinTick()
}

// expandHome turns a typed "~" or "~/..." into a path under Home.
func (m Model) expandHome(p string) string {
	home := m.welcome.env.Home
	switch {
	case home == "":
		return p
	case p == "~":
		return home
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home, p[2:])
	}
	return p
}

func (m Model) homeTilde(p string) string { return tildeUnder(m.welcome.env.Home, p) }

// shortenPaths shows path as ~/... inside an error message.
func (m Model) shortenPaths(msg, path string) string {
	if path == "" {
		return msg
	}
	return strings.ReplaceAll(msg, path, m.homeTilde(path))
}

func (m Model) welcomeView() view {
	return view{body: m.welcomeBody, status: m.tr.T("welcome.step", i18n.Args{"sep": m.sep()})}
}

func (m Model) welcomeBody(l layout, h int) []string {
	s := m.welcome
	var intro []string
	for _, p := range []string{m.tr.T("welcome.intro1", nil), m.tr.T("welcome.intro2", nil)} {
		for _, line := range m.wrap(p, l.width) {
			intro = append(intro, m.theme.Text.Render(line))
		}
	}
	ask := m.theme.Strong.Render(m.fit(m.tr.T("welcome.ask", nil), l.width))
	block, cur := m.welcomeChoiceLines(l)
	if s.mode == modeCloning {
		block, cur = m.cloningLines(l), 0
	}

	build := func(gaps, withIntro bool) []string {
		var out []string
		if withIntro {
			out = append(out, intro...)
			if gaps {
				out = append(out, "")
			}
		}
		out = append(out, ask)
		if gaps {
			out = append(out, "")
		}
		return append(out, block...)
	}
	for _, try := range [][2]bool{{true, true}, {false, true}, {false, false}} {
		if out := build(try[0], try[1]); len(out) <= h {
			return out
		}
	}
	// Shorter still: the choices scroll with the cursor.
	start, end := scrollWindow(len(block), cur, h)
	return block[start:end]
}

// welcomeChoiceLines renders the choices, the open path field and the inline
// error, and returns the cursor row's index.
func (m Model) welcomeChoiceLines(l layout) ([]string, int) {
	s := m.welcome
	type row struct{ label, note string }
	var rows []row
	for _, c := range s.choices() {
		switch c {
		case choiceHere:
			rows = append(rows, row{m.tr.T("welcome.here", nil), m.homeTilde(s.env.Candidate)})
		case choiceClone:
			label := m.tr.T("welcome.clone", i18n.Args{"path": m.homeTilde(s.env.CloneDest)})
			if s.cloneFailed {
				label = m.tr.T("welcome.clone_again", nil)
			}
			rows = append(rows, row{label, m.tr.T("welcome.clone_note", nil)})
		case choicePath:
			rows = append(rows, row{m.tr.T("welcome.path", nil), m.tr.T("welcome.path_note", nil)})
		}
	}
	const radio = "( ) "
	lead := 2 + len(radio) // marker plus radio
	labelW := 0
	for _, r := range rows {
		labelW = max(labelW, lipgloss.Width(r.label))
	}
	labelW = min(labelW, max(l.width-lead, 1))
	indent := strings.Repeat(" ", min(lead, max(l.width-1, 0)))

	var lines []string
	cur := 0
	for i, r := range rows {
		marker := "  "
		cell := m.theme.Text.Render(radio + m.pad(r.label, labelW))
		if i == s.cursor {
			marker = m.theme.Marker.Render(m.theme.Glyphs.Cursor + " ")
			cell = m.theme.Selected.Render(radio + m.pad(r.label, labelW))
			cur = len(lines)
		}
		line := marker + cell
		noteW := l.width - lead - labelW - 2
		switch {
		case lipgloss.Width(r.note) <= noteW:
			lines = append(lines, line+"  "+m.theme.Muted.Render(r.note))
		default:
			lines = append(lines, line)
			for _, nl := range m.wrap(r.note, max(l.width-len(indent), 1)) {
				lines = append(lines, indent+m.theme.Muted.Render(nl))
			}
		}
		if s.mode == modePath && s.choices()[i] == choicePath {
			lines = append(lines, m.pathField(l, indent))
		}
	}
	if s.err != "" {
		lines = append(lines, "")
		lines = append(lines, m.errorLines(l, s.err)...)
	}
	return lines, cur
}

// pathField is the inline text field: the typed path, its tail kept when it
// is too long, and a reverse-video cursor cell.
func (m Model) pathField(l layout, indent string) string {
	room := max(l.width-len(indent)-1, 1)
	text := string(m.welcome.input)
	if lipgloss.Width(text) > room {
		ell := m.theme.Glyphs.Ellipsis
		r := []rune(text)
		keep := max(room-lipgloss.Width(ell), 0)
		for len(r) > 0 && lipgloss.Width(string(r)) > keep {
			r = r[1:]
		}
		text = ell + string(r)
		if lipgloss.Width(text) > room {
			text = m.fitRunes(text, room)
		}
	}
	return indent + m.theme.Text.Render(text) + m.theme.Selected.Render(" ")
}

// errorLines is the fail glyph, then the message wrapped under it.
func (m Model) errorLines(l layout, msg string) []string {
	g := m.theme.Glyphs.Fail
	gw := lipgloss.Width(g) + 1
	wrapped := m.wrap(msg, max(l.width-gw, 1))
	out := make([]string, len(wrapped))
	for i, line := range wrapped {
		if i == 0 {
			out[i] = m.theme.State(StateFail, line)
			continue
		}
		out[i] = strings.Repeat(" ", gw) + m.theme.Text.Render(line)
	}
	return out
}

// cloningLines is the one spinner, or static text with motion off, and the
// command it runs.
func (m Model) cloningLines(l layout) []string {
	s := m.welcome
	text := m.tr.T("welcome.cloning", nil) + m.theme.Glyphs.Ellipsis
	if m.caps.Motion {
		text = spinFrames[s.frame%len(spinFrames)] + " " + text
	}
	lines := []string{m.theme.Text.Render(m.fit(text, l.width))}
	if s.env.CloneURL != "" {
		cmd := "git clone " + s.env.CloneURL + " " + m.homeTilde(s.env.CloneDest)
		lines = append(lines, m.theme.Muted.Render(m.fitMiddle(cmd, l.width)))
	}
	return lines
}
