package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/ui"
)

func installModel(t *testing.T) ui.Model {
	t.Helper()
	repo, home := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{Groups: []plan.Group{{ID: "one", Label: "One", Selected: true, Actions: []plan.Action{
		{Source: filepath.Join(repo, "a"), Dest: filepath.Join(home, ".a"), Label: "~/.a"},
	}}}}
	r := install.New(install.Options{Home: home, Symlinks: true})
	return ui.NewModel(p, r, ui.Options{Symlinks: true})
}

func keyMsg(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func step(m ui.Model, msg tea.Msg) (ui.Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(ui.Model), cmd
}

func completedModel(t *testing.T) ui.Model {
	t.Helper()
	m, cmd := step(installModel(t), keyMsg("enter"))
	for i := 0; i < 10 && cmd != nil && m.Screen() != ui.ScreenDone; i++ {
		m, cmd = step(m, cmd())
	}
	if !m.Completed() {
		t.Fatal("fixture: model did not complete")
	}
	return m
}

func TestQuittingOnSelectDoesNotFinishTheInstall(t *testing.T) {
	quit, _ := step(installModel(t), keyMsg("q"))
	finished := 0
	err := driveInstall(quit,
		func(tea.Model) (tea.Model, error) { return quit, nil },
		func() error { return nil },
		func() error { finished++; return nil })
	if err != nil {
		t.Fatalf("quitting is not an error: %v", err)
	}
	if finished != 0 {
		t.Errorf("finishInstall ran %d times after quitting on select, want 0", finished)
	}
}

func TestACompletedRunFinishesTheInstallOnce(t *testing.T) {
	done := completedModel(t)
	finished, closed := 0, 0
	err := driveInstall(done,
		func(tea.Model) (tea.Model, error) { return done, nil },
		func() error { closed++; return nil },
		func() error { finished++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if finished != 1 || closed != 1 {
		t.Errorf("finish=%d close=%d, want 1 and 1", finished, closed)
	}
}

func TestProgramErrorSkipsFinish(t *testing.T) {
	boom := errors.New("boom")
	finished := 0
	err := driveInstall(installModel(t),
		func(m tea.Model) (tea.Model, error) { return m, boom },
		func() error { return nil },
		func() error { finished++; return nil })
	if !errors.Is(err, boom) || finished != 0 {
		t.Errorf("err=%v finished=%d, want boom and 0", err, finished)
	}
}

func TestInstallWithoutTerminalFailsBeforeStartingTheProgram(t *testing.T) {
	started := false
	deps := installDeps{
		isTTY: func(uintptr) bool { return false },
		run:   func(m tea.Model) (tea.Model, error) { started = true; return m, nil },
		finish: func(string, string, bool) error {
			t.Error("finish must not run")
			return nil
		},
	}
	err := runInstallWith(t.TempDir(), t.TempDir(), false, false, false, deps)
	want := "aval install needs a terminal. For scripts use: aval install --yes"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if started {
		t.Error("tea program started without a terminal")
	}
}

func TestInstallGuardAllowsYesWithoutTerminal(t *testing.T) {
	if err := installGuard(true, func(uintptr) bool { return false }); err != nil {
		t.Errorf("--yes without a terminal must pass the guard: %v", err)
	}
	if err := installGuard(false, func(uintptr) bool { return true }); err != nil {
		t.Errorf("a terminal must pass the guard: %v", err)
	}
}
