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
		tr:   testTr,
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

// longUninstallItems is n links, named so each row is distinct.
func longUninstallItems(n int) []uninstall.Item {
	items := make([]uninstall.Item, n)
	for i := range items {
		items[i] = uninstall.Item{Path: fmt.Sprintf("/home/u/link%02d", i), Kind: uninstall.KindRemoveLink}
	}
	return items
}

// firstRow is the path of the first item row on screen ("" when none).
func firstRow(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, "~/link"); i >= 0 && strings.Contains(line, "remove link") {
			return strings.Fields(line[i:])[0]
		}
	}
	return ""
}

func TestUninstallPreviewScrollsALongList(t *testing.T) {
	m := goldenOpViewApp(t, true, newUninstallView(uninstallServices(&uninstallProbe{}, longUninstallItems(60))), false)
	out := renderModelAt(m, 80, 24)
	lines := strings.Split(out, "\n")
	if len(lines) > 24 {
		t.Errorf("frame is %d lines tall at 24 rows:\n%s", len(lines), out)
	}
	for _, want := range []string{"Uninstall", "60 links removed", "Nothing has changed yet", "↑↓ scroll"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview lacks %q:\n%s", want, out)
		}
	}
	rows := strings.Count(out, "remove link")
	if want := fmt.Sprintf("%d more below", 60-rows); !strings.Contains(out, want) {
		t.Errorf("want %q (60 items, %d visible):\n%s", want, rows, out)
	}
	before := firstRow(out)
	next, _ := m.Update(key("down"))
	after := firstRow(renderModelAt(next, 80, 24))
	if before == "" || after == "" || before == after {
		t.Errorf("scroll did not move the first visible row: %q then %q", before, after)
	}
	// k and j scroll too; scrolling back up returns to the top.
	next, _ = next.Update(key("up"))
	if got := firstRow(renderModelAt(next, 80, 24)); got != before {
		t.Errorf("up: first row %q, want %q", got, before)
	}
}

func TestUninstallScrollsToTheLastRow(t *testing.T) {
	var m tea.Model = goldenOpViewApp(t, true, newUninstallView(uninstallServices(&uninstallProbe{}, longUninstallItems(60))), false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for range 100 {
		m, _ = m.Update(key("down"))
	}
	out := renderModelAt(m, 80, 24)
	if !strings.Contains(out, "~/link59") || strings.Contains(out, "more below") {
		t.Errorf("end of the list:\n%s", out)
	}
}

func TestUninstallShortListHasNoScrollHint(t *testing.T) {
	m := goldenOpViewApp(t, true, newUninstallView(uninstallServices(&uninstallProbe{}, uninstallItems())), false)
	out := renderModelAt(m, 80, 24)
	if strings.Contains(out, "scroll") || strings.Contains(out, "more below") {
		t.Errorf("a list that fits shows scroll cues:\n%s", out)
	}
}

// At 36 columns the safety line used to end in an ellipsis; it wraps instead
// so the safety net is always named in full.
func TestUninstallConfirmWrapsTheSafetyLineAt36(t *testing.T) {
	for _, unicode := range []bool{true, false} {
		for _, h := range []int{24, 40} {
			m := goldenOpViewApp(t, unicode, newUninstallView(uninstallServices(&uninstallProbe{}, uninstallItems())), true)
			out := renderModelAt(m, 36, h)
			if n := len(strings.Split(out, "\n")); n > h {
				t.Errorf("unicode=%v 36x%d: frame is %d lines tall", unicode, h, n)
			}
			if strings.ContainsAny(out, "…") || strings.Contains(out, "...") {
				t.Errorf("unicode=%v 36x%d: text is truncated:\n%s", unicode, h, out)
			}
			words := strings.Join(strings.Fields(out), " ")
			sep := "·"
			if !unicode {
				sep = "-"
			}
			want := "Everything is recorded in ~/.aval-backup first " + sep + " undo: aval restore latest"
			if !unicode {
				want = strings.ReplaceAll(want, " - ", " ")
				words = strings.Join(strings.Fields(strings.ReplaceAll(words, " - ", " ")), " ")
			}
			if !strings.Contains(words, want) {
				t.Errorf("unicode=%v 36x%d: safety sentence is not whole:\n%s", unicode, h, out)
			}
		}
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
	}{"uninstall_long", func(t *testing.T, unicode bool) tea.Model {
		return goldenOpViewApp(t, unicode, newUninstallView(uninstallServices(&uninstallProbe{}, longUninstallItems(60))), false)
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
	ch := chrome{tr: testTr, theme: goldenTheme(t, unicode), version: "0.2.0", platform: "macOS (arm64)", width: 80, height: 40}
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
