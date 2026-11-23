package ui

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/devmode"
	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/release"
)

// The About view (DESIGN.md section 10, plan H19) shows what this binary is
// and where it lives: the build block `aval version` prints, the paths aval
// reads and writes, and where to report problems. It writes nothing. Values
// that overflow wrap rather than get cut, since a path or a link is only
// useful whole. The view scrolls when the terminal is short.

const (
	aboutSourceURL = "https://github.com/alvaroofernaandez/aval"
	aboutIssuesURL = aboutSourceURL + "/issues"
)

func init() {
	registerView(viewAbout, func(svc Services) screen { return newAbout(svc, realAboutEnv()) })
}

// aboutEnv is what the view reads from the process to derive the paths the
// way cmd/aval does: the environment, the OS and the running binary.
type aboutEnv struct {
	getenv func(string) string
	goos   string
	exe    func() (string, error)
}

func realAboutEnv() aboutEnv {
	return aboutEnv{getenv: os.Getenv, goos: runtime.GOOS, exe: os.Executable}
}

const (
	actAboutUp   action = -150
	actAboutDown action = -151
)

var aboutKeys = keyMap{
	{keys: []string{"up", "k"}, action: actAboutUp, label: labelUpDown, desc: i18n.Key("key.scroll"), core: true},
	{keys: []string{"down", "j"}, action: actAboutDown, label: labelUpDown, desc: i18n.Key("key.scroll"), core: true},
}

// aboutScroll is shared by the copies of the screen. The body is the only
// place that knows how many lines fit, so it records how far the view can
// scroll and update clamps against it.
type aboutScroll struct{ top, max int }

type about struct {
	svc    Services
	env    aboutEnv
	c      chrome
	scroll *aboutScroll
}

func newAbout(svc Services, env aboutEnv) about {
	return about{svc: svc, env: env, scroll: &aboutScroll{}}
}

func (s about) init() tea.Cmd { return nil }

func (s about) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case chromeMsg:
		s.c = msg.c
	case tea.KeyMsg:
		act, _ := s.keys().lookup(msg.String())
		switch act {
		case actAboutUp:
			s.scroll.top = max(s.scroll.top-1, 0)
		case actAboutDown:
			s.scroll.top = min(s.scroll.top+1, s.scroll.max)
		}
	}
	return s, nil
}

func (s about) keys() keyMap   { return aboutKeys }
func (s about) busy() busyKind { return busyIdle }

func (s about) view() view { return view{body: s.body} }

// aboutRow is one label and value.
type aboutRow struct{ label, value string }

// build is the block of `aval version`: version, commit, built, channel.
func (s about) build() []aboutRow {
	b := s.svc.Build
	or := func(v, def string) string {
		if v == "" {
			return def
		}
		return v
	}
	channel := b.Channel
	if channel == "" {
		channel = release.Channel(b.Version)
	}
	unknown := s.c.tr.T("about.unknown", nil)
	return []aboutRow{
		{s.c.tr.T("about.version", nil), or(b.Version, unknown)},
		{s.c.tr.T("about.commit", nil), or(b.Commit, unknown)},
		{s.c.tr.T("about.built", nil), or(b.Date, unknown)},
		{s.c.tr.T("about.channel", nil), channel},
	}
}

// paths derives each location the way cmd/aval does, shortened under home.
func (s about) paths() []aboutRow {
	home := s.svc.Home
	short := func(p string) string { return tildeUnder(home, p) }
	getenv := s.env.getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	repo := s.c.tr.T("about.not_found", nil)
	if s.svc.Repo != "" {
		repo = short(s.svc.Repo)
	}
	exe := s.c.tr.T("about.unknown", nil)
	if s.env.exe != nil {
		if p, err := s.env.exe(); err == nil && p != "" {
			exe = short(filepath.Clean(p))
		}
	}
	return []aboutRow{
		{s.c.tr.T("about.repository", nil), repo},
		{s.c.tr.T("about.config", nil), short(config.Path(getenv, home, s.env.goos))},
		{s.c.tr.T("about.backups", nil), short(filepath.Join(home, ".aval-backup"))},
		{s.c.tr.T("about.sandboxes", nil), short(devmode.BaseDir(getenv, home))},
		{s.c.tr.T("about.binary", nil), exe},
	}
}

// labelW is the label column: the widest label of any section and two
// spaces, so a longer language never cuts a label.
func (s about) labelW() int {
	w := 0
	for _, rows := range [][]aboutRow{s.build(), s.paths(), s.links()} {
		for _, r := range rows {
			w = max(w, lipgloss.Width(r.label))
		}
	}
	return w + 2
}

// links are where to read the source and report a problem.
func (s about) links() []aboutRow {
	return []aboutRow{{s.c.tr.T("about.source", nil), aboutSourceURL}, {s.c.tr.T("about.issues", nil), aboutIssuesURL}}
}

// hardWrap breaks s into lines of at most w columns at any rune, so a path
// or a link stays whole across lines.
func hardWrap(s string, w int) []string {
	w = max(w, 1)
	var lines []string
	var cur strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w && used > 0 {
			lines = append(lines, cur.String())
			cur.Reset()
			used = 0
		}
		cur.WriteRune(r)
		used += rw
	}
	return append(lines, cur.String())
}

// section renders a title and its rows. With room the label sits beside the
// value; compact stacks the label over its value.
func (s about) section(l layout, title string, rows []aboutRow) []string {
	t := s.c.theme
	out := []string{t.Strong.Render(title)}
	beside := l.width >= 44
	for _, r := range rows {
		if !beside {
			out = append(out, t.Muted.Render(s.c.fit(r.label, l.width)))
			for _, v := range hardWrap(r.value, l.width-2) {
				out = append(out, "  "+t.Text.Render(v))
			}
			continue
		}
		labelW := s.labelW()
		for i, v := range hardWrap(r.value, l.width-labelW) {
			label := strings.Repeat(" ", labelW)
			if i == 0 {
				label = t.Muted.Render(s.c.pad(r.label, labelW))
			}
			out = append(out, label+t.Text.Render(v))
		}
	}
	return out
}

func (s about) lines(l layout) []string {
	var out []string
	for i, sec := range [][]string{
		s.section(l, s.c.tr.T("about.build", nil), s.build()),
		s.section(l, s.c.tr.T("about.paths", nil), s.paths()),
		s.section(l, s.c.tr.T("about.links", nil), s.links()),
	} {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, sec...)
	}
	return out
}

// body is the sections, windowed to h lines at the current scroll. The frame
// also calls it with math.MaxInt32 to measure the natural height; that call
// must not touch the scroll.
func (s about) body(l layout, h int) []string {
	all := s.lines(l)
	if h == math.MaxInt32 {
		return all
	}
	s.scroll.max = max(len(all)-h, 0)
	s.scroll.top = min(s.scroll.top, s.scroll.max)
	return all[s.scroll.top : s.scroll.top+min(h, len(all))]
}
