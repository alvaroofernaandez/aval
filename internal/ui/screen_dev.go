package ui

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/devmode"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// The dev sandbox view (DESIGN.md section 10, plan H18) lists the throwaway
// homes of `aval dev`. Enter, new and reset hand the terminal to
// `aval dev [reset] --name <n>` through tea.ExecProcess and the view is back
// when the shell exits; remove calls RemoveSandbox. Reset and remove go
// through a destructive confirm that says there is no backup. Inside a
// sandbox the list is read-only and every writing key leaves the key map.

func init() {
	registerView(viewDev, func(svc Services) screen { return newDev(svc) })
}

const (
	actDevOpen   action = -140 // enter: a shell in the selected sandbox
	actDevNew    action = -141
	actDevReset  action = -142
	actDevRemove action = -143
	actDevSubmit action = -144 // enter in the name field
	actDevCancel action = -145 // esc in the name field
)

type devMode int

const (
	devList     devMode = iota
	devNew              // the inline name field is open
	devConfirm          // reset or remove awaits its confirm
	devRemoving         // RemoveSandbox is running; keys are inert
)

const (
	// devNameMax caps what the field accepts, well past ValidName's 40, so
	// the validator can still explain an over-long name.
	devNameMax = 64
	// devStderrLines is how many lines of the failed command's stderr show.
	devStderrLines = 3
)

// devAct is the destructive action a confirm asks about.
type devAct int

const (
	devReset devAct = iota
	devRemove
)

type (
	// devExitMsg is how the sandbox shell ended, with the tail of its stderr.
	devExitMsg struct {
		err    error
		stderr []string
	}
	// devRemovedMsg is the result of RemoveSandbox.
	devRemovedMsg struct {
		name string
		err  error
	}
)

// devFail is what the last action reported, shown under the list.
type devFail struct {
	title string
	lines []string
}

type dev struct {
	svc Services
	c   chrome

	infos   []devmode.Info
	listErr error
	cursor  int
	base    string

	mode devMode
	// field is the name being typed, fieldErr the validator's message.
	field, fieldErr string
	// conf is the confirm for act (reset or remove) on target.
	conf   confirm
	act    devAct
	target string

	fail *devFail
	// job is the generation of the shell or removal in flight.
	job ticket
}

func newDev(svc Services) dev {
	s := dev{svc: svc}
	s.reload()
	s.base = devmode.BaseDir(os.Getenv, svc.Home)
	if len(s.infos) > 0 {
		s.base = filepath.Dir(s.infos[0].Root)
	}
	return s
}

// reload reads the sandboxes again and keeps the cursor on a row.
func (s *dev) reload() {
	s.infos, s.listErr = nil, nil
	if s.svc.Sandboxes != nil {
		s.infos, s.listErr = s.svc.Sandboxes()
	}
	s.cursor = max(min(s.cursor, len(s.infos)-1), 0)
}

func (s dev) init() tea.Cmd { return nil }

func (s dev) sandboxed() bool { return s.svc.Sandbox != "" }

// canExec: aval dev can be run from here. Nil DevCommand means it cannot.
func (s dev) canExec() bool { return !s.sandboxed() && s.svc.DevCommand != nil }

func (s dev) canRemove() bool { return !s.sandboxed() && s.svc.RemoveSandbox != nil }

func (s dev) selected() (devmode.Info, bool) {
	if s.cursor < 0 || s.cursor >= len(s.infos) {
		return devmode.Info{}, false
	}
	return s.infos[s.cursor], true
}

func (s dev) typing() bool { return s.mode == devNew }

func (s dev) busy() busyKind {
	if s.mode == devRemoving {
		return busyAtomic
	}
	return busyIdle
}

var (
	devMoveKeys = keyMap{
		{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: i18n.Key("key.move")},
		{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: i18n.Key("key.move")},
	}
	devOpenKey   = binding{keys: []string{"enter"}, action: actDevOpen, label: "enter", desc: i18n.Key("key.open_shell"), core: true}
	devNewKey    = binding{keys: []string{"n"}, action: actDevNew, label: "n", desc: i18n.Key("key.new")}
	devResetKey  = binding{keys: []string{"r"}, action: actDevReset, label: "r", desc: i18n.Key("key.reset")}
	devRemoveKey = binding{keys: []string{"d"}, action: actDevRemove, label: "d", desc: i18n.Key("key.remove")}
	devFieldKeys = keyMap{
		{keys: []string{"enter"}, action: actDevSubmit, label: "enter", desc: i18n.Key("key.create"), core: true},
		{keys: []string{"esc"}, action: actDevCancel, label: "esc", desc: i18n.Key("key.cancel"), core: true},
	}
)

// keys lists only what works now: no row means no open, reset or remove;
// inside a sandbox nothing writes, so only the cursor moves.
func (s dev) keys() keyMap {
	switch s.mode {
	case devNew:
		return devFieldKeys
	case devConfirm:
		return s.conf.keys()
	case devRemoving:
		return nil
	}
	km := keyMap{}
	if len(s.infos) > 1 {
		km = append(km, devMoveKeys...)
	}
	if !s.canExec() && !s.canRemove() {
		return km
	}
	_, row := s.selected()
	if s.canExec() && row {
		km = append(km, devOpenKey)
	}
	if s.canExec() {
		km = append(km, devNewKey)
	}
	if s.canExec() && row {
		km = append(km, devResetKey)
	}
	if s.canRemove() && row {
		km = append(km, devRemoveKey)
	}
	return km
}

func (s dev) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		s.c = msg.c
		return s, nil
	case resumeMsg:
		s.reload()
		return s, nil
	case tea.KeyMsg:
		return s.key(msg)
	}
	inner, ok := s.job.accept(msg)
	if !ok {
		return s, nil
	}
	s.job.void()
	switch r := inner.(type) {
	case devExitMsg:
		s.reload()
		s.fail = nil
		if r.err != nil {
			s.fail = &devFail{title: s.c.tr.T("dev.failed", nil), lines: r.stderr}
			if len(r.stderr) == 0 {
				s.fail.lines = []string{r.err.Error()}
			}
		}
	case devRemovedMsg:
		s.mode = devList
		s.reload()
		s.fail = nil
		if r.err != nil {
			s.fail = &devFail{title: s.c.tr.T("dev.remove_failed", i18n.Args{"name": r.name}), lines: []string{r.err.Error()}}
		}
	}
	return s, nil
}

func (s dev) key(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch s.mode {
	case devNew:
		return s.fieldKey(msg)
	case devConfirm:
		return s.confirmKey(msg)
	case devRemoving:
		return s, nil
	}
	act, ok := s.keys().lookup(msg.String())
	if !ok {
		return s, nil
	}
	s.fail = nil
	info, _ := s.selected()
	switch act {
	case actUp:
		s.cursor = (s.cursor + len(s.infos) - 1) % len(s.infos)
	case actDown:
		s.cursor = (s.cursor + 1) % len(s.infos)
	case actDevOpen:
		return s.exec("", info.Name)
	case actDevNew:
		s.mode, s.field, s.fieldErr = devNew, "", ""
	case actDevReset:
		return s.ask(devReset, info.Name)
	case actDevRemove:
		return s.ask(devRemove, info.Name)
	}
	return s, nil
}

// ask opens the destructive confirm, on Cancel.
func (s dev) ask(act devAct, name string) (screen, tea.Cmd) {
	tr := s.c.tr
	// A sandbox has no backup, and the safety line says why that is fine.
	spec := confirmSpec{Safety: tr.T("dev.safety", nil)}
	args := i18n.Args{"name": name}
	switch act {
	case devReset:
		spec.Action, spec.ActionNote = tr.T("dev.reset.action", args), tr.T("dev.reset.note", nil)
		spec.Prompt = tr.T("dev.reset.prompt", args)
	default:
		spec.Action, spec.ActionNote = tr.T("dev.remove.action", args), tr.T("dev.remove.note", nil)
		spec.Prompt = tr.T("dev.remove.prompt", args)
	}
	conf, err := newConfirm(confirmDestructive, spec)
	if err != nil {
		panic(err) // the spec above always has a safety line and an action
	}
	s.mode, s.conf, s.act, s.target = devConfirm, conf, act, name
	return s, nil
}

func (s dev) confirmKey(msg tea.KeyMsg) (screen, tea.Cmd) {
	var ch confirmChoice
	s.conf, ch = s.conf.update(msg)
	switch ch {
	case choiceCancel:
		s.mode = devList
	case choiceAction:
		s.mode = devList
		if s.act == devReset {
			return s.exec("reset", s.target)
		}
		s.mode = devRemoving
		rm, name := s.svc.RemoveSandbox, s.target
		return s, s.job.issue(func() tea.Msg { return devRemovedMsg{name: name, err: rm(name)} })
	}
	return s, nil
}

// fieldKey edits the name. Every key but ctrl+c lands here while the field is
// open (the router's textField contract), so q and ? are typed.
func (s dev) fieldKey(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		s.mode, s.field, s.fieldErr = devList, "", ""
	case tea.KeyEnter:
		if err := devmode.ValidName(s.field); err != nil {
			s.fieldErr = err.Error()
			return s, nil
		}
		name := s.field
		s.mode, s.field, s.fieldErr = devList, "", ""
		return s.exec("", name)
	case tea.KeyBackspace:
		if r := []rune(s.field); len(r) > 0 {
			s.field = string(r[:len(r)-1])
		}
		s.fieldErr = ""
	case tea.KeyRunes, tea.KeySpace:
		if len([]rune(s.field)) < devNameMax {
			s.field += string(msg.Runes)
			if msg.Type == tea.KeySpace {
				s.field += " "
			}
		}
		s.fieldErr = ""
	}
	return s, nil
}

// exec hands the terminal to `aval dev [sub] --name <name>`. The command's
// stderr goes to the terminal as usual and into a small tail, so a failure
// can show its last lines. bubbletea 1.3.10 only fills a nil Stderr
// (osExecCommand.SetStderr), so the writer set here survives ExecProcess.
func (s dev) exec(sub, name string) (screen, tea.Cmd) {
	cmd := s.svc.DevCommand(sub, name)
	if cmd == nil {
		s.fail = &devFail{title: s.c.tr.T("dev.failed", nil), lines: []string{s.c.tr.T("dev.unavailable", nil)}}
		return s, nil
	}
	tail := &tailWriter{}
	if cmd.Stderr == nil {
		cmd.Stderr = io.MultiWriter(os.Stderr, tail)
	}
	s.job.gen = lastGen.Add(1)
	gen := s.job.gen
	return s, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return genMsg{gen: gen, msg: devExitMsg{err: err, stderr: tail.lines(devStderrLines)}}
	})
}

// tailWriter keeps the last few KiB written to it.
type tailWriter struct {
	mu  sync.Mutex
	buf []byte
}

const tailMax = 4096

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) > tailMax {
		w.buf = w.buf[len(w.buf)-tailMax:]
	}
	return len(p), nil
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// lines is the last n non-empty lines, stripped of escapes and controls.
func (w *tailWriter) lines(n int) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, l := range strings.Split(string(w.buf), "\n") {
		l = ansiSeq.ReplaceAllString(l, "")
		l = strings.Map(func(r rune) rune {
			if r < ' ' && r != '\t' || r == 0x7f {
				return -1
			}
			return r
		}, l)
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

func (s dev) status() string {
	switch s.mode {
	case devNew:
		return s.c.tr.T("dev.status.new", nil)
	case devConfirm:
		return s.conf.status()
	case devRemoving:
		return s.c.tr.T("dev.status.removing", nil)
	}
	switch {
	case s.sandboxed():
		return s.c.tr.T("dev.status.readonly", nil)
	case len(s.infos) == 0:
		return s.c.tr.T("dev.status.empty", nil)
	}
	return s.c.tr.T("dev.status.list", nil)
}

func (s dev) view() view {
	return view{body: s.body, status: s.status()}
}

func (s dev) heading(l layout) []string {
	t := s.c.theme
	return s.c.headingRow(l, s.c.tr.T("hub.dev.name", nil), t.Heading, tildeUnder(s.svc.Home, s.base), t.Muted)
}

func (s dev) clip(lines []string, h int) []string {
	if len(lines) > h {
		return lines[:max(h, 0)]
	}
	return lines
}

func (s dev) body(l layout, h int) []string {
	t := s.c.theme
	head := s.heading(l)
	switch s.mode {
	case devConfirm:
		return s.clip(append(append(head, ""), s.conf.lines(s.c, l)...), h)
	case devRemoving:
		return s.clip(append(head, "", t.Text.Render(s.c.fit(s.c.tr.T("dev.removing", i18n.Args{"name": s.target})+t.Glyphs.Ellipsis, l.width))), h)
	case devNew:
		return s.clip(s.fieldLines(l, head), h)
	}

	intro := s.c.tr.T("dev.intro", nil)
	if s.sandboxed() {
		intro = s.c.tr.T("dev.intro_inside", i18n.Args{"name": s.svc.Sandbox})
	}
	var top []string
	for _, w := range s.c.wrap(intro, l.width) {
		top = append(top, t.Text.Render(w))
	}
	lead := append(append(append([]string(nil), head...), ""), top...)
	lead = append(lead, "")
	foot := s.failLines(l)
	rows := s.rows(l)
	if h-len(lead)-len(foot) < 1 {
		lead = append(append([]string(nil), head...), "")
	}
	start, end := scrollWindow(len(rows), s.cursor, max(h-len(lead)-len(foot), 1))
	out := append(lead, rows[start:end]...)
	return s.clip(append(out, foot...), h)
}

// fieldLines is the inline name field and, under it, the validator's message.
func (s dev) fieldLines(l layout, head []string) []string {
	t := s.c.theme
	label := s.c.tr.T("dev.name_label", nil)
	value := s.c.fitMiddle(s.field, max(l.width-len(label)-1, 1))
	out := append(append(head, ""), t.Text.Render(s.c.tr.T("dev.name_prompt", nil)), "")
	out = append(out, t.Muted.Render(label)+t.Strong.Render(value)+t.Marker.Render("_"))
	if s.fieldErr == "" {
		return out
	}
	out = append(out, "")
	for i, w := range s.c.wrap(s.fieldErr, max(l.width-2, 1)) {
		if i == 0 {
			out = append(out, t.State(StateFail, w))
			continue
		}
		out = append(out, "  "+t.Text.Render(w))
	}
	return out
}

// failLines is the last failure: a fail glyph and title, then its lines.
func (s dev) failLines(l layout) []string {
	if s.fail == nil {
		return nil
	}
	t := s.c.theme
	out := []string{"", t.State(StateFail, s.c.fit(s.fail.title, max(l.width-lipgloss.Width(t.Glyphs.Fail)-1, 1)))}
	for _, line := range s.fail.lines {
		out = append(out, "  "+t.Muted.Render(s.c.fit(line, max(l.width-2, 1))))
	}
	return out
}

// rows is one line per sandbox: name, state and, when there is room, path.
func (s dev) rows(l layout) []string {
	t := s.c.theme
	if s.listErr != nil {
		return []string{t.State(StateFail, s.c.fit(s.c.tr.T("dev.list_error", nil)+s.listErr.Error(), max(l.width-2, 1)))}
	}
	if len(s.infos) == 0 {
		msg := s.c.tr.T("dev.none", nil)
		if s.sandboxed() {
			msg = s.c.tr.T("dev.none_inside", nil)
		}
		return []string{t.Muted.Render(s.c.fit(msg, l.width))}
	}
	g := t.Glyphs
	const gap = 2
	ready, incomplete := s.c.tr.T("dev.state.ready", nil), s.c.tr.T("dev.state.incomplete", nil)
	stateW := max(lipgloss.Width(g.OK), lipgloss.Width(g.Warn)) + 1 + max(lipgloss.Width(ready), lipgloss.Width(incomplete))
	room := max(l.width-hubCursorW, 1)
	// Below ~22 columns the word gives way to the glyph alone.
	glyphOnly := room < 4+gap+stateW
	if glyphOnly {
		stateW = max(lipgloss.Width(g.OK), lipgloss.Width(g.Warn))
	}
	nameW := 0
	for _, i := range s.infos {
		nameW = max(nameW, lipgloss.Width(i.Name))
	}
	nameW = max(min(nameW, 20, room-stateW-gap), 4)
	pathW := room - nameW - gap - stateW - gap
	out := make([]string, len(s.infos))
	for n, i := range s.infos {
		kind, glyph, word := StateOK, g.OK, ready
		if !i.Ready {
			kind, glyph, word = StateWarn, g.Warn, incomplete
		}
		if glyphOnly {
			word = ""
		}
		name := s.c.pad(i.Name, nameW)
		path := ""
		if pathW >= 12 {
			path = s.c.fitMiddle(tildeUnder(s.svc.Home, i.Root), pathW)
		}
		if n == s.cursor {
			plain := s.c.pad(name+strings.Repeat(" ", gap)+strings.TrimRight(glyph+" "+word, " "), nameW+gap+stateW)
			if path != "" {
				plain += strings.Repeat(" ", gap) + path
			}
			out[n] = t.Marker.Render(s.c.pad(g.Cursor, hubCursorW)) + t.Selected.Render(plain)
			continue
		}
		cell := t.State(kind, word)
		if glyphOnly {
			cell = strings.TrimRight(cell, " ")
		}
		cell += strings.Repeat(" ", max(stateW-lipgloss.Width(cell), 0))
		line := strings.Repeat(" ", hubCursorW) + t.Text.Render(name) + strings.Repeat(" ", gap) + cell
		if path != "" {
			line += strings.Repeat(" ", gap) + t.Muted.Render(path)
		}
		out[n] = line
	}
	return out
}
