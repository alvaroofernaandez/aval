package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	coreupdate "github.com/alvaroofernaandez/aval/internal/core/update"
	"github.com/alvaroofernaandez/aval/internal/devmode"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// The hub is the root of the router stack (DESIGN.md section 10, Hub and Hub
// icons): the wordmark beside the status panel, a rule, the menu with its
// detail pane, a rule, the status line and the key hints. Opening it writes
// nothing and makes no network call: every service it reads is read-only,
// the update line comes from the cache the last check left, and the
// commits-behind count from refs already fetched.

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
// icon and ascii are its glyph in each tier (one cell, non-ambiguous width),
// key the letter that opens it, and keys what its view offers, for the pane.
type hubEntry struct {
	group, name, view, desc string
	icon, ascii, key        string
	keys                    [][2]string
}

// hubMenu is nine entries in four groups of at most three (Hick).
var hubMenu = []hubEntry{
	{group: "Set up", name: "Install", view: viewInstall, icon: "⇣", ascii: "v", key: "i",
		desc: "Link the repo configs into this home.", keys: [][2]string{{"enter", "pick what to link"}}},
	{name: "Update", view: viewUpdate, icon: "⟳", ascii: "@", key: "u",
		desc: "Pull the repo, then re-apply.", keys: [][2]string{{"enter", "fetch and preview the pull"}}},
	{name: "Sync", view: viewSync, icon: "⇄", ascii: "=", key: "s",
		desc: "Merge repo settings into the live config.", keys: [][2]string{{"enter", "preview the merge"}}},
	{group: "Health", name: "Doctor", view: viewDoctor, icon: "⊞", ascii: "+", key: "d",
		desc: "Check this machine against the repo.", keys: [][2]string{{"enter", "open the full report"}, {"f", "fix what is safe to fix here"}}},
	{group: "Undo", name: "Backups", view: viewBackups, icon: "◷", ascii: "o", key: "b",
		desc: "List backup runs and restore one.", keys: [][2]string{{"enter", "preview a restore"}, {"p", "pin a run"}}},
	{name: "Uninstall", view: viewUninstall, icon: "⌫", ascii: "x", key: "x",
		desc: "Remove aval's links, put back what was there.", keys: [][2]string{{"enter", "preview what goes"}}},
	{group: "Tools", name: "Settings", view: viewSettings, icon: "⎈", ascii: "*", key: ",",
		desc: "Theme, glyphs, motion and update checks.", keys: [][2]string{{"enter", "change them"}}},
	{name: "Dev sandbox", view: viewDev, icon: "⧉", ascii: "#", key: "v",
		desc: "A throwaway home for trying aval.", keys: [][2]string{{"enter", "list them"}, {"n", "new sandbox"}}},
	{name: "About", view: viewAbout, icon: "◉", ascii: "i", key: "a",
		desc: "Version, paths and links.", keys: [][2]string{{"enter", "paths and links"}}},
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

// Menu and pane columns.
const (
	// hubGroupW is the group gutter, shown from gutterFrom columns up: the
	// longest label plus two spaces, so the cursor never touches a label.
	hubGroupW  = 8
	gutterFrom = 60
	// hubFieldW is the name plus its right-aligned badge.
	hubFieldW = 14
	// hubListW is the list column; the pane divider sits right after it.
	hubListW = 27
	// hubPanelGap separates the wordmark from the panel beside it.
	hubPanelGap = 2
	// hubLabelW is the panel's label column ("Version  ").
	hubLabelW = 9
	// hubStateW and hubKeyW are the pane's label and key columns.
	hubStateW = 10
	hubKeyW   = 7
	// hubCursorW is the cursor column the views' lists share ("› ").
	hubCursorW = 2
)

// actHubJump is a letter: open its entry directly.
const actHubJump action = -160

// hubDoctorMsg carries the read-only doctor results.
type hubDoctorMsg struct{ results []doctor.Result }

// hub is the root screen.
type hub struct {
	svc Services
	c   chrome
	// now is the clock relative times are read against.
	now func() time.Time

	cursor int

	// doctor is the generation of the running check; start is the command
	// newHub issued, which init hands to the runtime. doctorAt is when the
	// last result arrived.
	doctor   ticket
	start    tea.Cmd
	checking bool
	results  []doctor.Result
	doctorAt time.Time

	notice    coreupdate.Notice
	runs      []backup.Run
	backupErr error
	sandboxes []devmode.Info

	// behind, upstream and dirty are the local repo status; statusRead is
	// false when no RepoStatus service exists.
	behind          int
	upstream, dirty bool
	statusRead      bool
}

// NewHub builds the hub as the program runs it: a router with the hub at the
// root. Every service it reads at start is read-only.
func NewHub(theme Theme, platform string, svc Services, motion bool) tea.Model {
	c := chrome{theme: theme, version: svc.Build.Version, platform: platform, Motion: motion}
	return newApp(c, svc, newHub(svc))
}

func newHub(svc Services) hub { return newHubAt(svc, time.Now) }

// newHubAt is newHub with the clock the relative times are read against.
func newHubAt(svc Services, now func() time.Time) hub {
	h := hub{svc: svc, now: now}
	if svc.CachedNotice != nil {
		h.notice = svc.CachedNotice()
	}
	h.load()
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

// load reads the local, read-only state: backup runs, sandboxes and the repo
// status against its last fetched upstream.
func (h *hub) load() {
	h.runs, h.backupErr = nil, nil
	if h.svc.Backups != nil {
		h.runs, h.backupErr = h.svc.Backups()
	}
	h.sandboxes = nil
	if h.svc.Sandboxes != nil {
		h.sandboxes, _ = h.svc.Sandboxes()
	}
	h.behind, h.upstream, h.dirty, h.statusRead = 0, false, false, false
	if h.hasRepo() && h.svc.RepoStatus != nil {
		h.behind, h.upstream, h.dirty = h.svc.RepoStatus()
		h.statusRead = true
	}
}

func (h hub) clock() time.Time {
	if h.now == nil {
		return time.Now()
	}
	return h.now()
}

func (h hub) init() tea.Cmd { return h.start }

func (h hub) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		h.c = msg.c
		return h, nil
	case resumeMsg:
		h.load()
		return h, h.runDoctor()
	case tea.KeyMsg:
		act, _ := h.keys().lookup(msg.String())
		switch act {
		case actUp:
			h.cursor = (h.cursor + len(hubMenu) - 1) % len(hubMenu)
		case actDown:
			h.cursor = (h.cursor + 1) % len(hubMenu)
		case actInstall:
			return h, h.open()
		case actHubJump:
			for i, e := range hubMenu {
				if e.key == msg.String() {
					h.cursor = i
					return h, h.open()
				}
			}
		}
		return h, nil
	}
	if inner, ok := h.doctor.accept(msg); ok {
		if r, ok := inner.(hubDoctorMsg); ok {
			h.results, h.checking, h.doctorAt = r.results, false, h.clock()
		}
	}
	return h, nil
}

// open pushes the cursor entry's view, or nothing when it is unavailable.
func (h hub) open() tea.Cmd {
	e := hubMenu[h.cursor]
	if h.unavailable(e) != "" {
		return nil
	}
	ctor, _ := viewFor(e.view)
	return push(ctor(h.svc))
}

// hubKeys is the hub's map. Every letter shares one hint ("i u s ... jump")
// and keeps its own row in the `?` overlay.
var hubKeys = func() keyMap {
	km := keyMap{
		{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: "move"},
		{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: "move"},
		{keys: []string{"enter"}, action: actInstall, label: "enter", desc: "open", core: true},
	}
	letters := make([]string, len(hubMenu))
	for i, e := range hubMenu {
		letters[i] = e.key
	}
	label := strings.Join(letters, " ")
	for _, e := range hubMenu {
		km = append(km, binding{keys: []string{e.key}, action: actHubJump, label: label, desc: "jump", help: "open " + e.name})
	}
	return km
}()

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

// icon is e's glyph in the current tier.
func (h hub) icon(e hubEntry) string {
	if h.unicode() {
		return e.icon
	}
	return e.ascii
}

// divider separates the list from the detail pane.
func (h hub) divider() string {
	if h.unicode() {
		return "│"
	}
	return "|"
}

// arrow marks commits behind upstream.
func (h hub) arrow() string {
	if h.unicode() {
		return "↑"
	}
	return "^"
}

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
	if !h.doctorDone() {
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

// worstCount is the worst state's count and its word: 1, "warn".
func (h hub) worstCount() (int, string) {
	ok, warn, fail := h.counts()
	switch h.worst() {
	case StateFail:
		return fail, "fail"
	case StateWarn:
		return warn, "warn"
	}
	return ok, "ok"
}

// doctorDone is true once a doctor result is on screen.
func (h hub) doctorDone() bool { return !h.checking && h.results != nil }

// topProblem is the first failure, else the first warning; ok is false when
// every check passes.
func (h hub) topProblem() (doctor.Result, StateKind, bool) {
	for _, want := range []struct {
		s doctor.Status
		k StateKind
	}{{doctor.Fail, StateFail}, {doctor.Warn, StateWarn}} {
		for _, r := range h.results {
			if r.Status == want.s {
				return r, want.k, true
			}
		}
	}
	return doctor.Result{}, StateOK, false
}

func problemWord(r doctor.Result) string {
	if r.Message == "" {
		return r.ID
	}
	return r.ID + ": " + r.Message
}

// stateGlyph is the tier's glyph for k.
func (h hub) stateGlyph(k StateKind) string {
	g := h.glyphs()
	return map[StateKind]string{StateOK: g.OK, StateWarn: g.Warn, StateFail: g.Fail}[k]
}

// stateCell is a state glyph and its word, fitted to w columns.
func (h hub) stateCell(k StateKind, word string, w int) string {
	return h.theme().State(k, h.c.fit(word, w-lipgloss.Width(h.stateGlyph(k))-1))
}

// behindCell is the arrow and "N commits behind", fitted to w columns.
func (h hub) behindCell(w int) string {
	word := fmt.Sprintf("%d commits behind", h.behind)
	if h.behind == 1 {
		word = "1 commit behind"
	}
	return h.theme().StateWith(StateWarn, h.arrow(), h.c.fit(word, w-lipgloss.Width(h.arrow())-1))
}

// behindKnown is true when the update badge has something to say.
func (h hub) behindKnown() bool { return h.statusRead && h.upstream && h.behind > 0 }

// newer is the cached version when it is strictly newer than this build.
func (h hub) newer() string {
	if h.notice.String() == "" {
		return ""
	}
	return h.notice.Latest
}

// relDay is when t happened, read against now: "today 12:54",
// "yesterday 18:40", otherwise the date.
func relDay(now, t time.Time) string {
	t = t.In(now.Location())
	clock := t.Format("15:04")
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch {
	case !t.Before(today) && t.Before(today.AddDate(0, 0, 1)):
		return "today " + clock
	case !t.Before(today.AddDate(0, 0, -1)) && t.Before(today):
		return "yesterday " + clock
	}
	return t.Format("2006-01-02 15:04")
}

// shortenPath fits a path into w columns from the front, keeping its tail:
// whole trailing segments when they fit ("…/dev/default/repo"), the last
// characters otherwise. A path under home keeps its "~/" ("~/…/repo").
func (c chrome) shortenPath(home, p string, w int) string {
	s := tildeUnder(home, p)
	if lipgloss.Width(s) <= w {
		return s
	}
	ell := c.theme.Glyphs.Ellipsis
	lead, body := ell, s
	if strings.HasPrefix(s, "~/") {
		lead, body = "~/"+ell, s[1:]
	}
	if room := w - lipgloss.Width(lead); room >= 1 {
		for i := 1; i < len(body); i++ {
			if body[i] == '/' && lipgloss.Width(body[i:]) <= room {
				return lead + body[i:]
			}
		}
	}
	room := w - lipgloss.Width(ell)
	if room < 1 {
		return c.fit(s, w)
	}
	r := []rune(s)
	tail := len(r)
	for tail > 0 && lipgloss.Width(string(r[len(r)-tail:])) > room {
		tail--
	}
	return ell + string(r[len(r)-tail:])
}

// backupRow is a run as the panel and the pane show it: relative day and
// origin.
func (h hub) backupRow(r backup.Run) string {
	row := relDay(h.clock(), r.Time)
	if r.Origin != "" {
		row += h.sep() + r.Origin
	}
	return row
}

// lastRun is the newest run made by origin, or "none yet".
func (h hub) lastRun(origin string) string {
	for _, r := range h.runs {
		if r.Origin == origin {
			return relDay(h.clock(), r.Time)
		}
	}
	return "none yet"
}

// repoCell is the repo path keeping its tail, or "not found".
func (h hub) repoCell(w int) string {
	if !h.hasRepo() {
		return h.stateCell(StateFail, "not found", w)
	}
	return h.theme().Text.Render(h.c.shortenPath(h.svc.Home, h.svc.Repo, w))
}

// doctorCell is the worst state, its count and word: "! 1 warn".
func (h hub) doctorCell(w int) string {
	n, word := h.worstCount()
	return h.stateCell(h.worst(), fmt.Sprintf("%d %s", n, word), w)
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

	var doc string
	switch {
	case !h.hasRepo() || h.svc.Doctor == nil:
		doc = t.Muted.Render(h.c.fit("not run", vw))
	case h.checking:
		doc = text("checking")
	default:
		doc = h.doctorCell(vw)
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

	rows := []struct{ label, value string }{
		{"Version", versionCell},
		{"Repo", h.repoCell(vw)},
		{"Doctor", doc},
		{"Backup", bk},
		{"Theme", text(t.Name)},
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
		cell = h.doctorCell(w)
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
	panel := h.panel(l.width - wmW - hubPanelGap)
	for i, r := range wm {
		row := t.Strong.Render(h.c.pad(r, wmW))
		if i < len(panel) {
			row += strings.Repeat(" ", hubPanelGap) + panel[i]
		}
		rows[i] = row
	}
	return rows
}

// body is the panel (when the hero does not carry it), the menu and the
// detail pane: beside the menu from 80 columns, one line under it from 40 to
// 79, absent when compact. As the height runs short the collapsed pane goes
// first, then the panel collapses to one line, then the menu scrolls.
func (h hub) body(l layout, height int) []string {
	side := l.bp == bpRegular
	var head []string
	switch {
	case l.bp == bpCompact:
		head = []string{h.collapsed(l.width)}
	case l.hero && side:
	default:
		head = append(h.panel(l.width), "")
	}
	var tail []string
	if l.bp == bpNarrow {
		tail = []string{h.paneLine(l.width)}
	}
	if len(head)+len(hubMenu)+len(tail) > height {
		tail = nil
	}
	if len(head) > 1 && len(head)+len(hubMenu) > height {
		head = []string{h.collapsed(l.width)}
	}
	if len(head) > 0 && height-len(head) < 1 {
		head = nil
	}
	rows := h.menu(l)
	start, end := scrollWindow(len(rows), h.cursor, height-len(head)-len(tail))
	rows = rows[start:end]
	if side {
		pane := h.pane(l.width - hubListW - 2)
		div := h.theme().Rule.Render(h.divider())
		for i := range rows {
			rows[i] += div
			if i < len(pane) && pane[i] != "" {
				rows[i] += " " + pane[i]
			}
		}
	}
	return append(append(head, rows...), tail...)
}

// badge is e's right-aligned list badge, plain and styled; empty when it
// carries no information.
func (h hub) badge(e hubEntry) (string, string) {
	t := h.theme()
	count := func(n int) (string, string) {
		if n <= 0 {
			return "", ""
		}
		s := fmt.Sprint(n)
		return s, t.Muted.Render(s)
	}
	if h.unavailable(e) != "" {
		return "off", t.Muted.Render("off")
	}
	switch e.view {
	case viewUpdate:
		if h.behindKnown() {
			n := fmt.Sprint(h.behind)
			return h.arrow() + " " + n, t.StateWith(StateWarn, h.arrow(), n)
		}
	case viewDoctor:
		if h.doctorDone() {
			n, _ := h.worstCount()
			return h.stateGlyph(h.worst()) + " " + fmt.Sprint(n), t.State(h.worst(), fmt.Sprint(n))
		}
	case viewBackups:
		if h.backupErr == nil {
			return count(len(h.runs))
		}
	case viewDev:
		return count(len(h.sandboxes))
	}
	return "", ""
}

// menu renders every entry, one line each: gutter, cursor, icon, name and
// badge. From 80 columns each row is padded to the list column, so the pane
// divider lines up.
func (h hub) menu(l layout) []string {
	t := h.theme()
	gutter := l.bp != bpCompact && l.width >= gutterFrom
	side := l.bp == bpRegular
	lines := make([]string, len(hubMenu))
	for i, e := range hubMenu {
		selected := i == h.cursor
		var prefix string
		if gutter {
			prefix = t.Muted.Render(h.c.pad(e.group, hubGroupW))
		}
		cursor := " "
		if selected {
			cursor = t.Marker.Render(h.glyphs().Cursor)
		}
		prefix += cursor + " "
		used := lipgloss.Width(prefix)

		nameStyle, iconStyle := t.Text, t.Muted
		switch {
		case selected:
			nameStyle, iconStyle = t.Strong, t.Strong
		case h.unavailable(e) != "":
			nameStyle = t.Unavailable
		}
		room := l.width - used
		if side {
			room = hubListW - used
		}
		if room < 3 {
			lines[i] = prefix + nameStyle.Render(h.c.fit(e.name, max(l.width-used, 1)))
			continue
		}
		field := h.field(e, nameStyle, min(hubFieldW, room-2), room-2-1)
		row := prefix + iconStyle.Render(h.icon(e)) + " " + field
		if side {
			row += strings.Repeat(" ", max(hubListW-lipgloss.Width(row), 0))
		}
		lines[i] = row
	}
	return lines
}

// field is the name and its badge in fw columns, the badge right-aligned.
// When both do not fit, the badge may push into the gap up to stretch
// columns; failing that it is dropped and the name keeps the room.
func (h hub) field(e hubEntry, nameStyle lipgloss.Style, fw, stretch int) string {
	plain, styled := h.badge(e)
	nw, bw := lipgloss.Width(e.name), lipgloss.Width(plain)
	switch {
	case bw == 0:
	case nw+1+bw <= fw:
		return nameStyle.Render(e.name) + strings.Repeat(" ", fw-nw-bw) + styled
	case nw+1+bw <= stretch:
		return nameStyle.Render(e.name) + " " + styled
	}
	return nameStyle.Render(h.c.fit(e.name, max(fw, 1)))
}

// stateRow is one pane state line: a muted label and its value.
func (h hub) stateRow(label, value string) string {
	return h.theme().Muted.Render(h.c.pad(label, hubStateW)) + value
}

// pane is the detail pane for the cursor entry, w columns wide and never
// taller than the menu: the icon, name and letter; the description; the
// entry's state block; the keys its view offers.
func (h hub) pane(w int) []string {
	t := h.theme()
	e := hubMenu[h.cursor]
	icon := h.icon(e) + " "
	name := h.c.fit(e.name, max(w-lipgloss.Width(icon)-2, 1))
	gap := max(w-lipgloss.Width(icon)-lipgloss.Width(name)-lipgloss.Width(e.key), 1)
	out := []string{t.Strong.Render(icon+name) + strings.Repeat(" ", gap) + t.Key.Render(e.key)}
	if reason := h.unavailable(e); reason != "" {
		return append(out, t.Unavailable.Render(h.c.fit(reason, w)))
	}
	out = append(out, t.Text.Render(h.c.fit(e.desc, w)))
	if state := h.state(e, max(w-hubStateW, 1)); len(state) > 0 {
		out = append(append(out, ""), state...)
	}
	var keys []string
	for _, k := range e.keys {
		keys = append(keys, t.Key.Render(h.c.pad(k[0], hubKeyW))+t.Muted.Render(h.c.fit(k[1], max(w-hubKeyW, 1))))
	}
	if len(keys) > 0 {
		out = append(append(out, ""), keys...)
	}
	if len(out) > len(hubMenu) {
		out = out[:len(hubMenu)]
	}
	return out
}

// state is the entry's state block, two or three lines, values fitted to vw
// columns, built only from what the hub already holds.
func (h hub) state(e hubEntry, vw int) []string {
	t := h.theme()
	text := func(s string) string { return t.Text.Render(h.c.fit(s, vw)) }
	muted := func(s string) string { return t.Muted.Render(h.c.fit(s, vw)) }
	switch e.view {
	case viewInstall:
		return []string{h.stateRow("Repo", h.repoCell(vw)), h.stateRow("Last run", text(h.lastRun("install")))}
	case viewSync:
		return []string{h.stateRow("Repo", h.repoCell(vw)), h.stateRow("Last run", text(h.lastRun("sync")))}
	case viewUpdate:
		var up string
		switch {
		case !h.statusRead:
			up = muted("not read")
		case !h.upstream:
			up = muted("no upstream fetched")
		case h.behind > 0:
			up = h.behindCell(vw)
		default:
			up = h.stateCell(StateOK, "up to date", vw)
		}
		rows := []string{h.stateRow("Upstream", up)}
		if h.dirty {
			rows = append(rows, h.stateRow("Tree", h.stateCell(StateWarn, "uncommitted changes", vw)))
		}
		if n := h.newer(); n != "" {
			rows = append(rows, h.stateRow("aval", h.stateCell(StateWarn, n+" available", vw)))
		}
		return rows
	case viewDoctor:
		switch {
		case h.svc.Doctor == nil:
			return []string{h.stateRow("Last run", muted("not run"))}
		case h.checking:
			return []string{h.stateRow("Last run", text("checking"))}
		}
		ok, warn, fail := h.counts()
		counts := t.State(StateOK, fmt.Sprintf("%d ok", ok)) + "  " +
			t.State(StateWarn, fmt.Sprintf("%d warn", warn)) + "  " +
			t.State(StateFail, fmt.Sprintf("%d fail", fail))
		if lipgloss.Width(counts) > vw {
			counts = h.doctorCell(vw)
		}
		rows := []string{h.stateRow("Last run", text(relDay(h.clock(), h.doctorAt))), h.stateRow("", counts)}
		if r, k, ok := h.topProblem(); ok {
			rows = append(rows, h.stateRow("Needs you", h.stateCell(k, problemWord(r), vw)))
		}
		return rows
	case viewBackups:
		switch {
		case h.backupErr != nil:
			return []string{h.stateRow("Newest", h.stateCell(StateFail, "could not read backups", vw))}
		case len(h.runs) == 0:
			return []string{h.stateRow("Newest", text("none yet"))}
		}
		var rows []string
		for i, r := range h.runs[:min(len(h.runs), 3)] {
			label := ""
			if i == 0 {
				label = "Newest"
			}
			rows = append(rows, h.stateRow(label, text(h.backupRow(r))))
		}
		return rows
	case viewUninstall:
		return []string{h.stateRow("Safety", text("backs up first"+h.sep()+"aval restore latest"))}
	case viewSettings:
		tier, motion := "ascii", "off"
		if h.unicode() {
			tier = "unicode"
		}
		if h.c.Motion {
			motion = "on"
		}
		return []string{h.stateRow("Theme", text(t.Name)), h.stateRow("Glyphs", text(tier)), h.stateRow("Motion", text(motion))}
	case viewDev:
		var rows []string
		if h.sandboxed() {
			rows = append(rows, h.stateRow("Inside", text(h.svc.Sandbox)))
		}
		names := make([]string, len(h.sandboxes))
		for i, s := range h.sandboxes {
			names[i] = s.Name
		}
		list := strings.Join(names, h.sep())
		if list == "" {
			list = "none yet"
		}
		return append(rows, h.stateRow("Sandboxes", text(list)))
	case viewAbout:
		rows := []string{h.stateRow("Version", text(h.svc.Build.Version))}
		if h.svc.Build.Channel != "" {
			rows = append(rows, h.stateRow("Channel", text(h.svc.Build.Channel)))
		}
		return rows
	}
	return nil
}

// paneLine is the pane collapsed to one line under 80 columns: the most
// urgent state of the cursor entry, else its description.
func (h hub) paneLine(w int) string {
	t := h.theme()
	e := hubMenu[h.cursor]
	if reason := h.unavailable(e); reason != "" {
		return t.Unavailable.Render(h.c.fit(reason, w))
	}
	switch e.view {
	case viewDoctor:
		if r, k, ok := h.topProblem(); ok && h.doctorDone() {
			return h.stateCell(k, problemWord(r), w)
		}
	case viewUpdate:
		switch {
		case h.behindKnown():
			return h.behindCell(w)
		case h.dirty:
			return h.stateCell(StateWarn, "uncommitted changes", w)
		}
	case viewBackups:
		if h.backupErr != nil {
			return h.stateCell(StateFail, "could not read backups", w)
		}
	}
	return t.Text.Render(h.c.fit(e.desc, w))
}
