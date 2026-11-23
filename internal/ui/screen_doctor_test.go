package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/doctor"
)

// doctorFake is a set of fake Services for the doctor view: twelve checks in
// CLI order (nine ok, one warn, two fail, two of them fixable), with every
// call recorded. Nothing here reaches HOME or the network.
type doctorFake struct {
	security []bool
	fixes    []string
	// fixed turns the checks a fix touched to ok on the next run.
	fixed map[string]bool
}

func (f *doctorFake) results(security bool) []doctor.Result {
	f.security = append(f.security, security)
	rs := []doctor.Result{
		{ID: "links", Status: doctor.OK, Message: "57 manifest links in place"},
		{ID: "broken-links", Status: doctor.Fail, Message: "2 broken", Fixable: true, Fix: doctor.FixBrokenLinks,
			Details: []string{"/home/u/.claude/skills/old-skill", "/home/u/.claude/skills/retired"}},
		{ID: "hooks-resolve", Status: doctor.OK, Message: "14 hook commands resolve"},
		{ID: "hooks-retired", Status: doctor.OK, Message: "no retired hooks"},
		{ID: "hooks-drift", Status: doctor.OK, Message: "hooks match the repo"},
		{ID: "agents", Status: doctor.OK, Message: "12 agents pin an alias"},
		{ID: "skills", Status: doctor.Warn, Message: "1 skill incomplete", Details: []string{"/home/u/.claude/skills/draft"}},
		{ID: "plugins", Status: doctor.OK, Message: "3 plugins installed"},
		{ID: "settings-drift", Status: doctor.Fail, Message: "live settings differ in: hooks", Fixable: true, Fix: doctor.FixSync,
			Details: []string{"hooks"}},
		{ID: "overlay", Status: doctor.OK, Message: "no private overlay"},
		{ID: "overlay-links", Status: doctor.OK, Message: "nothing to link"},
		{ID: "config", Status: doctor.OK, Message: "no config file, defaults apply"},
	}
	for i, r := range rs {
		if f.fixed[r.ID] {
			rs[i] = doctor.Result{ID: r.ID, Status: doctor.OK, Message: "fixed"}
		}
	}
	if security {
		rs = append(rs, doctor.Result{ID: "sec-permissions-broad", Status: doctor.Warn, Message: "Bash(*) is allowed"})
	}
	return rs
}

func (f *doctorFake) services() Services {
	return Services{
		Home:   "/home/u",
		Repo:   "/home/u/.aval",
		Build:  BuildInfo{Version: "0.3.0"},
		Doctor: f.results,
		FixCheck: func(id string) ([]string, error) {
			f.fixes = append(f.fixes, id)
			if f.fixed == nil {
				f.fixed = map[string]bool{}
			}
			f.fixed[id] = true
			return []string{"/home/u/.claude/skills/old-skill", "/home/u/.claude/skills/retired"}, nil
		},
	}
}

// doctorApp is a router with a fake hub at the root and the doctor view
// pushed, before its checks arrive: the returned messages are what the push
// produced, to feed back when the test wants them.
func doctorApp(t *testing.T, unicode bool, svc Services) (App, []tea.Msg) {
	t.Helper()
	c := chrome{theme: goldenTheme(t, unicode), version: svc.Build.Version, platform: "macOS (arm64)", width: 80, height: 40}
	a := newApp(c, svc, newFake("hub"))
	ctor, ok := viewFor(viewDoctor)
	if !ok {
		t.Fatalf("the doctor view is not registered")
	}
	next, cmd := a.Update(pushMsg{ctor(svc)})
	return next.(App), drainCmd(cmd)
}

// startedDoctor is the doctor view with its checks arrived.
func startedDoctor(t *testing.T, unicode bool, svc Services) App {
	t.Helper()
	a, msgs := doctorApp(t, unicode, svc)
	for _, m := range msgs {
		a, _ = routerStep(a, m)
	}
	return a
}

func doctorPress(a App, keys ...string) App {
	for _, k := range keys {
		msg := key(k)
		if k == "esc" {
			msg = escKey()
		}
		a, _ = routerStep(a, msg)
	}
	return a
}

// doctorIDs is the list as the view orders it, read from the screen.
func doctorIDs(t *testing.T, a App) []string {
	t.Helper()
	d, ok := a.top().(doctorView)
	if !ok {
		t.Fatalf("top is %T, want the doctor view", a.top())
	}
	ids := make([]string, len(d.rows))
	for i, r := range d.rows {
		ids[i] = r.ID
	}
	return ids
}

func init() {
	type entry = struct {
		name  string
		build func(t *testing.T, unicode bool) tea.Model
	}
	goldenViews = append(goldenViews,
		entry{"doctor", func(t *testing.T, unicode bool) tea.Model {
			f := &doctorFake{}
			return startedDoctor(t, unicode, f.services())
		}},
		entry{"doctor_fix_confirm", func(t *testing.T, unicode bool) tea.Model {
			f := &doctorFake{}
			return doctorPress(startedDoctor(t, unicode, f.services()), "f", "enter")
		}},
	)
}

func TestDoctorFirstPaintShowsChecking(t *testing.T) {
	f := &doctorFake{}
	a, _ := doctorApp(t, false, f.services())
	out := renderModelAt(a, 80, 24)
	if !strings.Contains(out, "Doctor") || !strings.Contains(out, "checking") {
		t.Errorf("first paint does not show checking:\n%s", out)
	}
	if strings.Contains(out, "broken-links") {
		t.Errorf("first paint shows results before they arrived:\n%s", out)
	}
}

func TestDoctorProblemsSortFirst(t *testing.T) {
	f := &doctorFake{}
	a := startedDoctor(t, false, f.services())
	want := []string{
		"broken-links", "settings-drift", "skills",
		"links", "hooks-resolve", "hooks-retired", "hooks-drift", "agents", "plugins", "overlay", "overlay-links", "config",
	}
	if got := doctorIDs(t, a); !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want problems first, then the rest in CLI order: %v", got, want)
	}
	out := renderModelAt(a, 80, 40)
	for _, s := range []string{"9 ok  1 warn  2 fail", "3 of 12 need attention  2 can be fixed here", "x fail   broken-links", "fix available"} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q:\n%s", s, out)
		}
	}
}

func TestDoctorDetailFollowsTheCursor(t *testing.T) {
	f := &doctorFake{}
	a := startedDoctor(t, false, f.services())
	out := renderModelAt(a, 80, 40)
	title, why, _ := doctor.Describe("broken-links")
	for _, s := range []string{title, why[:30], "~/.claude/skills/old-skill", "~/.claude/skills/retired"} {
		if !strings.Contains(out, s) {
			t.Errorf("broken-links detail: missing %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, "How to fix") {
		t.Errorf("a check with a fix key shows How to fix:\n%s", out)
	}

	a = doctorPress(a, "down", "down")
	out = renderModelAt(a, 80, 40)
	title, _, how := doctor.Describe("skills")
	for _, s := range []string{title, "~/.claude/skills/draft", "How to fix", how[:30]} {
		if !strings.Contains(out, s) {
			t.Errorf("skills detail: missing %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, "old-skill") {
		t.Errorf("the detail pane did not follow the cursor:\n%s", out)
	}
}

func TestDoctorFixKeyOnlyWhenTheCheckHasAFix(t *testing.T) {
	f := &doctorFake{}
	a := startedDoctor(t, false, f.services())
	if _, ok := a.keys().lookup("f"); !ok {
		t.Errorf("broken-links has a fix but f is not in the key map")
	}
	a = doctorPress(a, "down", "down") // skills: a warning with no fix
	if _, ok := a.keys().lookup("f"); ok {
		t.Errorf("f is in the key map on a check without a fix")
	}
	if a = doctorPress(a, "f"); a.depth() != 2 {
		t.Errorf("f on a check without a fix opened something (depth %d)", a.depth())
	}
	for _, k := range []string{"a", "r", "s"} {
		if _, ok := a.keys().lookup(k); !ok {
			t.Errorf("%s is not in the key map", k)
		}
	}
}

func TestDoctorFixConfirmsOnCancel(t *testing.T) {
	f := &doctorFake{}
	a := doctorPress(startedDoctor(t, false, f.services()), "f")
	out := renderModelAt(a, 80, 40)
	for _, s := range []string{"~/.claude/skills/old-skill", "~/.claude/skills/retired"} {
		if !strings.Contains(out, s) {
			t.Errorf("the fix preview does not list %q:\n%s", s, out)
		}
	}
	a = doctorPress(a, "enter")
	out = renderModelAt(a, 80, 40)
	for _, s := range []string{"> Cancel", "Remove 2 links", "~/.aval-backup", "aval restore latest"} {
		if !strings.Contains(out, s) {
			t.Errorf("the confirm step: missing %q:\n%s", s, out)
		}
	}
	a = doctorPress(a, "enter") // on Cancel
	if len(f.fixes) != 0 {
		t.Errorf("enter on Cancel called FixCheck %d times", len(f.fixes))
	}
	if a.depth() != 3 {
		t.Errorf("enter on Cancel left the fix (depth %d)", a.depth())
	}
}

func TestDoctorFixRunsOnceThenReruns(t *testing.T) {
	f := &doctorFake{}
	a := doctorPress(startedDoctor(t, false, f.services()), "f", "enter", "up")
	if a.top().busy() != busyIdle {
		t.Fatalf("the confirm step is busy")
	}
	a = doctorPress(a, "enter")
	if !reflect.DeepEqual(f.fixes, []string{"broken-links"}) {
		t.Fatalf("FixCheck calls = %v, want [broken-links] once", f.fixes)
	}
	if out := renderModelAt(a, 80, 40); !strings.Contains(out, "aval restore latest") {
		t.Errorf("the result does not name the undo command:\n%s", out)
	}
	runs := len(f.security)
	a = doctorPress(a, "enter") // close the result
	if a.depth() != 2 {
		t.Fatalf("closing the result did not return to doctor (depth %d)", a.depth())
	}
	if len(f.security) != runs+1 {
		t.Errorf("Doctor ran %d times after the fix, want once", len(f.security)-runs)
	}
	out := renderModelAt(a, 80, 40)
	if !strings.Contains(out, "2 of 12 need attention") {
		t.Errorf("the list did not update after the fix:\n%s", out)
	}
}

func TestDoctorFixAllRunsEachFixOnce(t *testing.T) {
	f := &doctorFake{}
	doctorPress(startedDoctor(t, false, f.services()), "a", "enter", "up", "enter")
	if !reflect.DeepEqual(f.fixes, []string{"broken-links", "settings-drift"}) {
		t.Errorf("fix all called FixCheck with %v", f.fixes)
	}
}

func TestDoctorSecurityKeyAddsTheSecurityChecks(t *testing.T) {
	f := &doctorFake{}
	a := doctorPress(startedDoctor(t, false, f.services()), "s")
	if n := len(f.security); n != 2 || !f.security[1] {
		t.Fatalf("Doctor calls = %v, want a second call with security=true", f.security)
	}
	if out := renderModelAt(a, 80, 40); !strings.Contains(out, "sec-permissions-broad") {
		t.Errorf("the security checks are not listed:\n%s", out)
	}
	a = doctorPress(a, "r")
	if !f.security[2] {
		t.Errorf("re-run dropped the security checks: %v", f.security)
	}
}

func TestDoctorDropsAStaleResult(t *testing.T) {
	f := &doctorFake{}
	svc := f.services()
	calls := 0
	svc.Doctor = func(bool) []doctor.Result {
		calls++
		if calls == 1 {
			return []doctor.Result{{ID: "old", Status: doctor.Fail, Message: "stale"}}
		}
		return []doctor.Result{{ID: "fresh", Status: doctor.OK, Message: "new"}}
	}
	a, stale := doctorApp(t, false, svc)
	next, cmd := a.Update(key("r"))
	a = next.(App)
	fresh := drainCmd(cmd)
	for _, m := range stale {
		a, _ = routerStep(a, m)
	}
	if out := renderModelAt(a, 80, 24); strings.Contains(out, "old") || !strings.Contains(out, "checking") {
		t.Errorf("the stale result was shown:\n%s", out)
	}
	for _, m := range fresh {
		a, _ = routerStep(a, m)
	}
	if out := renderModelAt(a, 80, 24); !strings.Contains(out, "fresh") {
		t.Errorf("the fresh result was dropped:\n%s", out)
	}
}

func TestDoctorEscGoesBack(t *testing.T) {
	f := &doctorFake{}
	if a := doctorPress(startedDoctor(t, false, f.services()), "esc"); a.depth() != 1 {
		t.Errorf("esc did not go back (depth %d)", a.depth())
	}
}
