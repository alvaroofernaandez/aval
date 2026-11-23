package ui

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

func init() {
	// Goldens must not depend on the machine's shell.
	userShell = func() string { return "zsh" }
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// doneText renders the done content at width and height as plain text.
func doneText(t *testing.T, m Model, width, h int, stopped bool) string {
	t.Helper()
	m.width, m.height = width, 40
	return ansiRE.ReplaceAllString(strings.Join(m.doneContent(m.layout(), h, stopped), "\n"), "")
}

func doneModel(t *testing.T) Model {
	t.Helper()
	m := screenFixture(t, ScreenDone, true)
	m.errs = []error{fmt.Errorf("link ~/.config/ghostty: source missing")}
	m.queue = make([]plan.Action, 5)
	// The fixture fakes the run, so it advances the index as record would.
	m.index = len(m.results) + len(m.errs)
	return m
}

func TestDoneHeadlineStatesCounts(t *testing.T) {
	m := doneModel(t)
	got := doneText(t, m, 80, math.MaxInt32, false)
	if !strings.Contains(got, "Installed. 2 linked, 1 failed.") {
		t.Errorf("headline missing counts:\n%s", got)
	}
	m.errs = nil
	if got := doneText(t, m, 80, math.MaxInt32, false); !strings.Contains(got, "Installed. 2 linked.") {
		t.Errorf("clean headline wrong:\n%s", got)
	}
}

func TestDoneCountsBlockAndFailureReason(t *testing.T) {
	got := doneText(t, doneModel(t), 80, math.MaxInt32, false)
	for _, want := range []string{"linked", "already in place", "failed", "~/.config/ghostty: source missing"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestDoneUndoLineAndMovedFiles(t *testing.T) {
	got := doneText(t, doneModel(t), 80, math.MaxInt32, false)
	for _, want := range []string{"1 existing file moved to", ".aval-backup", "Undo everything with: aval restore latest"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	// Written, but nothing displaced: still undoable, no moved line.
	m := doneModel(t)
	m.results = []install.Result{{Action: plan.Action{Label: "a"}, Status: install.StatusLinked}}
	got = doneText(t, m, 80, math.MaxInt32, false)
	if !strings.Contains(got, "aval restore latest") || strings.Contains(got, "moved to") {
		t.Errorf("undo without backups wrong:\n%s", got)
	}
}

func TestDoneNextSteps(t *testing.T) {
	got := doneText(t, doneModel(t), 80, math.MaxInt32, false)
	for _, want := range []string{"Next", "aval doctor", "check this machine against the repo", "aval config", "theme, glyphs and update checks", "exec zsh", "reload your shell"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestDoneReloadHintFollowsTheShell(t *testing.T) {
	m := doneModel(t)
	for shell, want := range map[string]string{"bash": "exec bash", "fish": "exec fish"} {
		userShell = func() string { return shell }
		if got := doneText(t, m, 80, math.MaxInt32, false); !strings.Contains(got, want) || strings.Contains(got, "exec zsh") {
			t.Errorf("shell %s: want %q only:\n%s", shell, want, got)
		}
	}
	userShell = func() string { return "nu" }
	if got := doneText(t, m, 80, math.MaxInt32, false); strings.Contains(got, "exec ") {
		t.Errorf("unknown shell must omit the reload hint:\n%s", got)
	}
	userShell = func() string { return "zsh" }
}

func TestDoneDryRunWording(t *testing.T) {
	m := doneModel(t)
	m.DryRun = true
	m.errs = nil
	m.results = []install.Result{
		{Action: plan.Action{Label: "a"}, Status: install.StatusWould},
		{Action: plan.Action{Label: "b"}, Status: install.StatusWould},
	}
	got := doneText(t, m, 80, math.MaxInt32, false)
	if !strings.Contains(got, "Dry run complete: nothing was written.") || !strings.Contains(got, "would") {
		t.Errorf("dry run wording:\n%s", got)
	}
	if strings.Contains(got, "restore") || strings.Contains(got, "Installed.") {
		t.Errorf("dry run must not offer undo or claim an install:\n%s", got)
	}
}

func TestDoneStoppedWording(t *testing.T) {
	m := doneModel(t)
	m.queue = make([]plan.Action, 9)
	got := doneText(t, m, 80, math.MaxInt32, true)
	if !strings.Contains(got, "Stopped after 4 of 9.") || !strings.Contains(got, "Undo everything with: aval restore latest") {
		t.Errorf("stopped wording:\n%s", got)
	}
	if strings.Contains(got, "exec zsh") {
		t.Errorf("a stopped run must not suggest reloading:\n%s", got)
	}
}

func TestDoneHeightPriority(t *testing.T) {
	m := doneModel(t)
	m.errs = []error{fmt.Errorf("link ~/.a: first"), fmt.Errorf("link ~/.b: second")}
	for _, h := range []int{1, 2, 3, 4, 5, 6, 8, 12, 40} {
		if n := len(strings.Split(doneText(t, m, 80, h, false), "\n")); n > h {
			t.Errorf("h=%d: %d lines", h, n)
		}
	}
	// Headline > failures > undo > counts > next.
	at := func(h int) string { return doneText(t, m, 80, h, false) }
	if g := at(1); !strings.Contains(g, "Installed.") {
		t.Errorf("h=1 keeps the headline:\n%s", g)
	}
	if g := at(3); !strings.Contains(g, "first") || !strings.Contains(g, "second") || strings.Contains(g, "restore") {
		t.Errorf("h=3 keeps failures before undo:\n%s", g)
	}
	g := at(6)
	if !strings.Contains(g, "aval restore latest") || strings.Contains(g, "already in place") || strings.Contains(g, "Next") {
		t.Errorf("h=6 keeps undo before counts and next:\n%s", g)
	}
	if g := at(12); !strings.Contains(g, "aval restore latest") || !strings.Contains(g, "already in place") {
		t.Errorf("h=12 keeps undo and counts:\n%s", g)
	}
	if g := at(40); !strings.Contains(g, "exec zsh") {
		t.Errorf("h=40 keeps next steps:\n%s", g)
	}
}

func TestDoneStaysWithinTerminalHeight(t *testing.T) {
	m := doneModel(t)
	for _, h := range []int{8, 12} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: h})
		out := next.(Model).View()
		if n := len(strings.Split(out, "\n")); n > h {
			t.Errorf("height %d: frame has %d lines", h, n)
		}
		if h == 8 && !strings.Contains(out, "aval restore latest") {
			t.Errorf("height 8 must keep the undo command:\n%s", out)
		}
	}
}

// DESIGN.md section 10: colour belongs to state glyphs only; headings are
// bold ink. The headline must never be painted in a state colour.
func TestDoneHeadlineIsBoldInkNotAStateColour(t *testing.T) {
	m := doneModel(t).WithTheme(trueColorTheme(t, "aval"))
	m.width, m.height = 80, 40
	stateColours := map[string]string{
		"ok":   "38;2;152;187;10",
		"warn": "38;2;230;195;13",
		"fail": "38;2;228;104;11",
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*Model)
		stopped bool
	}{
		{"failed", func(*Model) {}, false},
		{"clean", func(m *Model) { m.errs = nil }, false},
		{"dry run", func(m *Model) { m.DryRun = true }, false},
		{"stopped", func(*Model) {}, true},
	} {
		mm := m
		tc.mutate(&mm)
		headline := mm.doneContent(mm.layout(), math.MaxInt32, tc.stopped)[0]
		for state, seq := range stateColours {
			if strings.Contains(headline, seq) {
				t.Errorf("%s headline is painted in the %s colour: %q", tc.name, state, headline)
			}
		}
		if !strings.Contains(headline, "\x1b[1") && !strings.Contains(headline, ";1m") && !strings.Contains(headline, "[1;") {
			t.Errorf("%s headline is not bold: %q", tc.name, headline)
		}
	}
}
