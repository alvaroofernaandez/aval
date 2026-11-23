package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	coreupdate "github.com/alvaroofernaandez/aval/internal/core/update"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// The hub is the root of the router stack (DESIGN.md section 10, Hub): the
// wordmark beside the status panel, a rule, the menu, a rule, the status line
// and the key hints. Opening it writes nothing and makes no network call:
// every service it reads is read-only, and the update line comes from the
// cache the last check left.

// View names, as each screen_<name>.go registers itself.
const (
	viewInstall   = "install"
	viewUpdate    = "update"
	viewSync      = "sync"
	viewDoctor    = "doctor"
	viewBackups   = "backups"
	viewUninstall = "uninstall"
	viewSettings  = "settings"
	viewDev       = "dev"
	viewAbout     = "about"
)

// Reasons an entry is unavailable, shown dimmed in place of its description.
const (
	reasonNoRepo = "needs a repository: run Install first"
	reasonNoView = "not in this build yet"
	reasonNoDev  = "needs a checkout and Go"
)

// hubEntry is one menu row. group is set on the first entry of a group only.
type hubEntry struct{ group, name, view, desc string }

// hubMenu is nine entries in four groups of at most three (Hick).
var hubMenu = []hubEntry{
	{"Set up", "Install", viewInstall, "link the repo configs into this home"},
	{"", "Update", viewUpdate, "pull the repo, then re-apply"},
	{"", "Sync", viewSync, "merge repo-managed settings into the live config"},
	{"Health", "Doctor", viewDoctor, "check this machine against the repo"},
	{"Undo", "Backups", viewBackups, "list backup runs and restore one"},
	{"", "Uninstall", viewUninstall, "remove the links aval made, put back what was there"},
	{"Tools", "Settings", viewSettings, "theme, glyphs, motion and update checks"},
	{"", "Dev sandbox", viewDev, "a throwaway home for trying aval"},
	{"", "About", viewAbout, "version, paths and links"},
}

// liveWithoutRepo are the entries that work before anything is set up.
var liveWithoutRepo = map[string]bool{viewInstall: true, viewBackups: true, viewSettings: true, viewAbout: true}

func hubEntryFor(name string) hubEntry {
	for _, e := range hubMenu {
		if e.view == name {
			return e
		}
	}
	return hubEntry{}
}

// Menu columns: the group gutter (from gutterFrom columns up), the cursor
// and the entry name.
const (
	hubGutterW = 9
	hubCursorW = 2
	hubNameW   = 13
	gutterFrom = 60
	// hubPanelGap separates the wordmark from the panel beside it.
	hubPanelGap = 2
	// hubLabelW is the panel's label column ("Version  ").
	hubLabelW = 9
)

// hubDoctorMsg carries the read-only doctor results.
type hubDoctorMsg struct{ results []doctor.Result }

// hub is the root screen.
type hub struct {
	svc Services
	c   chrome

	cursor int

	// doctor is the generation of the running check; start is the command
	// newHub issued, which init hands to the runtime.
	doctor   ticket
	start    tea.Cmd
	checking bool
	results  []doctor.Result

	notice    coreupdate.Notice
	runs      []backup.Run
	backupErr error
}

// NewHub builds the hub as the program runs it: a router with the hub at the
// root. Every service it reads at start is read-only.
func NewHub(theme Theme, platform string, svc Services) tea.Model {
	c := chrome{theme: theme, version: svc.Build.Version, platform: platform}
	return newApp(c, svc, newHub(svc))
}

func newHub(svc Services) hub {
	h := hub{svc: svc}
	if svc.CachedNotice != nil {
		h.notice = svc.CachedNotice()
	}
	h.loadBackups()
	h.start = h.runDoctor()
	return h
}

// runDoctor starts the read-only checks under a new generation, so a result
// from an earlier run is dropped. With no repository there is nothing to
// check against and doctor does not run.
func (h *hub) runDoctor() tea.Cmd {
	if h.svc.Repo == "" || h.svc.Doctor == nil {
		h.doctor.void()
		h.checking = false
		return nil
	}
	h.checking = true
	run := h.svc.Doctor
	return h.doctor.issue(func() tea.Msg { return hubDoctorMsg{results: run(false)} })
}

func (h *hub) loadBackups() {
	h.runs, h.backupErr = nil, nil
	if h.svc.Backups != nil {
		h.runs, h.backupErr = h.svc.Backups()
	}
}

func (h hub) init() tea.Cmd { return h.start }

func (h hub) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		h.c = msg.c
		return h, nil
	case resumeMsg:
		h.loadBackups()
		return h, h.runDoctor()
	case tea.KeyMsg:
		act, _ := h.keys().lookup(msg.String())
		switch act {
		case actUp:
			h.cursor = (h.cursor + len(hubMenu) - 1) % len(hubMenu)
		case actDown:
			h.cursor = (h.cursor + 1) % len(hubMenu)
		case actInstall:
			e := hubMenu[h.cursor]
			if h.unavailable(e) != "" {
				return h, nil
			}
			ctor, _ := viewFor(e.view)
			return h, push(ctor(h.svc))
		}
		return h, nil
	}
	if inner, ok := h.doctor.accept(msg); ok {
		if r, ok := inner.(hubDoctorMsg); ok {
			h.results, h.checking = r.results, false
		}
	}
	return h, nil
}

var hubKeys = keyMap{
	{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: "move"},
	{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: "move"},
	{keys: []string{"enter"}, action: actInstall, label: "enter", desc: "open", core: true},
}

func (h hub) keys() keyMap     { return hubKeys }
func (h hub) busy() busyKind   { return busyIdle }
func (h hub) hasRepo() bool    { return h.svc.Repo != "" }
func (h hub) compact() bool    { return breakpointFor(h.c.width) == bpCompact }
func (h hub) sep() string      { return h.c.sep() }
func (h hub) theme() Theme     { return h.c.theme }
func (h hub) glyphs() Glyphs   { return h.c.theme.Glyphs }
func (h hub) unicode() bool    { return h.glyphs().Cursor == UnicodeGlyphs.Cursor }
func (h hub) sandboxed() bool  { return h.svc.Sandbox != "" }
func (h hub) marker() string   { return fmt.Sprintf("[aval dev:%s]", h.svc.Sandbox) }
func (h hub) devCapable() bool { return h.hasRepo() && h.svc.DevCommand != nil }

// unavailable is why e cannot open, or "" when it can. Dev sandbox learns
// about Go and the checkout from the services: cmd/aval leaves DevCommand
// nil when `go` is not on PATH, per the nil-means-unavailable contract.
func (h hub) unavailable(e hubEntry) string {
	if e.view == viewDev && !h.devCapable() {
		return reasonNoDev
	}
	if !h.hasRepo() && !liveWithoutRepo[e.view] {
		return reasonNoRepo
	}
	if _, ok := viewFor(e.view); !ok {
		return reasonNoView
	}
	return ""
}

func (h hub) view() view {
	v := view{body: h.body, hero: h.hero, status: h.status()}
	if h.sandboxed() {
		v.platform = h.marker()
	}
	return v
}

// status is the line under the bottom rule. Compact keeps it only when it
// says something the collapsed panel does not.
func (h hub) status() string {
	if !h.hasRepo() {
		return "Not set up yet. Start with Install."
	}
	if h.compact() {
		return h.unavailable(hubMenu[h.cursor])
	}
	if h.checking || h.results == nil {
		return ""
	}
	_, warn, fail := h.counts()
	plural := func(n int, one, many string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s%sopen Doctor to see it", one, h.sep())
		}
		return fmt.Sprintf("%d %s%sopen Doctor to see them", n, many, h.sep())
	}
	switch {
	case fail > 0:
		return plural(fail, "failure", "failures")
	case warn > 0:
		return plural(warn, "warning", "warnings")
	}
	return "Every check passes."
}

func (h hub) counts() (ok, warn, fail int) {
	for _, r := range h.results {
		switch r.Status {
		case doctor.OK:
			ok++
		case doctor.Warn:
			warn++
		case doctor.Fail:
			fail++
		}
	}
	return ok, warn, fail
}

func (h hub) worst() StateKind {
	_, warn, fail := h.counts()
	switch {
	case fail > 0:
		return StateFail
	case warn > 0:
		return StateWarn
	}
	return StateOK
}

// stateCell is a state glyph and its word, fitted to w columns.
func (h hub) stateCell(k StateKind, word string, w int) string {
	g := h.glyphs()
	glyph := map[StateKind]string{StateOK: g.OK, StateWarn: g.Warn, StateFail: g.Fail}[k]
	return h.theme().State(k, h.c.fit(word, w-lipgloss.Width(glyph)-1))
}

// newer is the cached version when it is strictly newer than this build.
func (h hub) newer() string {
	if h.notice.String() == "" {
		return ""
	}
	return h.notice.Latest
}

// panel is the five status rows, each fitted to w columns.
func (h hub) panel(w int) []string {
	t := h.theme()
	vw := max(w-hubLabelW, 1)
	text := func(s string) string { return t.Text.Render(h.c.fit(s, vw)) }

	version := h.svc.Build.Version
	if version == "" {
		version = "unknown"
	}
	versionCell := text(version)
	if n := h.newer(); n != "" {
		lead := version + h.sep()
		if room := vw - lipgloss.Width(lead); room > lipgloss.Width(h.glyphs().Warn)+1 {
			versionCell = t.Text.Render(lead) + h.stateCell(StateWarn, n+" available", room)
		}
	}

	repo := h.stateCell(StateFail, "not found", vw)
	if h.hasRepo() {
		repo = t.Text.Render(h.c.fitMiddle(tildeUnder(h.svc.Home, h.svc.Repo), vw))
	}

	var doc string
	switch {
	case !h.hasRepo() || h.svc.Doctor == nil:
		doc = t.Muted.Render(h.c.fit("not run", vw))
	case h.checking:
		doc = text("checking")
	default:
		ok, warn, fail := h.counts()
		doc = h.stateCell(h.worst(), fmt.Sprintf("%d ok, %d warn, %d fail", ok, warn, fail), vw)
	}

	var bk string
	switch {
	case h.backupErr != nil:
		bk = h.stateCell(StateFail, "could not read backups", vw)
	case len(h.runs) == 0:
		bk = text("none yet")
	default:
		bk = text(h.backupRow(h.runs[0]))
	}

	tier := "ascii"
	if h.unicode() {
		tier = "unicode"
	}
	rows := []struct{ label, value string }{
		{"Version", versionCell},
		{"Repo", repo},
		{"Doctor", doc},
		{"Backup", bk},
		{"Theme", text(t.Name + h.sep() + tier)},
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = t.Muted.Render(h.c.pad(r.label, hubLabelW)) + r.value
		if w < hubLabelW+1 {
			out[i] = t.Muted.Render(h.c.fit(r.label, w))
		}
	}
	return out
}

// backupRow is the newest run: absolute time, origin and entry count.
func (h hub) backupRow(r backup.Run) string {
	parts := []string{r.Time.Format("2006-01-02 15:04")}
	if r.Origin != "" {
		parts = append(parts, r.Origin)
	}
	entries := fmt.Sprintf("%d entries", r.Entries)
	if r.Entries == 1 {
		entries = "1 entry"
	}
	return strings.Join(append(parts, entries), h.sep())
}

// collapsed is the panel in one line: the doctor state and the update.
func (h hub) collapsed(w int) string {
	t := h.theme()
	var cell string
	switch {
	case !h.hasRepo():
		cell = h.stateCell(StateFail, "repo not found", w)
	case h.svc.Doctor == nil:
		cell = t.Muted.Render(h.c.fit("doctor not run", w))
	case h.checking:
		cell = t.Text.Render(h.c.fit("checking", w))
	default:
		ok, warn, fail := h.counts()
		word := fmt.Sprintf("%d ok", ok)
		switch h.worst() {
		case StateFail:
			word = fmt.Sprintf("%d fail", fail)
		case StateWarn:
			word = fmt.Sprintf("%d warn", warn)
		}
		cell = h.stateCell(h.worst(), word, w)
	}
	if n := h.newer(); n != "" {
		extra := "  " + n + " available"
		if lipgloss.Width(cell)+lipgloss.Width(extra) <= w {
			cell += t.Text.Render(extra)
		}
	}
	return cell
}

// hero is the wordmark: beside the panel from 80 columns, alone (the panel
// stacks into the body) from 40 to 79. nil when it does not fit.
func (h hub) hero(l layout, height int) []string {
	t := h.theme()
	wm := wordmark(caps.Caps{Unicode: h.unicode()}, l.width, height)
	if wm == nil {
		return nil
	}
	wmW := 0
	for _, r := range wm {
		wmW = max(wmW, lipgloss.Width(r))
	}
	rows := make([]string, len(wm))
	if l.bp != bpRegular {
		for i, r := range wm {
			rows[i] = t.Strong.Render(r)
		}
		return rows
	}
	pw := l.width - wmW - hubPanelGap
	panel := h.panel(pw)
	off := max((len(wm)-len(panel))/2, 0)
	for i, r := range wm {
		row := t.Strong.Render(h.c.pad(r, wmW))
		if j := i - off; j >= 0 && j < len(panel) {
			row += strings.Repeat(" ", hubPanelGap) + panel[j]
		}
		rows[i] = row
	}
	return rows
}

// body is the panel (when the hero does not carry it) and the menu. As the
// height runs short the panel collapses to one line, then the menu scrolls.
func (h hub) body(l layout, height int) []string {
	menu := h.menu(l)
	var head []string
	switch {
	case l.bp == bpCompact:
		head = []string{h.collapsed(l.width)}
	case l.hero && l.bp == bpRegular:
	default:
		head = append(h.panel(l.width), "")
		if len(head)+len(menu) > height {
			head = []string{h.collapsed(l.width)}
		}
	}
	if len(head) > 0 && height-len(head) < 1 {
		head = nil
	}
	start, end := scrollWindow(len(menu), h.cursor, height-len(head))
	return append(head, menu[start:end]...)
}

// menu renders every entry, one line each.
func (h hub) menu(l layout) []string {
	t := h.theme()
	gutter := l.bp != bpCompact && l.width >= gutterFrom
	lines := make([]string, len(hubMenu))
	for i, e := range hubMenu {
		prefix := ""
		if gutter {
			prefix = t.Muted.Render(h.c.pad(e.group, hubGutterW))
			if e.group == "" {
				prefix = strings.Repeat(" ", hubGutterW)
			}
		}
		marker := strings.Repeat(" ", hubCursorW)
		if i == h.cursor {
			marker = t.Marker.Render(h.c.pad(h.glyphs().Cursor, hubCursorW))
		}
		room := l.width - lipgloss.Width(prefix) - hubCursorW
		reason := h.unavailable(e)
		desc := e.desc
		if reason != "" {
			desc = reason
		}
		var plain string
		if l.bp == bpCompact || room < hubNameW+4 {
			plain = h.c.fit(e.name, max(room, 1))
		} else {
			plain = h.c.pad(e.name, hubNameW) + h.c.fit(desc, room-hubNameW)
		}
		style := t.Text
		switch {
		case i == h.cursor:
			style = t.Selected
		case reason != "":
			style = t.Unavailable
		}
		lines[i] = prefix + marker + style.Render(plain)
	}
	return lines
}
