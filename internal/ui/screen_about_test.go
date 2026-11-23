package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// aboutTestEnv is a fixed environment: no XDG overrides, Linux, and a binary
// under the fake home.
func aboutTestEnv(exe string) aboutEnv {
	return aboutEnv{
		getenv: func(string) string { return "" },
		goos:   "linux",
		exe:    func() (string, error) { return exe, nil },
	}
}

func aboutServices() Services {
	return Services{
		Home:  "/home/u",
		Repo:  "/home/u/.aval",
		Build: BuildInfo{Version: "0.3.0", Commit: "abc1234", Date: "2026-05-06T00:00:00Z", Channel: "stable"},
	}
}

func aboutApp(t *testing.T, unicode bool, svc Services, exe string, w, h int) App {
	t.Helper()
	c := chrome{theme: goldenTheme(t, unicode), version: svc.Build.Version, platform: "macOS (arm64)", width: w, height: h}
	a := newApp(c, svc, newAbout(svc, aboutTestEnv(exe)))
	for _, m := range drainCmd(a.Init()) {
		a, _ = routerStep(a, m)
	}
	next, _ := a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(App)
}

func TestAboutShortensHomePathsWithATilde(t *testing.T) {
	out := aboutApp(t, true, aboutServices(), "/home/u/go/bin/aval", 120, 40).View()
	for _, want := range []string{"~/.aval", "~/.config/aval/config", "~/.aval-backup", "~/.cache/aval/dev", "~/go/bin/aval"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/home/u") {
		t.Errorf("a path under home is not shortened:\n%s", out)
	}
}

func TestAboutKeepsPathsOutsideHomeAbsolute(t *testing.T) {
	out := aboutApp(t, true, aboutServices(), "/usr/local/bin/aval", 120, 40).View()
	if !strings.Contains(out, "/usr/local/bin/aval") {
		t.Errorf("a path outside home was changed:\n%s", out)
	}
}

func TestAboutPathsFollowXDGAndMissingParts(t *testing.T) {
	env := aboutTestEnv("/usr/bin/aval")
	env.getenv = func(k string) string {
		return map[string]string{"XDG_CONFIG_HOME": "/home/u/cfg", "XDG_CACHE_HOME": "/home/u/cc"}[k]
	}
	svc := aboutServices()
	svc.Repo = ""
	svc.RepoErr = errors.New("no checkout")
	c := chrome{theme: goldenTheme(t, true), version: "0.3.0", width: 120, height: 40}
	next, _ := newApp(c, svc, newAbout(svc, env)).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	out := next.View()
	for _, want := range []string{"~/cfg/aval/config", "~/cc/aval/dev", "not found"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
}

// The build block carries the fields `aval version` prints, in its order and
// with its words: version, commit, built, channel.
func TestAboutBuildBlockMatchesAvalVersion(t *testing.T) {
	out := aboutApp(t, true, aboutServices(), "/usr/bin/aval", 120, 40).View()
	rows := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			rows[strings.ToLower(f[0])] = f[1]
		}
	}
	for label, want := range map[string]string{
		"version": "0.3.0", "commit": "abc1234", "built": "2026-05-06T00:00:00Z", "channel": "stable",
	} {
		if rows[label] != want {
			t.Errorf("%s = %q, want %q (as `aval version` prints it):\n%s", label, rows[label], want, out)
		}
	}
	order := []string{"Version", "Commit", "Built", "Channel"}
	last := -1
	for _, w := range order {
		i := strings.Index(out, w)
		if i < last {
			t.Errorf("%s is out of `aval version` order:\n%s", w, out)
		}
		last = i
	}
}

func TestAboutChannelFallsBackToTheVersion(t *testing.T) {
	svc := aboutServices()
	svc.Build = BuildInfo{Version: "0.4.0-rc.1", Commit: "unknown", Date: "unknown"}
	out := aboutApp(t, true, svc, "/usr/bin/aval", 120, 40).View()
	if !strings.Contains(out, "prerelease") {
		t.Errorf("channel not derived from the version:\n%s", out)
	}
}

func TestAboutShowsTheLinksWhole(t *testing.T) {
	out := aboutApp(t, true, aboutServices(), "/usr/bin/aval", 120, 40).View()
	for _, want := range []string{"https://github.com/alvaroofernaandez/aval", "https://github.com/alvaroofernaandez/aval/issues"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
}

func TestAboutLongValuesWrapInsteadOfBeingCut(t *testing.T) {
	out := aboutApp(t, true, aboutServices(), "/usr/bin/aval", 36, 60).View()
	joined := strings.ReplaceAll(strings.ReplaceAll(out, "\n", ""), " ", "")
	if !strings.Contains(joined, "https://github.com/alvaroofernaandez/aval/issues") {
		t.Errorf("the issues link is cut at 36 columns:\n%s", out)
	}
}

func TestAboutScrollsAt80x12(t *testing.T) {
	a := aboutApp(t, true, aboutServices(), "/usr/bin/aval", 80, 12)
	first := a.View()
	if got := len(strings.Split(first, "\n")); got > 12 {
		t.Fatalf("frame is %d lines tall at 80x12", got)
	}
	const last = "aval/issues"
	if strings.Contains(first, last) {
		t.Fatalf("the whole view fits at 80x12, so there is nothing to scroll:\n%s", first)
	}
	for i := 0; i < 30; i++ {
		a, _ = routerStep(a, key("down"))
	}
	if out := a.View(); !strings.Contains(out, last) {
		t.Errorf("scrolling down never reaches the links:\n%s", out)
	}
	if got := len(strings.Split(a.View(), "\n")); got > 12 {
		t.Errorf("scrolled frame is %d lines tall", got)
	}
	for i := 0; i < 40; i++ {
		a, _ = routerStep(a, key("up"))
	}
	if a.View() != first {
		t.Errorf("scrolling back up does not return to the top:\n%s", a.View())
	}
}

func TestAboutEscGoesBack(t *testing.T) {
	root := newFake("root")
	root.next = newAbout(aboutServices(), aboutTestEnv("/usr/bin/aval"))
	a, _ := routerStep(testApp(t, root), key("enter"))
	if a.depth() != 2 {
		t.Fatalf("depth = %d, want 2", a.depth())
	}
	if _, ok := a.keys().lookup("esc"); !ok {
		t.Fatal("esc is not in the key map")
	}
	a, _ = routerStep(a, escKey())
	if a.depth() != 1 {
		t.Errorf("esc did not go back, depth = %d", a.depth())
	}
}

func init() {
	goldenViews = append(goldenViews, struct {
		name  string
		build func(t *testing.T, unicode bool) tea.Model
	}{"about", func(t *testing.T, unicode bool) tea.Model {
		return aboutApp(t, unicode, aboutServices(), "/home/u/go/bin/aval", 80, 40)
	}})
}
