package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/uninstall"
)

// uninstallProbe counts what the view asked of the fake services.
type uninstallProbe struct{ previews, runs int }

func uninstallItems() []uninstall.Item {
	return []uninstall.Item{
		{Path: "/home/u/.zshrc", Kind: uninstall.KindRemoveLink, Restores: true},
		{Path: "/home/u/.config/tmux", Kind: uninstall.KindRemoveLink},
		{Path: "/home/u/.codex/AGENTS.md", Kind: uninstall.KindCutBlock},
	}
}

func uninstallServices(p *uninstallProbe, items []uninstall.Item) Services {
	return Services{
		Home: "/home/u",
		UninstallPreview: func() ([]uninstall.Item, error) {
			p.previews++
			return items, nil
		},
		Uninstall: func() ([]uninstall.Item, error) {
			p.runs++
			return items, nil
		},
	}
}

func loadedUninstall(t *testing.T, svc Services) screen {
	t.Helper()
	return loaded(t, newUninstallView(svc))
}

func TestUninstallOpeningCallsOnlyThePreview(t *testing.T) {
	p := &uninstallProbe{}
	s := loadedUninstall(t, uninstallServices(p, uninstallItems()))
	if p.previews != 1 || p.runs != 0 {
		t.Fatalf("after opening: previews=%d runs=%d, want 1 and 0", p.previews, p.runs)
	}
	if got := uninstallPhase(t, s); got != phasePreview {
		t.Fatalf("phase = %v, want preview", got)
	}
	text := viewText(s)
	for _, want := range []string{"! remove link", "~/.zshrc", "restores the previous file", "! cut block", "Nothing has changed yet"} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
}

func TestUninstallCountsLineMatchesTheItems(t *testing.T) {
	p := &uninstallProbe{}
	s := loadedUninstall(t, uninstallServices(p, uninstallItems()))
	want := "2 links removed · 1 previous file restored · 1 rules block cut"
	if text := viewText(s); !strings.Contains(text, want) {
		t.Errorf("counts line: want %q in:\n%s", want, text)
	}

	var many []uninstall.Item
	for i := range 57 {
		many = append(many, uninstall.Item{Path: fmt.Sprintf("/home/u/l%d", i), Kind: uninstall.KindRemoveLink, Restores: i < 3})
	}
	many = append(many, uninstall.Item{Path: "/home/u/.codex/AGENTS.md", Kind: uninstall.KindCutBlock})
	s = loadedUninstall(t, uninstallServices(p, many))
	want = "57 links removed · 3 previous files restored · 1 rules block cut"
	if text := viewText(s); !strings.Contains(text, want) {
		t.Errorf("counts line: want %q in:\n%s", want, text)
	}
}

func TestUninstallEmptyPreviewHasNoContinueKey(t *testing.T) {
	p := &uninstallProbe{}
	s := loadedUninstall(t, uninstallServices(p, nil))
	if text := viewText(s); !strings.Contains(text, "nothing to uninstall: no link points into this repo") {
		t.Errorf("empty preview:\n%s", text)
	}
	if km := s.keys(); len(km) != 0 {
		t.Errorf("empty preview has keys %v, want none (esc back only)", km)
	}
	next, cmd := s.update(key("enter"))
	if cmd != nil || uninstallPhase(t, next) != phasePreview {
		t.Errorf("enter on an empty preview moved on (cmd=%v)", cmd != nil)
	}
}

func TestUninstallConfirmOpensOnCancel(t *testing.T) {
	p := &uninstallProbe{}
	s := loadedUninstall(t, uninstallServices(p, uninstallItems()))
	s, _ = opStep(s, key("enter"))
	if got := uninstallPhase(t, s); got != phaseConfirm {
		t.Fatalf("enter: phase = %v, want confirm", got)
	}
	text := viewText(s)
	if !strings.Contains(text, "› Cancel") || !strings.Contains(text, "  Uninstall") {
		t.Errorf("confirm is not on Cancel:\n%s", text)
	}
	want := "Everything is recorded in ~/.aval-backup first · undo: aval restore latest"
	if got := s.view().status; got != want {
		t.Errorf("safety line = %q, want %q", got, want)
	}
	// Enter on Cancel runs nothing and returns to the preview.
	s, _ = opStep(s, key("enter"))
	if p.runs != 0 || uninstallPhase(t, s) != phasePreview {
		t.Errorf("cancel: runs=%d phase=%v, want 0 and preview", p.runs, uninstallPhase(t, s))
	}
}

func TestUninstallActionCallsUninstallOnce(t *testing.T) {
	p := &uninstallProbe{}
	s := loadedUninstall(t, uninstallServices(p, uninstallItems()))
	s, _ = opStep(s, key("enter"))
	s, _ = opStep(s, key("up"))
	s, _ = opStep(s, key("enter"))
	if p.runs != 1 {
		t.Fatalf("Uninstall called %d times, want 1", p.runs)
	}
	if p.previews != 1 {
		t.Errorf("preview called %d times, want 1", p.previews)
	}
	if got := uninstallPhase(t, s); got != phaseResult {
		t.Fatalf("phase = %v, want result", got)
	}
}

func TestUninstallResultNamesRestore(t *testing.T) {
	p := &uninstallProbe{}
	s := loadedUninstall(t, uninstallServices(p, uninstallItems()))
	s, _ = opStep(s, key("enter"))
	s, _ = opStep(s, key("up"))
	s, _ = opStep(s, key("enter"))
	if text := viewText(s); !strings.Contains(text, "aval restore latest") {
		t.Errorf("result does not name restore:\n%s", text)
	}
}

func TestUninstallRunErrorIsShown(t *testing.T) {
	svc := uninstallServices(&uninstallProbe{}, uninstallItems())
	svc.Uninstall = func() ([]uninstall.Item, error) { return nil, errors.New("disk full") }
	s := loadedUninstall(t, svc)
	s, _ = opStep(s, key("enter"))
	s, _ = opStep(s, key("up"))
	s, _ = opStep(s, key("enter"))
	if text := viewText(s); !strings.Contains(text, "disk full") {
		t.Errorf("result hides the error:\n%s", text)
	}
}

// opPhaseOf reads the phase of the operation inside the view's wrapper.
func opPhaseOf[P any](t *testing.T, s screen) opPhase {
	t.Helper()
	w, ok := s.(glyphSepScreen)
	if !ok {
		t.Fatalf("screen is %T, want glyphSepScreen", s)
	}
	o, ok := w.screen.(operation[P])
	if !ok {
		t.Fatalf("wrapped screen is %T, want the operation", w.screen)
	}
	return o.phase
}

func uninstallPhase(t *testing.T, s screen) opPhase { return opPhaseOf[[]uninstall.Item](t, s) }

func init() {
	goldenViews = append(goldenViews, struct {
		name  string
		build func(t *testing.T, unicode bool) tea.Model
	}{"uninstall", func(t *testing.T, unicode bool) tea.Model {
		return goldenOpViewApp(t, unicode, newUninstallView(uninstallServices(&uninstallProbe{}, uninstallItems())), false)
	}}, struct {
		name  string
		build func(t *testing.T, unicode bool) tea.Model
	}{"uninstall_confirm", func(t *testing.T, unicode bool) tea.Model {
		return goldenOpViewApp(t, unicode, newUninstallView(uninstallServices(&uninstallProbe{}, uninstallItems())), true)
	}}, struct {
		name  string
		build func(t *testing.T, unicode bool) tea.Model
	}{"uninstall_empty", func(t *testing.T, unicode bool) tea.Model {
		return goldenOpViewApp(t, unicode, newUninstallView(uninstallServices(&uninstallProbe{}, nil)), false)
	}})
}

// goldenOpViewApp is a view as the program runs it: an App with a fake hub at
// the bottom and the view on top, loaded; with enter set, one enter is sent.
func goldenOpViewApp(t *testing.T, unicode bool, s screen, enter bool) tea.Model {
	t.Helper()
	ch := chrome{theme: goldenTheme(t, unicode), version: "0.2.0", platform: "macOS (arm64)", width: 80, height: 40}
	s, _ = opStep(s, chromeMsg{ch})
	for _, m := range drainOp(s.init()) {
		s, _ = opStep(s, m)
	}
	if enter {
		s, _ = opStep(s, key("enter"))
	}
	a := newApp(ch, Services{}, newFake("hub"))
	a.stack = append(a.stack, s)
	return a
}
