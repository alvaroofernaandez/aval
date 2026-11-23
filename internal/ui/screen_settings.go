package ui

import (
	"maps"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// The settings screen edits the config file. The rows hold the file values,
// never the effective ones: a key overridden by the environment or a flag says
// so in the status line, and editing it still edits the file. A preview line
// repaints live in the candidate theme and glyphs. Enter writes the changed
// keys through config.Set, esc discards, q quits.
//
// It is reached two ways: bare `aval config` on a terminal (NewSettings, where
// enter and esc end the program) and `s` on the installer's select and done
// screens, where enter and esc go back to the screen it came from. Hosted by
// the hub (flow.go), enter and esc return to the hub and a save hands the
// router the new theme.

// SettingsEnv is what the settings screen reads and writes. An empty Path
// leaves the screen out of the installer.
type SettingsEnv struct {
	// Path is the config file.
	Path string
	// Home shortens Path and the repository for display.
	Home string
	// Getenv reads the environment overrides; nil means none.
	Getenv func(string) string
	// Flags are command-line values per key (flag > env > file), and
	// FlagNames the flag that set each, for the note ("--theme").
	Flags, FlagNames map[string]string
	// Detect returns the terminal's capabilities for candidate glyphs and
	// background values. Nil derives them from the model's own.
	Detect func(glyphs, background string) caps.Caps
}

func (e SettingsEnv) getenv(k string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(k)
}

// The settings screen's actions; negative like the select screen's, and
// apart from them.
const (
	actPrev     action = -10 // previous value
	actNext     action = -11 // next value
	actSave     action = -12
	actDiscard  action = -13
	actSettings action = -14 // open the settings screen
)

var settingsKeys = keyMap{
	{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: i18n.Key("key.move")},
	{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: i18n.Key("key.move")},
	{keys: []string{"left", "h"}, action: actPrev, label: labelLeftRight, desc: i18n.Key("key.change")},
	{keys: []string{"right", "l"}, action: actNext, label: labelLeftRight, desc: i18n.Key("key.change")},
	{keys: []string{"enter"}, action: actSave, label: "enter", desc: i18n.Key("key.save"), core: true},
	{keys: []string{"esc"}, action: actDiscard, label: "esc", desc: i18n.Key("key.discard"), core: true},
	helpBinding,
	quitBinding,
}

// settingsBinding opens the screen from the installer.
var settingsBinding = binding{keys: []string{"s"}, action: actSettings, label: "s", desc: i18n.Key("key.settings")}

// settingRow is one row; nil choices make it read-only. label is a message
// key, and the choices are the config file's own values, never translated.
type settingRow struct {
	key, label string
	choices    []string
}

// settingRows is seven rows at most (Hick, DESIGN.md section 10). The
// editable ones come first, so the cursor never lands on the read-only one.
var settingRows = []settingRow{
	{"theme", i18n.Key("settings.label.theme"), themeCycle},
	{"background", i18n.Key("settings.label.background"), []string{"auto", "dark", "light"}},
	{"glyphs", i18n.Key("settings.label.glyphs"), []string{"auto", "unicode", "ascii"}},
	{"motion", i18n.Key("settings.label.motion"), []string{"auto", "off"}},
	{"update_check", i18n.Key("settings.label.update_check"), []string{"on", "off"}},
	{"language", i18n.Key("settings.label.language"), []string{"auto", "en", "es"}},
	{"repo", i18n.Key("settings.label.repo"), nil},
}

func editableRows() int {
	n := 0
	for _, r := range settingRows {
		if r.choices != nil {
			n++
		}
	}
	return n
}

// settingsState is the screen's own state.
type settingsState struct {
	// back is the screen to return to; standalone ends the program instead.
	back       Screen
	standalone bool
	cursor     int
	// values are the candidate file values; initial what the file held.
	values, initial map[string]string
	// sources is where each effective value comes from: flag, env, file:N
	// or default.
	sources map[string]string
	// repo is the effective repository, for the read-only row.
	repo string
	// preview is the candidate theme the preview line renders in.
	preview Theme
	// baseTr is the language the screen opened in: esc restores it, and a
	// language set by a flag or the environment keeps it.
	baseTr i18n.Translator
	err    string
}

// NewSettings builds a model that opens on the settings screen and ends the
// program on enter, esc or q. It is bare `aval config` on a terminal.
func NewSettings(o Options) Model {
	m := NewModel(&plan.Plan{}, nil, o).openSettings()
	m.settings.standalone = true
	return m
}

// openSettings loads the file and shows the screen; it returns to the
// current screen.
func (m Model) openSettings() Model {
	env := m.settingsEnv
	st := settingsState{
		back:    m.screen,
		values:  map[string]string{},
		initial: map[string]string{},
		sources: map[string]string{},
	}
	f, _, err := config.Load(env.Path)
	if err != nil {
		st.err = err.Error()
		f, _, _ = config.Load("")
	}
	none := func(string) string { return "" }
	for _, r := range settingRows {
		v, _ := f.Resolve(r.key, nil, none)
		st.values[r.key], st.initial[r.key] = v, v
		_, src := f.Resolve(r.key, env.Flags, env.getenv)
		st.sources[r.key] = src.String()
	}
	st.repo, _ = f.Resolve("repo", env.Flags, env.getenv)
	st.baseTr = m.tr
	m.settings = st
	m.screen = ScreenSettings
	m.settings.preview = m.candidateTheme()
	return m
}

// overridden reports whether key's effective value is not the file's.
func (s settingsState) overridden(key string) bool {
	src := s.sources[key]
	return src == "env" || src == "flag"
}

// candidateCaps is the terminal as it would be with these glyph and
// background values.
func (m Model) candidateCaps(glyphs, background string) caps.Caps {
	if d := m.settingsEnv.Detect; d != nil {
		return d(glyphs, background)
	}
	c := m.caps
	if m.theme.plain {
		c.Profile = termenv.Ascii
	}
	switch glyphs {
	case "unicode":
		c.Unicode = true
	case "ascii":
		c.Unicode = false
	default:
		c.Unicode = m.theme.Glyphs == UnicodeGlyphs
	}
	switch background {
	case "dark":
		c.Dark = true
	case "light":
		c.Dark = false
	}
	return c
}

// candidateTheme is the theme the preview renders in. Until a look-and-feel
// value changes, and while none is overridden, it is the current theme.
func (m Model) candidateTheme() Theme {
	s := m.settings
	same := true
	for _, k := range []string{"theme", "glyphs", "background"} {
		if s.values[k] != s.initial[k] || s.overridden(k) {
			same = false
		}
	}
	if same {
		return m.theme
	}
	th, err := NewTheme(s.values["theme"], m.candidateCaps(s.values["glyphs"], s.values["background"]), m.theme.renderer)
	if err != nil {
		return m.theme
	}
	return th
}

// effectiveTheme re-resolves the theme after a save: the file values, under
// any flag or environment override.
func (m Model) effectiveTheme() Theme {
	env := m.settingsEnv
	f, _, err := config.Load(env.Path)
	if err != nil {
		return m.theme
	}
	pick := func(k string) string {
		v, _ := f.Resolve(k, env.Flags, env.getenv)
		return v
	}
	th, err := NewTheme(pick("theme"), m.candidateCaps(pick("glyphs"), pick("background")), m.theme.renderer)
	if err != nil {
		return m.theme
	}
	return th
}

// candidateTr is the language the screen previews: the language row's value
// while it is the file's to set, and the language the screen opened in while
// a flag or $AVAL_LANG overrides it. "auto" is detected from the environment.
func (m Model) candidateTr() i18n.Translator {
	s := m.settings
	if s.overridden("language") {
		return s.baseTr
	}
	return i18n.New(i18n.Select(s.values["language"], m.settingsEnv.getenv))
}

// cycle steps through choices from cur. A value outside them (a pinned
// theme variant) steps from its family's place.
func cycle(choices []string, cur string, step int) string {
	i := -1
	for j, c := range choices {
		if c == cur {
			i = j
		}
	}
	if i < 0 {
		for j, c := range choices {
			if strings.HasPrefix(cur, c+"-") {
				i = j
			}
		}
		if i >= 0 && step < 0 {
			return choices[i]
		}
	}
	if i < 0 && step < 0 {
		i = 0
	}
	n := len(choices)
	return choices[((i+step)%n+n)%n]
}

func (m Model) settingsAction(act action) (tea.Model, tea.Cmd) {
	s := &m.settings
	s.err = ""
	switch act {
	case actUp:
		s.cursor = max(s.cursor-1, 0)
	case actDown:
		s.cursor = min(s.cursor+1, editableRows()-1)
	case actPrev, actNext:
		step := 1
		if act == actPrev {
			step = -1
		}
		r := settingRows[s.cursor]
		s.values = maps.Clone(s.values)
		s.values[r.key] = cycle(r.choices, s.values[r.key], step)
		s.preview = m.candidateTheme()
		// The whole screen repaints in the chosen language at once.
		m.tr = m.candidateTr()
	case actSave:
		for _, r := range settingRows {
			if r.choices == nil || s.values[r.key] == s.initial[r.key] {
				continue
			}
			if err := config.Set(m.settingsEnv.Path, r.key, s.values[r.key]); err != nil {
				s.err = err.Error()
				return m, nil
			}
		}
		if s.standalone && !m.hosted {
			return m, m.exit()
		}
		m.theme = m.effectiveTheme()
		// Hosted, the router adopts the saved theme, so the hub and every
		// later view repaint in it at once.
		var adopt tea.Cmd
		if m.hosted {
			c := m.chrome
			adopt = func() tea.Msg { return chromeMsg{c} }
		}
		if s.standalone {
			return m, tea.Batch(adopt, m.exit())
		}
		m.screen = s.back
		return m, adopt
	case actDiscard:
		if s.standalone {
			return m, m.exit()
		}
		m.tr = s.baseTr
		m.screen = s.back
	}
	return m, nil
}

func (m Model) settingsView() view {
	return view{body: m.settingsBody, status: m.settingsStatus()}
}

// settingsStatus is a save error, or the override note for the cursor row,
// or for the first overridden row.
func (m Model) settingsStatus() string {
	s := m.settings
	if s.err != "" {
		return s.err
	}
	rows := append([]settingRow{settingRows[s.cursor]}, settingRows[:editableRows()]...)
	for _, r := range rows {
		if !s.overridden(r.key) {
			continue
		}
		name := strings.ToLower(m.tr.T(r.label, nil))
		if s.sources[r.key] == "flag" {
			flag := m.settingsEnv.FlagNames[r.key]
			if flag == "" {
				flag = m.tr.T("settings.a_flag", nil)
			}
			return m.tr.T("settings.set_by_flag", i18n.Args{"name": name, "flag": flag})
		}
		return m.tr.T("settings.set_by_env", i18n.Args{"name": name, "var": config.EnvName(r.key)})
	}
	return ""
}

// tilde shortens a path under the settings home.
func (m Model) tilde(p string) string { return tildeUnder(m.settingsEnv.Home, p) }

func (m Model) settingsBody(l layout, h int) []string {
	head := m.headingRow(l, m.tr.T("settings.title", nil), m.theme.Heading, m.tilde(m.settingsEnv.Path), m.theme.Muted)
	rows, cur := m.settingLines(l)
	preview := m.previewLines(l, false)
	short := m.previewLines(l, true)

	build := func(gaps, fullPreview, withHead bool) []string {
		var out []string
		if withHead {
			out = append(out, head...)
			if gaps {
				out = append(out, "")
			}
		}
		out = append(out, rows...)
		if gaps {
			out = append(out, "")
		}
		if fullPreview {
			return append(out, preview...)
		}
		return append(out, short...)
	}
	for _, try := range [][3]bool{{true, true, true}, {false, true, true}, {false, false, true}, {false, false, false}} {
		if out := build(try[0], try[1], try[2]); len(out) <= h {
			return out
		}
	}
	// Shorter still: the rows scroll with the cursor, the preview goes last.
	pv := short
	room := h - len(pv)
	if room < 1 {
		room, pv = h, nil
	}
	start, end := scrollWindow(len(rows), cur, room)
	return append(append([]string{}, rows[start:end]...), pv...)
}

// settingLines renders the rows and returns the cursor row's index.
func (m Model) settingLines(l layout) ([]string, int) {
	s := m.settings
	compact := l.bp == bpCompact
	// The label column is the widest label and a space, within half the row.
	widestLabel := 0
	for _, r := range settingRows {
		widestLabel = max(widestLabel, lipgloss.Width(m.tr.T(r.label, nil)))
	}
	labelW := min(widestLabel+1, max((l.width-4)/2, 1))
	widest := 13
	for _, r := range settingRows {
		if r.choices != nil {
			widest = max(widest, lipgloss.Width(s.values[r.key]))
		}
	}
	valueW := min(widest, max(l.width-2-labelW-2, 1))
	var lines []string
	cur := 0
	for i, r := range settingRows {
		marker := "  "
		if r.choices != nil && i == s.cursor {
			marker = m.theme.Marker.Render(m.theme.Glyphs.Cursor + " ")
			cur = len(lines)
		}
		label := m.pad(m.tr.T(r.label, nil), labelW)
		if r.choices == nil {
			v := m.tilde(s.repo)
			style := m.theme.Text
			if v == "" {
				v, style = m.tr.T("settings.not_set", nil), m.theme.Muted
			}
			lines = append(lines, marker+m.theme.Text.Render(label)+"  "+style.Render(m.fitMiddle(v, max(l.width-2-labelW-2, 1))))
			continue
		}
		rest := l.width - 2 - labelW - 2 - valueW - 2
		choices := !compact && rest >= 8
		value := m.fit(s.values[r.key], valueW)
		// The value pads to its column only where something follows it or
		// the cursor's reverse video shows the row's extent.
		if choices || i == s.cursor {
			value = m.pad(value, valueW)
		}
		cell := label + "  " + value
		if i == s.cursor {
			cell = m.theme.Selected.Render(cell)
		} else {
			cell = m.theme.Text.Render(cell)
		}
		line := marker + cell
		if choices {
			line += "  " + m.theme.Muted.Render(m.choiceList(r.choices, rest))
		}
		lines = append(lines, line)
	}
	return lines, cur
}

// choiceList is "a · b · c …": whole choices while they fit in w.
func (m Model) choiceList(choices []string, w int) string {
	sep, ell := m.sep(), m.theme.Glyphs.Ellipsis
	out := ""
	for i, c := range choices {
		next := c
		if out != "" {
			next = out + sep + c
		}
		tail := ""
		if i < len(choices)-1 {
			tail = " " + ell
		}
		if lipgloss.Width(next+tail) > w {
			if out == "" {
				return m.fit(c, w)
			}
			return out + " " + ell
		}
		out = next
	}
	return out
}

// previewLines is the preview in the candidate theme, packed into the width.
// oneLine keeps the items that fit on a single line, without the label, for
// a short terminal. It always returns one line at least.
func (m Model) previewLines(l layout, oneLine bool) []string {
	pt := m.settings.preview
	g := pt.Glyphs
	type item struct{ styled, plain string }
	items := []item{
		{pt.Checked.Render(g.Checked) + " " + pt.Text.Render(m.tr.T("settings.preview.selected", nil)), g.Checked + " " + m.tr.T("settings.preview.selected", nil)},
		{pt.Unchecked.Render(g.Unchecked) + " " + pt.Text.Render(m.tr.T("settings.preview.not_selected", nil)), g.Unchecked + " " + m.tr.T("settings.preview.not_selected", nil)},
		{pt.State(StateOK, m.tr.T("install.status.linked", nil)), g.OK + " " + m.tr.T("install.status.linked", nil)},
		{pt.State(StateWarn, m.tr.T("install.word.backed_up", nil)), g.Warn + " " + m.tr.T("install.word.backed_up", nil)},
		{pt.State(StateFail, m.tr.T("install.word.failed", nil)), g.Fail + " " + m.tr.T("install.word.failed", nil)},
	}
	label, gap := m.tr.T("settings.preview.label", nil), "   "
	indent := lipgloss.Width(label)
	var lines []string
	line := m.theme.Text.Render(label)
	if oneLine {
		indent, line = 0, ""
	} else if l.width < indent+lipgloss.Width(items[1].plain) {
		// Too narrow beside the label: the label goes on its own line.
		lines = append(lines, m.theme.Text.Render(strings.TrimSpace(label)))
		indent, line = 2, "  "
	}
	used, empty := lipgloss.Width(line), true
	for _, it := range items {
		w := lipgloss.Width(it.plain)
		if !empty && used+len(gap)+w > l.width {
			if oneLine {
				break
			}
			lines = append(lines, line)
			line, used, empty = strings.Repeat(" ", indent), indent, true
		}
		styled := it.styled
		if used+w > l.width {
			styled = pt.Text.Render(m.fit(it.plain, max(l.width-used, 1)))
			w = lipgloss.Width(m.fit(it.plain, max(l.width-used, 1)))
		}
		if !empty {
			line += gap
			used += len(gap)
		}
		line += styled
		used += w
		empty = false
	}
	return append(lines, line)
}
