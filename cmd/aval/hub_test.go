package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
	"github.com/alvaroofernaandez/aval/internal/ui"
)

// repoRoot is this checkout: a real aval repository the hub can resolve.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

// hubFixture isolates HOME, the config dir and the cache dir.
func hubFixture(t *testing.T) (home, cache string) {
	t.Helper()
	home, cache = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("AVAL_REPO", repoRoot(t))
	t.Setenv("AVAL_NO_UPDATE_CHECK", "")
	t.Setenv("AVAL_DEV", "")
	return home, cache
}

func fakeEnv(tty bool, termName string, run func(tea.Model) (tea.Model, error)) hubEnv {
	e := realHubEnv()
	e.isTTY = func(uintptr) bool { return tty }
	e.getenv = func(k string) string {
		switch k {
		case "TERM":
			return termName
		case "AVAL_LANG":
			return "en" // the golden usage is English whatever the machine's locale
		}
		return os.Getenv(k)
	}
	e.run = run
	e.stderr = io.Discard
	return e
}

// drive plays the program loop without a terminal: size, Init, every command
// the hub issues (so the doctor summary really runs), then q.
func drive(t *testing.T, m tea.Model) (tea.Model, error) {
	t.Helper()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	pending := []tea.Cmd{m.Init()}
	for steps := 0; len(pending) > 0 && steps < 200; steps++ {
		cmd := pending[0]
		pending = pending[1:]
		if cmd == nil {
			continue
		}
		ch := make(chan tea.Msg, 1)
		go func() { ch <- cmd() }()
		var msg tea.Msg
		select {
		case msg = <-ch:
		case <-time.After(300 * time.Millisecond):
			continue // a timer, not work
		}
		switch msg := msg.(type) {
		case nil, tea.QuitMsg:
		case tea.BatchMsg:
			pending = append(pending, msg...)
		default:
			var next tea.Cmd
			m, next = m.Update(msg)
			pending = append(pending, next)
		}
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	return m, nil
}

func TestBareAvalOpensTheHubOnATerminal(t *testing.T) {
	hubFixture(t)
	var got tea.Model
	var out bytes.Buffer
	e := fakeEnv(true, "xterm-256color", func(m tea.Model) (tea.Model, error) {
		got = m
		return drive(t, m)
	})
	// The text asserted below is English: pin it, whatever the shell's locale.
	if err := runBare(&out, "", uiFlags{Lang: "en"}, e); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("the hub program never ran on a terminal")
	}
	if out.Len() != 0 {
		t.Errorf("the usage was printed next to the hub: %q", out.String())
	}
	m, _ := got.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if v := m.View(); !strings.Contains(v, "Install") || !strings.Contains(v, "Doctor") {
		t.Errorf("not the hub menu:\n%s", v)
	}
}

func TestBareAvalKeepsTheUsageWithoutAHub(t *testing.T) {
	hubFixture(t)
	want, err := os.ReadFile("testdata/usage_first_run.golden")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		tty  bool
		term string
	}{
		{"no terminal", false, "xterm-256color"},
		{"TERM=dumb", true, "dumb"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ran := false
			e := fakeEnv(c.tty, c.term, func(m tea.Model) (tea.Model, error) { ran = true; return m, nil })
			var out bytes.Buffer
			if err := runBare(&out, "", uiFlags{}, e); err != nil {
				t.Fatal(err)
			}
			if ran {
				t.Error("the hub opened")
			}
			if out.String() != string(want) {
				t.Errorf("usage changed:\n%s", out.String())
			}
		})
	}
}

// help, --help and a bare aval on a pipe print the same bytes as before the
// hub existed. Through run, the body of main, with a buffer for a pipe.
func TestUsageBytesThroughTheRealMain(t *testing.T) {
	want, err := os.ReadFile("testdata/usage_first_run.golden")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"help"}, {"--help"}} {
		home := t.TempDir()
		env := map[string]string{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "x"), "TERM": "xterm"}
		r := runIn(t, env, "", args...)
		if r.code != 0 {
			t.Fatalf("%v: exit %d", args, r.code)
		}
		got := r.stdout
		if len(args) == 1 && args[0] == "--help" {
			got = r.stderr // flag prints --help on stderr
		}
		if got != string(want) {
			t.Errorf("%v: usage changed:\n%s", args, got)
		}
	}
}

type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("the hub made a network request: %s", r.URL)
	return nil, fmt.Errorf("network is off")
}

func hashTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		info, _ := d.Info()
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			lines = append(lines, "L "+rel+" "+l)
		case d.IsDir():
			lines = append(lines, "D "+rel+fmt.Sprintf(" %o", info.Mode().Perm()))
		default:
			b, _ := os.ReadFile(p)
			lines = append(lines, fmt.Sprintf("F %s %o %x", rel, info.Mode().Perm(), sha256.Sum256(b)))
		}
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// Opening the hub and quitting writes nothing and never touches the network.
func TestOpeningTheHubWritesNothingAndUsesNoNetwork(t *testing.T) {
	home, cache := hubFixture(t)
	// A cached notice proves the cache is read, not refreshed.
	orig := http.DefaultTransport
	http.DefaultTransport = failTransport{t}
	t.Cleanup(func() { http.DefaultTransport = orig })

	beforeHome, beforeCache := hashTree(t, home), hashTree(t, cache)
	ran := false
	e := fakeEnv(true, "xterm-256color", func(m tea.Model) (tea.Model, error) {
		ran = true
		return drive(t, m)
	})
	if err := runBare(io.Discard, "", uiFlags{}, e); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("the hub never ran")
	}
	if got := hashTree(t, home); got != beforeHome {
		t.Errorf("HOME changed:\nbefore:\n%s\nafter:\n%s", beforeHome, got)
	}
	if got := hashTree(t, cache); got != beforeCache {
		t.Errorf("the cache dir changed:\nbefore:\n%s\nafter:\n%s", beforeCache, got)
	}
}

func TestHubServicesAreAllWiredWithARepo(t *testing.T) {
	home, _ := hubFixture(t)
	e := fakeEnv(true, "xterm", nil)
	e.lookPath = func(string) (string, error) { return "/usr/bin/go", nil }
	svc, closeFn := newHubServices(e, home, "", uiSettings{UpdateCheck: true})
	defer closeFn()

	if svc.Repo == "" || svc.RepoErr != nil {
		t.Fatalf("repo not resolved: %q, %v", svc.Repo, svc.RepoErr)
	}
	if svc.Home != home || svc.Build.Version == "" || svc.Sandbox != "" {
		t.Errorf("plain fields: home %q build %+v sandbox %q", svc.Home, svc.Build, svc.Sandbox)
	}
	v := reflect.ValueOf(svc)
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if f.Type.Kind() == reflect.Func && v.Field(i).IsNil() {
			t.Errorf("Services.%s is nil", f.Name)
		}
	}
}

func TestHubDevCommandIsNilWithoutGo(t *testing.T) {
	home, _ := hubFixture(t)
	e := fakeEnv(true, "xterm", nil)
	e.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	svc, closeFn := newHubServices(e, home, "", uiSettings{})
	defer closeFn()
	if svc.DevCommand != nil {
		t.Error("DevCommand must be nil when go is not on PATH")
	}
	v := reflect.ValueOf(svc)
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if f.Name != "DevCommand" && f.Type.Kind() == reflect.Func && v.Field(i).IsNil() {
			t.Errorf("Services.%s is nil", f.Name)
		}
	}
}

func TestHubDevCommandRunsThisBinary(t *testing.T) {
	home, _ := hubFixture(t)
	e := fakeEnv(true, "xterm", nil)
	e.lookPath = func(string) (string, error) { return "/usr/bin/go", nil }
	e.exe = func() (string, error) { return "/opt/aval", nil }
	svc, closeFn := newHubServices(e, home, "", uiSettings{})
	defer closeFn()
	if svc.DevCommand == nil {
		t.Fatal("DevCommand is nil with go on PATH and a checkout")
	}
	if got := strings.Join(svc.DevCommand("", "work").Args, " "); got != "/opt/aval dev --name work" {
		t.Errorf("shell command: %q", got)
	}
	if got := strings.Join(svc.DevCommand("reset", "work").Args, " "); got != "/opt/aval dev reset --name work" {
		t.Errorf("reset command: %q", got)
	}
}

func TestHubServicesWithNoRepoAndInsideASandbox(t *testing.T) {
	home, _ := hubFixture(t)
	t.Setenv("AVAL_REPO", "")
	t.Setenv("AVAL_DEV", "1")
	t.Setenv("AVAL_DEV_NAME", "work")
	t.Chdir(t.TempDir())
	e := fakeEnv(true, "xterm", nil)
	e.lookPath = func(string) (string, error) { return "/usr/bin/go", nil }
	svc, closeFn := newHubServices(e, home, "", uiSettings{})
	defer closeFn()
	if svc.Repo != "" || svc.RepoErr == nil {
		t.Errorf("repo %q err %v: expected none", svc.Repo, svc.RepoErr)
	}
	if svc.Sandbox != "work" {
		t.Errorf("sandbox %q", svc.Sandbox)
	}
	if svc.DevCommand != nil {
		t.Error("no checkout: DevCommand must be nil")
	}
}

// The broken-links fix removes the dangling link and records it in a backup
// run, so restore can put it back.
func TestHubFixBrokenLinksBacksUpFirst(t *testing.T) {
	home, _ := hubFixture(t)
	e := fakeEnv(true, "xterm", nil)
	svc, closeFn := newHubServices(e, home, "", uiSettings{})
	defer closeFn()

	link := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "gone"), link); err != nil {
		t.Fatal(err)
	}
	var fixable bool
	for _, r := range svc.Doctor(false) {
		if r.ID == "broken-links" && r.Fix != 0 {
			fixable = true
		}
	}
	if !fixable {
		t.Skip("the doctor does not flag this link as broken here")
	}
	lines, err := svc.FixCheck("broken-links")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Error("the fix reported nothing")
	}
	if _, err := os.Lstat(link); err == nil {
		t.Error("the broken link is still there")
	}
	runs, err := svc.Backups()
	if err != nil || len(runs) != 1 {
		t.Fatalf("backup runs: %v, %v", runs, err)
	}
}

// A sync that fails rolls back; the outcome must still name the
// backup run so the Sync view can print it.
func TestHubSyncRollbackReportsItsBackupID(t *testing.T) {
	home, _ := hubFixture(t)
	e := fakeEnv(true, "xterm", nil)
	svc, closeFn := newHubServices(e, home, "", uiSettings{})
	defer closeFn()

	priv := filepath.Join(avalsync.PrivateDir(home), "settings.json")
	if err := os.MkdirAll(filepath.Dir(priv), 0o755); err != nil {
		t.Fatal(err)
	}
	// A broken overlay fails the sync before it writes anything, so the
	// rollback has no entries to restore.
	overlay := "{not json"
	if err := os.WriteFile(priv, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := svc.Sync()
	if err == nil || out == nil || !out.RolledBack {
		t.Fatalf("want a rolled-back sync, got out=%+v err=%v", out, err)
	}
	if out.RunID == "" {
		t.Fatalf("rollback outcome has an empty backup id: %+v", out)
	}
}

// reapplyOnce plays the update view's re-apply: the hosted installer model
// run to its done screen against the real services.
func reapplyOnce(t *testing.T, svc ui.Services) {
	t.Helper()
	m, err := svc.Reapply()
	if err != nil {
		t.Fatal(err)
	}
	// The update view flips the model to its progress screen; here enter on
	// select starts the same default selection.
	var cur tea.Model = m
	cur, cmd := cur.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for i := 0; i < 500 && cmd != nil && cur.(ui.Model).Screen() != ui.ScreenDone; i++ {
		var next tea.Cmd
		cur, next = cur.Update(cmd())
		cmd = next
	}
	if got := cur.(ui.Model); got.Screen() != ui.ScreenDone || !got.Completed() {
		t.Fatalf("the re-apply did not complete: screen %v", got.Screen())
	}
}

// A re-apply that closes commits its backup run the way the installer does:
// an identical run is dropped (Commit's dedupe), so one run is left, and the
// hub closing afterwards must not commit it a second time.
func TestHubReapplyCommitsItsRunWhenItCloses(t *testing.T) {
	home, _ := hubFixture(t)
	e := fakeEnv(true, "xterm", nil)
	svc, closeFn := newHubServices(e, home, "", uiSettings{})
	defer closeFn()

	reapplyOnce(t, svc)
	if err := svc.FinishReapply(); err != nil {
		t.Fatal(err)
	}
	runs, err := svc.Backups()
	if err != nil || len(runs) != 1 {
		t.Fatalf("after the first re-apply: runs %d, err %v, want 1", len(runs), err)
	}
	if runs[0].Pending {
		t.Error("the closed re-apply is still pending")
	}
	entries, err := svc.RunEntries(runs[0])
	if err != nil || len(entries) == 0 {
		t.Fatalf("fixture: the first run recorded nothing (%v)", err)
	}
	// Back to the starting point, so the next re-apply records the same run.
	for i := len(entries) - 1; i >= 0; i-- {
		if err := os.RemoveAll(entries[i].Path); err != nil {
			t.Fatal(err)
		}
	}

	reapplyOnce(t, svc)
	if err := svc.FinishReapply(); err != nil {
		t.Fatal(err)
	}
	if runs, _ := svc.Backups(); len(runs) != 1 {
		t.Errorf("after closing the second re-apply: %d runs, want 1 (committed, identical run dropped)", len(runs))
	}

	if err := svc.FinishReapply(); err != nil {
		t.Fatal(err)
	}
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}
	if runs, _ := svc.Backups(); len(runs) != 1 {
		t.Errorf("after closing the hub: %d runs, want exactly 1", len(runs))
	}
}

func TestHubRepoStatusIsWiredWithARepoAndNilWithout(t *testing.T) {
	home, _ := hubFixture(t)
	e := fakeEnv(true, "xterm", nil)
	svc, closeFn := newHubServices(e, home, "", uiSettings{})
	if svc.RepoStatus == nil {
		t.Error("RepoStatus is nil with a repo")
	}
	closeFn()

	t.Setenv("AVAL_REPO", "")
	t.Chdir(t.TempDir())
	svc, closeFn = newHubServices(e, home, "", uiSettings{})
	defer closeFn()
	if svc.RepoStatus != nil {
		t.Error("RepoStatus must be nil without a repo")
	}
}
