package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func helpOpen(t *testing.T, s Screen, unicode bool) Model {
	t.Helper()
	return send(screenFixture(t, s, unicode), "?")
}

func TestQuestionMarkOpensHelpAndEscCloses(t *testing.T) {
	for _, s := range goldenScreens {
		m := screenFixture(t, s.screen, true)
		before := renderAt(m, 80)
		open := send(m, "?")
		if renderAt(open, 80) == before {
			t.Errorf("%s: ? did not change the view", s.name)
		}
		if !strings.Contains(renderAt(open, 80), "Keys") {
			t.Errorf("%s: help view has no Keys heading:\n%s", s.name, renderAt(open, 80))
		}
		if got := renderAt(send(open, "esc"), 80); got != before {
			t.Errorf("%s: esc did not restore the view", s.name)
		}
		if got := renderAt(send(open, "?"), 80); got != before {
			t.Errorf("%s: ? did not close the help", s.name)
		}
	}
}

func TestHelpListsEveryBindingOfTheScreen(t *testing.T) {
	for _, unicode := range []bool{true, false} {
		for _, s := range goldenScreens {
			m := screenFixture(t, s.screen, unicode)
			km := m.screenKeys()
			view := renderAtSize(send(m, "?"), 120, 60)
			for _, b := range km {
				if !strings.Contains(view, m.tr.T(b.desc, nil)) {
					t.Errorf("%s unicode=%v: help misses the description %q:\n%s", s.name, unicode, m.tr.T(b.desc, nil), view)
				}
				for _, k := range b.keys {
					want := k
					if k == " " {
						want = "space"
					}
					if k == "up" || k == "down" || k == "left" || k == "right" {
						continue // shown as the glyph set's arrows
					}
					if !strings.Contains(view, want) {
						t.Errorf("%s unicode=%v: help misses key %q:\n%s", s.name, unicode, want, view)
					}
				}
			}
		}
	}
}

func TestHelpUsesTheGlyphSetArrows(t *testing.T) {
	asc := renderAtSize(helpOpen(t, ScreenSelect, false), 120, 60)
	if !strings.Contains(asc, "Keys") || !strings.Contains(asc, "up") || strings.ContainsAny(asc, "↑↓←→") {
		t.Errorf("ascii help must say up/down, got:\n%s", asc)
	}
	uni := renderAtSize(helpOpen(t, ScreenSelect, true), 120, 60)
	if !strings.Contains(uni, "Keys") || !strings.Contains(uni, "↑") || !strings.Contains(uni, "↓") {
		t.Errorf("unicode help must show arrows, got:\n%s", uni)
	}
}

func TestHelpHasAHintOnEveryScreen(t *testing.T) {
	for _, s := range goldenScreens {
		m := screenFixture(t, s.screen, true)
		found := false
		for _, h := range m.screenKeys().hints(m.chrome) {
			if h.key == "?" && h.desc == "help" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no ? help hint in the key map", s.name)
		}
	}
}

func TestKeysAreInertWhileHelpIsOpen(t *testing.T) {
	for _, s := range goldenScreens {
		m := helpOpen(t, s.screen, true)
		m.cursor = 1
		base := snap(m, false)
		for _, k := range candidateKeys() {
			switch k {
			case "?", "esc", "up", "down", "k", "j":
				continue // close and scroll
			}
			next, cmd := m.Update(key(k))
			if got := snap(next.(Model), cmd != nil); fmt.Sprint(got) != fmt.Sprint(base) {
				t.Errorf("%s: key %q acted while help was open: %+v -> %+v", s.name, k, base, got)
			}
		}
	}
}

func TestCtrlCStillStopsAnInstallWhileHelpIsOpen(t *testing.T) {
	m := helpOpen(t, ScreenInstall, true)
	if got := send(m, "q"); got.stopping {
		t.Error("q acted on install while help was open")
	}
	if got := send(m, "ctrl+c"); !got.stopping {
		t.Error("ctrl+c did not stop the install while help was open")
	}
}

func TestHelpStaysInsideTheTerminal(t *testing.T) {
	forEachLang(t, func(t *testing.T) {
		for _, unicode := range []bool{true, false} {
			for _, s := range goldenScreens {
				m := helpOpen(t, s.screen, unicode)
				for _, w := range []int{20, 36, 80, 120} {
					for h := 8; h <= 40; h++ {
						lines := strings.Split(renderAtSize(m, w, h), "\n")
						if len(lines) > h {
							t.Errorf("%s unicode=%v %dx%d: help frame is %d lines", s.name, unicode, w, h, len(lines))
						}
						for i, line := range lines {
							if lw := lipgloss.Width(line); lw > w {
								t.Errorf("%s unicode=%v %dx%d: help line %d is %d wide", s.name, unicode, w, h, i, lw)
							}
						}
						if strings.Contains(strings.Join(lines, "\n"), "\u2014") {
							t.Errorf("%s: em dash in help", s.name)
						}
					}
				}
			}
		}
	})
}

func TestHelpScrollsWhenTallerThanTheTerminal(t *testing.T) {
	m := helpOpen(t, ScreenSelect, true)
	top := renderAtSize(m, 80, 8)
	if strings.Contains(top, "quit") {
		t.Skip("help fits at 8 rows")
	}
	for i := 0; i < 30; i++ {
		m = send(m, "down")
	}
	bottom := renderAtSize(m, 80, 8)
	if top == bottom || !strings.Contains(bottom, "quit") {
		t.Errorf("scrolling down never reached the last binding:\n%s", bottom)
	}
	for i := 0; i < 60; i++ {
		m = send(m, "up")
	}
	if got := renderAtSize(m, 80, 8); got != top {
		t.Errorf("scrolling back up did not return to the top:\n%s", got)
	}
}

func TestGoldenHelp(t *testing.T) {
	forEachLang(t, func(t *testing.T) {
		for _, tier := range []struct {
			name    string
			unicode bool
		}{{"unicode", true}, {"ascii", false}} {
			for _, w := range []int{36, 80} {
				name := goldenName(fmt.Sprintf("help_select_%s_%d", tier.name, w))
				t.Run(name, func(t *testing.T) {
					assertGolden(t, name, renderAt(helpOpen(t, ScreenSelect, tier.unicode), w))
				})
			}
		}
	})
}
