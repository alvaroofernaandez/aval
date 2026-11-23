package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/repoloc"
	"github.com/alvaroofernaandez/aval/internal/ui"
)

// notFound is the error repoloc returns when no source names a repository.
func notFound(t *testing.T) error {
	t.Helper()
	_, err := repoloc.Resolve(repoloc.Sources{})
	if !errors.Is(err, repoloc.ErrNotFound) {
		t.Fatalf("fixture: Resolve with no sources = %v, want ErrNotFound", err)
	}
	return err
}

type nopCloner struct{ calls int }

func (c *nopCloner) Clone(string) error { c.calls++; return errors.New("no network in tests") }

// welcomeDeps is a terminal on both ends, a fake program and a recording
// finish. finished holds the repo each finish call got.
func welcomeDeps(run func(tea.Model) (tea.Model, error), finished *[]string) installDeps {
	return installDeps{
		isTTY:  func(uintptr) bool { return true },
		run:    run,
		cloner: &nopCloner{},
		finish: func(repo, _ string, _ bool) error {
			*finished = append(*finished, repo)
			return nil
		},
	}
}

func TestInstallWithNoRepoOnATerminalOpensWelcome(t *testing.T) {
	var finished []string
	var screen ui.Screen = -1
	deps := welcomeDeps(func(m tea.Model) (tea.Model, error) {
		screen = m.(ui.Model).Screen()
		next, _ := m.Update(keyMsg("q"))
		return next, nil
	}, &finished)
	err := runInstallWith(func() (string, error) { return "", notFound(t) }, t.TempDir(), false, false, false, deps)
	if err != nil {
		t.Fatalf("no repository on a terminal must open welcome, got %v", err)
	}
	if screen != ui.ScreenWelcome {
		t.Errorf("the program started on screen %v, want welcome", screen)
	}
	// Quitting at welcome records nothing: finish is what writes the repo.
	if len(finished) != 0 {
		t.Errorf("quitting at welcome ran finish with %v", finished)
	}
}

func TestInstallYesWithNoRepoKeepsTheOldError(t *testing.T) {
	want := notFound(t)
	var finished []string
	started := false
	deps := welcomeDeps(func(m tea.Model) (tea.Model, error) { started = true; return m, nil }, &finished)
	err := runInstallWith(func() (string, error) { return "", want }, t.TempDir(), false, true, false, deps)
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if started || len(finished) != 0 {
		t.Errorf("--yes with no repo started=%v finished=%v", started, finished)
	}
}

func TestInstallWithoutTerminalAndNoRepoKeepsTheRepoError(t *testing.T) {
	want := notFound(t)
	var finished []string
	deps := welcomeDeps(func(m tea.Model) (tea.Model, error) {
		t.Error("the program started without a terminal")
		return m, nil
	}, &finished)
	deps.isTTY = func(uintptr) bool { return false }
	err := runInstallWith(func() (string, error) { return "", want }, t.TempDir(), false, false, false, deps)
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestInstallWithAWrongExplicitRepoKeepsFailingLoudly(t *testing.T) {
	_, want := repoloc.Resolve(repoloc.Sources{Flag: t.TempDir()})
	var finished []string
	deps := welcomeDeps(func(m tea.Model) (tea.Model, error) {
		t.Error("a wrong --repo opened the program")
		return m, nil
	}, &finished)
	err := runInstallWith(func() (string, error) { return "", want }, t.TempDir(), false, false, false, deps)
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

// The whole first run in one program: welcome, a typed path, select,
// install, done. finish gets the repository the person chose.
func TestWelcomePathThenACompletedInstallRecordsTheChosenRepo(t *testing.T) {
	t.Chdir(t.TempDir()) // outside any checkout: welcome offers two choices
	repo, home := fakeMachine(t)
	var finished []string
	var final ui.Model
	deps := welcomeDeps(func(tm tea.Model) (tea.Model, error) {
		m := tm.(ui.Model)
		m, _ = step(m, tea.KeyMsg{Type: tea.KeyDown})
		m, _ = step(m, keyMsg("enter"))
		m, _ = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(repo)})
		m, _ = step(m, keyMsg("enter"))
		if m.Screen() != ui.ScreenSelect {
			t.Fatalf("after a valid path the screen is %v, want select:\n%s", m.Screen(), m.View())
		}
		m, cmd := step(m, keyMsg("enter"))
		for i := 0; i < 20 && cmd != nil && m.Screen() != ui.ScreenDone; i++ {
			m, cmd = step(m, cmd())
		}
		final = m
		return m, nil
	}, &finished)
	err := runInstallWith(func() (string, error) { return "", notFound(t) }, home, false, false, false, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !final.Completed() {
		t.Fatalf("fixture: the run did not complete, screen %v", final.Screen())
	}
	if len(finished) != 1 || finished[0] != repo {
		t.Errorf("finish ran with %v, want once with %s", finished, repo)
	}
}

func TestInstallNotifiesOnlyOnceTheRepoQuestionIsSettled(t *testing.T) {
	want := notFound(t)
	var finished []string
	notified := 0
	deps := welcomeDeps(func(m tea.Model) (tea.Model, error) { return m, nil }, &finished)
	deps.notify = func() { notified++ }
	_ = runInstallWith(func() (string, error) { return "", want }, t.TempDir(), false, true, false, deps)
	if notified != 0 {
		t.Errorf("--yes with no repo ran the update check %d times before failing", notified)
	}
	_ = runInstallWith(func() (string, error) { return "", want }, t.TempDir(), false, false, false, deps)
	if notified != 1 {
		t.Errorf("opening welcome ran notify %d times, want 1", notified)
	}
}

func TestCloneURLIsTheCanonicalRemote(t *testing.T) {
	if cloneURL != "https://github.com/alvaroofernaandez/aval.git" {
		t.Errorf("cloneURL = %q", cloneURL)
	}
}

func TestGitClonerSaysWhenGitIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no git: nothing is ever cloned here
	err := gitCloner{ctx: context.Background(), url: cloneURL}.Clone(filepath.Join(t.TempDir(), ".aval"))
	if err == nil || !strings.Contains(err.Error(), "git is not installed") {
		t.Errorf("err = %v, want it to say git is not installed", err)
	}
}
