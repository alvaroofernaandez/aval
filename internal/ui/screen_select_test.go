package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

// previewModel builds a select screen over a real temp home where each state
// exists once: ~/.n is missing (new), ~/.l already links to the repo (linked)
// and ~/.r is a file of the user's (replaces a file). Group Two has two new
// destinations.
func previewModel(t *testing.T) Model {
	t.Helper()
	repo, home := t.TempDir(), t.TempDir()
	src := func(n string) string {
		p := filepath.Join(repo, n)
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	act := func(n string) plan.Action {
		return plan.Action{Source: src(n), Dest: filepath.Join(home, "."+n), Label: "~/." + n}
	}
	n, l, r := act("n"), act("l"), act("r")
	if err := os.Symlink(l.Source, l.Dest); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.WriteFile(r.Dest, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{Groups: []plan.Group{
		{ID: "one", Label: "Group One", Detail: "first", Selected: true, Actions: []plan.Action{n, l, r}},
		{ID: "two", Label: "Group Two", Detail: "second", Selected: true, Actions: []plan.Action{act("x"), act("y")}},
	}}
	run := install.New(install.Options{Home: home, BackupRoot: filepath.Join(home, ".aval-backup", "t"), Symlinks: true})
	m := NewModel(p, run, Options{Version: "0.2.0", Platform: "macOS (arm64)", Symlinks: true})
	return m.WithTheme(goldenTheme(t, true))
}

func press(m Model, k tea.KeyMsg) Model {
	next, _ := m.Update(k)
	return next.(Model)
}

var (
	keyRight = tea.KeyMsg{Type: tea.KeyRight}
	keyLeft  = tea.KeyMsg{Type: tea.KeyLeft}
)

func TestSelectRowsCountItemsAndStates(t *testing.T) {
	v := renderAtSize(previewModel(t), 100, 40)
	for _, want := range []string{
		"3 items",
		"1 new · 1 linked · 1 replaces a file",
		"2 items",
		"2 new",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("select view is missing %q:\n%s", want, v)
		}
	}
}

func TestSelectStatusTotalsTheSelection(t *testing.T) {
	m := previewModel(t)
	want := "5 of 5 items selected · 3 new · 1 linked · 1 file moved to backup"
	if v := renderAtSize(m, 100, 40); !strings.Contains(v, want) {
		t.Errorf("status line is missing %q:\n%s", want, v)
	}

	// Deselecting Group Two drops its two new items from every count.
	m = send(send(m, "down"), " ")
	want = "3 of 5 items selected · 1 new · 1 linked · 1 file moved to backup"
	if v := renderAtSize(m, 100, 40); !strings.Contains(v, want) {
		t.Errorf("status line is missing %q:\n%s", want, v)
	}

	m = send(m, "n")
	want = "0 of 5 items selected · 0 new · 0 linked · 0 files moved to backup"
	if v := renderAtSize(m, 100, 40); !strings.Contains(v, want) {
		t.Errorf("status line is missing %q:\n%s", want, v)
	}
}

func TestSelectStatusShrinksByBreakpoint(t *testing.T) {
	m := previewModel(t)
	if v := renderAtSize(m, 36, 40); !strings.Contains(v, "5/5 selected") {
		t.Errorf("compact status must read 5/5 selected:\n%s", v)
	}
	// Narrow keeps the backup count, the one number that means a file moves.
	v := renderAtSize(m, 50, 40)
	if !strings.Contains(v, "1 to backup") {
		t.Errorf("narrow status must keep the backup count:\n%s", v)
	}
}

func TestSelectPreviewIsTakenOnceNotOnEveryView(t *testing.T) {
	m := previewModel(t)
	// Remove the user's file after the model is built: a View that re-ran
	// Preview would now report two new destinations instead of one replace.
	for _, a := range m.plan.Groups[0].Actions {
		if a.Label == "~/.r" {
			if err := os.Remove(a.Dest); err != nil {
				t.Fatal(err)
			}
		}
	}
	if v := renderAtSize(m, 100, 40); !strings.Contains(v, "1 replaces a file") {
		t.Errorf("the preview must be computed once, not per View:\n%s", v)
	}
}

func TestRightExpandsTheCursorGroupAndLeftCollapsesIt(t *testing.T) {
	m := previewModel(t)
	if v := renderAtSize(m, 100, 40); strings.Contains(v, "~/.r") {
		t.Fatalf("destinations must stay hidden until expanded:\n%s", v)
	}

	m = press(m, keyRight)
	v := renderAtSize(m, 100, 40)
	for _, want := range []string{"~/.n", "~/.l", "~/.r"} {
		if !strings.Contains(v, want) {
			t.Errorf("expanded group is missing %q:\n%s", want, v)
		}
	}
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "~/.r") && !strings.Contains(line, "replaces a file") {
			t.Errorf("a destination line must carry its state word: %q", line)
		}
		if strings.Contains(line, "~/.l") && !strings.Contains(line, "linked") {
			t.Errorf("a destination line must carry its state word: %q", line)
		}
	}
	if strings.Contains(v, "~/.x") {
		t.Error("only the cursor group expands")
	}

	m = press(m, keyLeft)
	if v := renderAtSize(m, 100, 40); strings.Contains(v, "~/.r") {
		t.Errorf("left must collapse the group:\n%s", v)
	}
}

func TestDetailsHintIsListed(t *testing.T) {
	v := renderAtSize(previewModel(t), 100, 40)
	if !strings.Contains(v, "←→ details") {
		t.Errorf("hint line must offer ←→ details:\n%s", v)
	}
}

func TestDetailLineShowsOnlyForTheCursorRow(t *testing.T) {
	m := previewModel(t)
	v := renderAtSize(m, 100, 40)
	if !strings.Contains(v, "first") || strings.Contains(v, "second") {
		t.Errorf("only the cursor row shows its detail:\n%s", v)
	}
	v = renderAtSize(send(m, "down"), 100, 40)
	if strings.Contains(v, "first") || !strings.Contains(v, "second") {
		t.Errorf("the detail line must follow the cursor:\n%s", v)
	}
}

func TestEnterWithNothingSelectedSaysWhy(t *testing.T) {
	m := send(send(previewModel(t), "n"), "enter")
	if m.Screen() != ScreenSelect {
		t.Fatalf("screen = %v, want select", m.Screen())
	}
	if v := renderAtSize(m, 100, 40); !strings.Contains(v, "Select at least one group.") {
		t.Errorf("empty enter must explain itself:\n%s", v)
	}
	// The message answers one key press; the next action clears it.
	m = send(m, " ")
	if v := renderAtSize(m, 100, 40); strings.Contains(v, "Select at least one group.") {
		t.Errorf("the message must clear once the selection changes:\n%s", v)
	}
}

func TestExpandedListStaysInsideTheFrame(t *testing.T) {
	home := t.TempDir()
	var acts []plan.Action
	for i := range 30 {
		acts = append(acts, plan.Action{Dest: filepath.Join(home, fmt.Sprint(i)), Label: fmt.Sprintf("~/dest-%02d", i)})
	}
	p := &plan.Plan{Groups: []plan.Group{
		{ID: "a", Label: "Alpha", Selected: true, Actions: acts[:2]},
		{ID: "b", Label: "Bravo", Selected: true, Actions: acts},
		{ID: "c", Label: "Charlie", Selected: true, Actions: acts[:1]},
	}}
	m := NewModel(p, install.New(install.Options{Home: home, BackupRoot: filepath.Join(home, "bk"), Symlinks: true}), Options{Version: "0.2.0", Symlinks: true})
	m = m.WithTheme(goldenTheme(t, true))
	m = press(send(m, "down"), keyRight)
	for _, h := range []int{8, 12, 16, 24} {
		v := renderAtSize(m, 80, h)
		lines := strings.Split(v, "\n")
		if len(lines) > h {
			t.Errorf("height %d: frame is %d lines", h, len(lines))
		}
		if !strings.Contains(v, "› [√] Bravo") {
			t.Errorf("height %d: the cursor row must stay visible:\n%s", h, v)
		}
	}
}

func TestSelectViewsHaveNoEmDash(t *testing.T) {
	for _, unicode := range []bool{true, false} {
		m := screenFixture(t, ScreenSelect, unicode)
		for _, mm := range []Model{m, press(m, keyRight), send(send(m, "n"), "enter")} {
			for _, w := range []int{36, 60, 80, 120} {
				if v := renderAt(mm, w); strings.Contains(v, "—") {
					t.Errorf("unicode=%v width %d: em dash in UI copy:\n%s", unicode, w, v)
				}
			}
		}
	}
}
