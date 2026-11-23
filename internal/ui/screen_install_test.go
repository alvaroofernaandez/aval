package ui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// startInstall presses enter and returns the model with the first action's
// command in flight, the way bubbletea leaves it.
func startInstall(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	if m.Screen() != ScreenInstall || cmd == nil {
		t.Fatalf("fixture: install did not start (screen %v)", m.Screen())
	}
	return m, cmd
}

// drain runs commands until the model reaches the done screen.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; i < 10 && cmd != nil && m.Screen() != ScreenDone; i++ {
		next, c := m.Update(cmd())
		m, cmd = next.(Model), c
	}
	return m
}

func lineWith(v, want string) string {
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, want) {
			return l
		}
	}
	return ""
}

func TestStopMidInstallFinishesTheCurrentStepAndSkipsTheRest(t *testing.T) {
	for _, k := range []tea.KeyMsg{{Type: tea.KeyCtrlC}, key("q")} {
		t.Run(k.String(), func(t *testing.T) {
			m, cmd := startInstall(t, testModel(t))

			next, stopCmd := m.Update(k)
			m = next.(Model)
			if m.Quitting || stopCmd != nil {
				t.Fatal("stop must not quit: the step in flight still has to finish")
			}
			if !strings.Contains(m.View(), "stopping after the current step") {
				t.Errorf("the status must say the run is stopping:\n%s", m.View())
			}

			m = drain(t, m, cmd)
			if m.Screen() != ScreenDone {
				t.Fatalf("screen = %v, want done", m.Screen())
			}
			if !m.Stopped() {
				t.Error("Stopped() = false after a stop with actions left")
			}
			if m.Completed() {
				t.Error("Completed() = true for a stopped run")
			}
			if done, total := m.Progress(); done != 1 || total != 2 {
				t.Errorf("Progress() = %d of %d, want 1 of 2", done, total)
			}
			if _, err := os.Lstat(m.queue[0].Dest); err != nil {
				t.Errorf("the step in flight was not completed: %v", err)
			}
			if _, err := os.Lstat(m.queue[1].Dest); err == nil {
				t.Error("an action after the stop was applied")
			}
		})
	}
}

func TestStopDuringTheLastStepIsACompletedRun(t *testing.T) {
	// Nothing was skipped, so there is nothing to call stopped.
	m, cmd := startInstall(t, testModel(t))
	next, cmd := m.Update(cmd())
	m = next.(Model)
	m = send(m, "q")
	m = drain(t, m, cmd)
	if m.Stopped() {
		t.Error("Stopped() = true although every action ran")
	}
	if !m.Completed() {
		t.Error("Completed() = false although every action ran")
	}
}

func TestInstallHintOffersTheStop(t *testing.T) {
	m, _ := startInstall(t, testModel(t))
	if !strings.Contains(m.View(), "ctrl+c stops after the current step") {
		t.Errorf("the install hint must offer the stop:\n%s", m.View())
	}
}

func TestFailuresShowInlineWithPathAndReason(t *testing.T) {
	m := testModel(t)
	if err := os.Remove(m.plan.Groups[0].Actions[0].Source); err != nil {
		t.Fatal(err)
	}
	m, cmd := startInstall(t, m)
	next, _ := m.Update(cmd())
	m = next.(Model)

	row := lineWith(m.View(), "~/.a")
	g := m.theme.Glyphs
	for _, want := range []string{g.Fail + " failed", "source missing"} {
		if !strings.Contains(row, want) {
			t.Errorf("failure row %q is missing %q", row, want)
		}
	}
	if strings.Contains(row, m.plan.Groups[0].Actions[0].Source) {
		t.Errorf("the reason must be short, not the full error: %q", row)
	}
	if running := lineWith(m.View(), "~/.b"); !strings.Contains(running, g.Cursor+" linking") {
		t.Errorf("current action row = %q, want %q", running, g.Cursor+" linking")
	}
}
