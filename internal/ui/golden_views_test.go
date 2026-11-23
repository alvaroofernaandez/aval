package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// goldenViews is the hub and its views, each built as the tea.Model the
// program runs (an App with the view on top). It twins goldenScreens, whose
// table and testdata stay untouched: every view added here gets the same
// width, height, em-dash and key-map invariants, and goldens at 36, 80 and
// 120 columns in both glyph tiers, under testdata/view_<name>_<tier>_<w>.
// Fixtures use fake Services: no network and no real HOME.
var goldenViews = []struct {
	name  string
	build func(t *testing.T, unicode bool) tea.Model
}{
	{"confirm_destructive", func(t *testing.T, unicode bool) tea.Model { return goldenConfirmApp(t, unicode, confirmDestructive) }},
	{"confirm_inline", func(t *testing.T, unicode bool) tea.Model { return goldenConfirmApp(t, unicode, confirmInline) }},
}

// renderModelAt drives m with a window size and returns its View.
func renderModelAt(m tea.Model, width, height int) string {
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.View()
}

func TestHubViewsNeverExceedTheWidth(t *testing.T) {
	forEachLang(t, func(t *testing.T) {
		for _, unicode := range []bool{true, false} {
			for _, v := range goldenViews {
				m := v.build(t, unicode)
				for w := 20; w <= 120; w++ {
					for i, line := range strings.Split(renderModelAt(m, w, 40), "\n") {
						if got := lipgloss.Width(line); got > w {
							t.Errorf("%s unicode=%v width %d: line %d is %d wide: %q", v.name, unicode, w, i, got, line)
							break
						}
					}
				}
			}
		}
	})
}

func TestHubViewsNeverExceedTheHeight(t *testing.T) {
	forEachLang(t, func(t *testing.T) {
		for _, unicode := range []bool{true, false} {
			for _, v := range goldenViews {
				m := v.build(t, unicode)
				for _, w := range []int{36, 80} {
					for h := 8; h <= 40; h++ {
						lines := strings.Split(renderModelAt(m, w, h), "\n")
						if len(lines) > h {
							t.Errorf("%s unicode=%v %dx%d: frame is %d lines tall", v.name, unicode, w, h, len(lines))
						}
						for i, line := range lines {
							if lw := lipgloss.Width(line); lw > w {
								t.Errorf("%s unicode=%v %dx%d: line %d is %d wide", v.name, unicode, w, h, i, lw)
							}
						}
					}
				}
			}
		}
	})
}

func TestNoEmDashInAnyHubView(t *testing.T) {
	forEachLang(t, func(t *testing.T) {
		for _, unicode := range []bool{true, false} {
			for _, v := range goldenViews {
				m := v.build(t, unicode)
				for _, w := range []int{36, 80, 120} {
					if out := renderModelAt(m, w, 40); strings.Contains(out, "—") {
						t.Errorf("%s unicode=%v width %d has an em dash:\n%s", v.name, unicode, w, out)
					}
				}
			}
		}
	})
}

// TestEveryHubKeyIsInTheKeyMap: a key the merged map does not list changes
// nothing and returns no command, so the hints can never drift from what
// the router and the view handle.
func TestEveryHubKeyIsInTheKeyMap(t *testing.T) {
	forEachLang(t, func(t *testing.T) {
		for _, v := range goldenViews {
			for _, k := range candidateKeys() {
				m := v.build(t, true)
				a, ok := m.(App)
				if !ok {
					t.Fatalf("%s: build returned %T, want App", v.name, m)
				}
				if _, mapped := a.keys().lookup(key(k).String()); mapped {
					continue
				}
				next, cmd := a.Update(key(k))
				if cmd != nil || !reflect.DeepEqual(next.View(), a.View()) {
					t.Errorf("%s: key %q acts but is not in the key map", v.name, k)
				}
			}
		}
	})
}

// TestEveryHubHintComesFromTheKeyMap: the hint line is the merged map's
// hints, in order.
func TestEveryHubHintComesFromTheKeyMap(t *testing.T) {
	forEachLang(t, func(t *testing.T) {
		for _, v := range goldenViews {
			m := v.build(t, true)
			a, ok := m.(App)
			if !ok {
				t.Fatalf("%s: build returned %T, want App", v.name, m)
			}
			next, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			a = next.(App)
			var want []string
			for _, h := range a.keys().hints(a.chrome) {
				want = append(want, h.key+" "+h.desc)
			}
			sep := " " + a.theme.Glyphs.Sep + " "
			got := strings.Join(a.hintLines(a.layout(), true, a.keys()), sep)
			if strings.TrimSpace(got) != strings.Join(want, sep) {
				t.Errorf("%s: hint line %q, want %q from the key map", v.name, got, strings.Join(want, sep))
			}
			if out := a.View(); !strings.Contains(out, want[0]) {
				t.Errorf("%s: the view does not show the hint line:\n%s", v.name, out)
			}
		}
	})
}

func TestGoldenHubViews(t *testing.T) {
	forEachLang(t, func(t *testing.T) {
		for _, tier := range []struct {
			name    string
			unicode bool
		}{{"unicode", true}, {"ascii", false}} {
			for _, v := range goldenViews {
				for _, w := range []int{36, 80, 120} {
					name := goldenName(fmt.Sprintf("view_%s_%s_%d", v.name, tier.name, w))
					t.Run(name, func(t *testing.T) {
						assertGolden(t, name, renderModelAt(v.build(t, tier.unicode), w, 40))
					})
				}
			}
		}
	})
}
