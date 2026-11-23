package ui

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	coreupdate "github.com/alvaroofernaandez/aval/internal/core/update"
	"github.com/alvaroofernaandez/aval/internal/devmode"
)

// hubNow is the clock every hub test reads relative times against.
var hubNow = time.Date(2026, 11, 23, 13, 30, 0, 0, time.UTC)

// hubServices is a set of fake Services mirroring the hub v2 target: a
// repository three commits behind its upstream, ten ok checks and a warning,
// four backup runs, two sandboxes and a cached notice for 0.3.1. Nothing here
// reaches the network, HOME or git.
func hubServices() Services {
	return Services{
		Home:  "/home/u",
		Repo:  "/home/u/.aval",
		Build: BuildInfo{Version: "0.3.0", Channel: "stable"},
		Doctor: func(bool) []doctor.Result {
			rs := make([]doctor.Result, 0, 11)
			for range 10 {
				rs = append(rs, doctor.Result{ID: "link", Status: doctor.OK})
			}
			return append(rs, doctor.Result{ID: "plugins", Status: doctor.Warn, Message: "1 missing"})
		},
		Backups: func() ([]backup.Run, error) {
			return []backup.Run{
				{ID: "r4", Time: time.Date(2026, 11, 23, 12, 54, 0, 0, time.UTC), Origin: "install", Entries: 3},
				{ID: "r3", Time: time.Date(2026, 11, 22, 18, 40, 0, 0, time.UTC), Origin: "sync", Entries: 1},
				{ID: "r2", Time: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC), Origin: "sync", Entries: 2},
				{ID: "r1", Time: time.Date(2026, 11, 19, 8, 30, 0, 0, time.UTC), Origin: "install", Entries: 5},
			}, nil
		},
		CachedNotice: func() coreupdate.Notice {
			return coreupdate.Notice{Current: "0.3.0", Latest: "0.3.1", Command: "npm i -g aval"}
		},
		DevCommand: func(sub, name string) *exec.Cmd { return nil },
		Sandboxes: func() ([]devmode.Info, error) {
			return []devmode.Info{{Name: "default", Ready: true}, {Name: "scratch", Ready: true}}, nil
		},
		RepoStatus: func() (int, bool, bool) { return 3, true, false },
	}
}

var errNoRepo = errors.New("no aval checkout found")

// withHubViews registers a fake for every menu entry, so availability comes
// only from the services.
func withHubViews(t *testing.T) {
	t.Helper()
	withViews(t)
	for _, e := range hubMenu {
		registerView(e.view, func(Services) screen { return newFake(e.view) })
	}
}

// hubApp is the hub as the program runs it: an App with the hub at the root,
// before Init.
func hubApp(t *testing.T, unicode bool, svc Services) App {
	t.Helper()
	c := chrome{theme: goldenTheme(t, unicode), version: svc.Build.Version, platform: "macOS (arm64)", width: 80, height: 24}
	return newApp(c, svc, newHubAt(svc, func() time.Time { return hubNow }))
}

// startedHub runs Init and feeds back what it produces, so the doctor result
// has arrived.
func startedHub(t *testing.T, unicode bool, svc Services) App {
	t.Helper()
	a := hubApp(t, unicode, svc)
	for _, m := range drainCmd(a.Init()) {
		a, _ = routerStep(a, m)
	}
	return a
}

func init() {
	goldenViews = append(goldenViews, struct {
		name  string
		build func(t *testing.T, unicode bool) tea.Model
	}{"hub", func(t *testing.T, unicode bool) tea.Model {
		withHubViews(t)
		return startedHub(t, unicode, hubServices())
	}})
}

func TestHubShowsCheckingBeforeDoctorArrives(t *testing.T) {
	withHubViews(t)
	a := hubApp(t, false, hubServices())
	msgs := drainCmd(a.Init())
	if out := renderModelAt(a, 80, 24); !strings.Contains(out, "Doctor   checking") {
		t.Errorf("first paint does not show checking:\n%s", out)
	}
	for _, m := range msgs {
		a, _ = routerStep(a, m)
	}
	out := renderModelAt(a, 80, 24)
	if !strings.Contains(out, "Doctor   ! 1 warn") {
		t.Errorf("the worst state is not shown after doctor:\n%s", out)
	}
	if !strings.Contains(out, "1 warning") {
		t.Errorf("the status line does not name the warning:\n%s", out)
	}
}

func TestHubWithoutRepoDisablesEntriesWithReason(t *testing.T) {
	withHubViews(t)
	svc := hubServices()
	svc.Repo, svc.RepoErr = "", errNoRepo
	a := startedHub(t, false, svc)
	h := a.stack[0].(hub)
	live := map[string]bool{viewInstall: true, viewBackups: true, viewSettings: true, viewAbout: true}
	for _, e := range hubMenu {
		reason := h.unavailable(e)
		if live[e.view] && reason != "" {
			t.Errorf("%s is disabled without a repo: %q", e.name, reason)
		}
		if !live[e.view] && reason == "" {
			t.Errorf("%s is live without a repo", e.name)
		}
	}
	out := renderModelAt(a, 120, 40)
	for _, want := range []string{"Repo     x not found", "Not set up yet. Start with Install."} {
		if !strings.Contains(out, want) {
			t.Errorf("no repo: missing %q:\n%s", want, out)
		}
	}
	// Dimmed is invisible in monochrome, so the badge slot says it too.
	if row := menuRow(t, out, "@", "Update"); !strings.HasSuffix(row, "Update     off") {
		t.Errorf("no repo: the update row does not read off: %q", row)
	}
	a, _ = routerStep(a, key("down"))
	out = renderModelAt(a, 120, 40)
	if !strings.Contains(out, "| "+reasonNoRepo) {
		t.Errorf("no repo: the pane does not give the reason for Update:\n%s", out)
	}
	if strings.Contains(out, "checking") {
		t.Errorf("doctor runs without a repo:\n%s", out)
	}
}

func TestHubWordmarkGivesWayToTheLockup(t *testing.T) {
	withHubViews(t)
	a := startedHub(t, true, hubServices())
	tall := renderModelAt(a, 80, 24)
	if !strings.Contains(tall, "▟█▛") || strings.Contains(tall, "aval  0.3.0") {
		t.Errorf("80x24 does not show the wordmark:\n%s", tall)
	}
	short := renderModelAt(a, 80, 20)
	if strings.Contains(short, "▟█▛") || !strings.Contains(short, "aval  0.3.0") {
		t.Errorf("80x20 keeps the wordmark instead of the lockup:\n%s", short)
	}
	if compact := renderModelAt(a, 36, 40); strings.Contains(compact, "▟█▛") {
		t.Errorf("compact shows the wordmark:\n%s", compact)
	}
}

func TestHubUnregisteredViewIsUnavailable(t *testing.T) {
	withViews(t)
	for _, e := range hubMenu {
		if e.view != viewAbout {
			registerView(e.view, func(Services) screen { return newFake(e.view) })
		}
	}
	a := startedHub(t, false, hubServices())
	h := a.stack[0].(hub)
	for _, e := range hubMenu {
		got := h.unavailable(e)
		if e.view == viewAbout && got != reasonNoView {
			t.Errorf("about with no view: reason %q, want %q", got, reasonNoView)
		}
		if e.view != viewAbout && got != "" {
			t.Errorf("%s is unavailable: %q", e.name, got)
		}
	}
	// Enter on it pushes nothing.
	for range len(hubMenu) - 1 {
		a, _ = routerStep(a, key("down"))
	}
	if a, _ = routerStep(a, key("enter")); a.depth() != 1 {
		t.Errorf("enter on an unavailable entry pushed a view (depth %d)", a.depth())
	}
}

func TestHubEnterPushesTheView(t *testing.T) {
	withHubViews(t)
	a := startedHub(t, false, hubServices())
	a, _ = routerStep(a, key("down"))
	a, _ = routerStep(a, key("enter"))
	if a.depth() != 2 {
		t.Fatalf("enter did not push (depth %d)", a.depth())
	}
	if f, ok := a.top().(fakeScreen); !ok || f.name != viewUpdate {
		t.Errorf("enter pushed %v, want the update view", a.top())
	}
}

func TestHubDevSandboxNeedsCheckoutAndGo(t *testing.T) {
	withHubViews(t)
	svc := hubServices()
	svc.DevCommand = nil
	a := startedHub(t, false, svc)
	if got := a.stack[0].(hub).unavailable(hubEntryFor(viewDev)); got != "needs a checkout and Go" {
		t.Errorf("dev sandbox without Go: reason %q", got)
	}
	a, _ = routerStep(a, key("v"))
	if a.depth() != 1 {
		t.Fatalf("v opened the dev sandbox without Go (depth %d)", a.depth())
	}
	if out := renderModelAt(a, 120, 40); !strings.Contains(out, "| needs a checkout and Go") {
		t.Errorf("the reason is not shown:\n%s", out)
	}
}

func TestHubSandboxMarkerReplacesPlatform(t *testing.T) {
	withHubViews(t)
	svc := hubServices()
	svc.Sandbox = "default"
	a := startedHub(t, false, svc)
	for _, h := range []int{24, 20} {
		out := renderModelAt(a, 80, h)
		if !strings.Contains(out, "[aval dev:default]") || strings.Contains(out, "macOS (arm64)") {
			t.Errorf("80x%d inside aval dev: the marker does not replace the platform:\n%s", h, out)
		}
	}
}

func TestHubDropsAStaleDoctorResult(t *testing.T) {
	withHubViews(t)
	calls := 0
	svc := hubServices()
	svc.Doctor = func(bool) []doctor.Result {
		calls++
		if calls == 1 {
			return []doctor.Result{{ID: "old", Status: doctor.Fail}}
		}
		return []doctor.Result{{ID: "new", Status: doctor.OK}}
	}
	a := hubApp(t, false, svc)
	stale := drainCmd(a.Init())
	// Returning to the hub re-runs doctor under a new generation.
	next, fresh := a.Update(resumeMsg{})
	a = next.(App)
	freshMsgs := drainCmd(fresh)
	for _, m := range stale {
		a, _ = routerStep(a, m)
	}
	if out := renderModelAt(a, 80, 24); strings.Contains(out, "1 fail") || !strings.Contains(out, "checking") {
		t.Errorf("the stale doctor result was shown:\n%s", out)
	}
	for _, m := range freshMsgs {
		a, _ = routerStep(a, m)
	}
	if out := renderModelAt(a, 80, 24); !strings.Contains(out, "Doctor   ok 1 ok") {
		t.Errorf("the fresh doctor result was dropped:\n%s", out)
	}
}

func TestHubReturnReloadsBackupsAndDoctor(t *testing.T) {
	withHubViews(t)
	svc := hubServices()
	runs := 0
	svc.Backups = func() ([]backup.Run, error) {
		runs++
		if runs == 1 {
			return nil, nil
		}
		return []backup.Run{{ID: "r2", Time: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC), Origin: "install", Entries: 1}}, nil
	}
	a := startedHub(t, false, svc)
	if out := renderModelAt(a, 80, 24); !strings.Contains(out, "none yet") {
		t.Fatalf("no backups: missing none yet:\n%s", out)
	}
	a, _ = routerStep(a, key("enter"))
	a, _ = routerStep(a, escKey())
	// 80x20 has no room for the wordmark, so the panel spans the full width.
	out := renderModelAt(a, 80, 20)
	if !strings.Contains(out, "Backup   today 09:00  install") {
		t.Errorf("returning did not reload the backup row:\n%s", out)
	}
	if !strings.Contains(out, "Doctor   ! 1 warn") {
		t.Errorf("returning did not re-run doctor:\n%s", out)
	}
}

func TestRouterPopResumesTheScreenBelow(t *testing.T) {
	root := newFake("root")
	root.next = newFake("child")
	a, _ := routerStep(testApp(t, root), key("enter"))
	a, _ = routerStep(a, escKey())
	if !root.received(resumeMsg{}) {
		t.Errorf("esc did not resume the screen below")
	}
	*root.log = nil
	a, _ = routerStep(a, key("enter"))
	routerStep(a, popMsg{})
	if !root.received(resumeMsg{}) {
		t.Errorf("popMsg did not resume the screen below")
	}
}

func TestGoldenHubAt80x24(t *testing.T) {
	for _, tier := range []struct {
		name    string
		unicode bool
	}{{"unicode", true}, {"ascii", false}} {
		t.Run(tier.name, func(t *testing.T) {
			withHubViews(t)
			a := startedHub(t, tier.unicode, hubServices())
			assertGolden(t, "view_hub_"+tier.name+"_80x24", renderModelAt(a, 80, 24))
		})
	}
}
