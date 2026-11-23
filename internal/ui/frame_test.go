package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

// longListFixture has more groups than an 8-row terminal can show, with the
// cursor on the last one, so the select list must scroll to keep it visible.
func longListFixture(t *testing.T, unicode bool) Model {
	t.Helper()
	m := screenFixture(t, ScreenSelect, unicode)
	for i := 0; i < 10; i++ {
		m.plan.Groups = append(m.plan.Groups, plan.Group{
			ID: fmt.Sprintf("g%d", i), Label: fmt.Sprintf("Group number %d", i),
			Detail: "a detail line that takes a row", Actions: []plan.Action{{Label: "~/.x"}},
		})
	}
	m.cursor = len(m.plan.Groups) - 1
	return m
}

func TestFrameNeverExceedsTheHeight(t *testing.T) {
	for _, unicode := range []bool{true, false} {
		fixtures := map[string]Model{"long select": longListFixture(t, unicode)}
		for _, s := range goldenScreens {
			fixtures[s.name] = screenFixture(t, s.screen, unicode)
		}
		dry := screenFixture(t, ScreenSelect, unicode)
		dry.DryRun = true
		fixtures["dry select"] = dry

		for name, m := range fixtures {
			for _, w := range []int{36, 80} {
				for h := 8; h <= 40; h++ {
					lines := strings.Split(renderAtSize(m, w, h), "\n")
					if len(lines) > h {
						t.Errorf("%s unicode=%v %dx%d: frame is %d lines tall", name, unicode, w, h, len(lines))
					}
					for i, line := range lines {
						if lw := lipgloss.Width(line); lw > w {
							t.Errorf("%s unicode=%v %dx%d: line %d is %d wide", name, unicode, w, h, i, lw)
						}
					}
				}
			}
		}
	}
}

func TestScrolledListKeepsTheCursorVisible(t *testing.T) {
	for _, w := range []int{36, 80} {
		for h := 8; h <= 40; h++ {
			m := longListFixture(t, true)
			v := renderAtSize(m, w, h)
			if !strings.Contains(v, "Group number 9") {
				t.Errorf("%dx%d: the row under the cursor is not visible:\n%s", w, h, v)
			}
		}
	}
}

func TestEightRowsDropTheTaglineBeforeTheList(t *testing.T) {
	m := screenFixture(t, ScreenSelect, true)
	v := renderAtSize(m, 80, 8)
	if strings.Contains(v, wantTagline) {
		t.Errorf("an 8-row frame keeps the wantTagline:\n%s", v)
	}
	for _, want := range []string{"Terminal and shell", "Editor"} {
		if !strings.Contains(v, want) {
			t.Errorf("an 8-row frame dropped the list row %q before the wantTagline:\n%s", want, v)
		}
	}
}

func TestLockupShowsVersionAndPlatform(t *testing.T) {
	m := screenFixture(t, ScreenSelect, true)
	first := strings.Split(renderAt(m, 80), "\n")[0]
	if !strings.HasPrefix(first, "  aval  0.2.0") {
		t.Errorf("lockup = %q, want it to start with the margin, aval and the version", first)
	}
	if !strings.HasSuffix(first, "macOS (arm64)") {
		t.Errorf("lockup = %q, want the platform right-aligned", first)
	}
	if got := lipgloss.Width(first); got != 2+maxContent {
		t.Errorf("lockup is %d wide, want the margin plus the %d-column cap", got, maxContent)
	}
}

func TestTaglineIsDroppedUnderFortyColumns(t *testing.T) {
	m := screenFixture(t, ScreenSelect, true)
	if v := renderAt(m, 39); strings.Contains(v, wantTagline) {
		t.Errorf("39 columns keeps the wantTagline:\n%s", v)
	}
	if v := renderAt(m, 40); !strings.Contains(v, wantTagline) {
		t.Errorf("40 columns lost the wantTagline:\n%s", v)
	}
}

func TestPlatformLabel(t *testing.T) {
	for _, c := range []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "macOS (arm64)"},
		{"linux", "amd64", "Linux (amd64)"},
		{"windows", "amd64", "Windows (amd64)"},
		{"freebsd", "amd64", "freebsd (amd64)"},
	} {
		if got := PlatformLabel(c.goos, c.goarch); got != c.want {
			t.Errorf("PlatformLabel(%q, %q) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestNoEmDashInAnyView(t *testing.T) {
	for _, unicode := range []bool{true, false} {
		for _, s := range goldenScreens {
			for _, dry := range []bool{false, true} {
				for _, symlinks := range []bool{false, true} {
					m := screenFixture(t, s.screen, unicode)
					m.DryRun, m.Symlinks = dry, symlinks
					for _, w := range []int{36, 80, 120} {
						if v := renderAt(m, w); strings.Contains(v, "\u2014") {
							t.Errorf("%s unicode=%v dry=%v symlinks=%v width %d has an em dash:\n%s", s.name, unicode, dry, symlinks, w, v)
						}
					}
				}
			}
		}
	}
}

// candidateKeys is every key a user could plausibly press.
func candidateKeys() []string {
	keys := []string{" ", "enter", "esc", "up", "down", "left", "right", "tab", "ctrl+c", "?", "/"}
	for r := 'a'; r <= 'z'; r++ {
		keys = append(keys, string(r))
	}
	return keys
}

type snapshot struct {
	Screen   Screen
	Cursor   int
	Quitting bool
	Selected []bool
	Cmd      bool
}

func snap(m Model, cmdSet bool) snapshot {
	s := snapshot{Screen: m.screen, Cursor: m.cursor, Quitting: m.Quitting, Cmd: cmdSet}
	for _, g := range m.plan.Groups {
		s.Selected = append(s.Selected, g.Selected)
	}
	return s
}

func TestEveryHandledKeyIsInTheKeyMap(t *testing.T) {
	for _, s := range goldenScreens {
		for _, k := range candidateKeys() {
			m := screenFixture(t, s.screen, true)
			m.cursor = 1 // so both up and down can move
			before := snap(m, false)
			next, cmd := m.Update(key(k))
			after := snap(next.(Model), cmd != nil)
			_, mapped := m.keyMap().lookup(key(k).String())
			if !mapped && !reflect.DeepEqual(before, after) {
				t.Errorf("%s: key %q changes the model but is not in the key map", s.name, k)
			}
		}
	}
}

func TestEveryHintComesFromTheKeyMap(t *testing.T) {
	for _, s := range []Screen{ScreenSelect, ScreenDone} {
		m := screenFixture(t, s, true)
		km := m.keyMap()
		hints := km.hints(m.theme.Glyphs)
		if len(hints) == 0 {
			t.Fatalf("screen %v: the key map has no hints", s)
		}
		lines := strings.Split(renderAt(m, 120), "\n")
		last := lines[len(lines)-1]
		parts := strings.Split(strings.TrimSpace(last), " "+m.theme.Glyphs.Sep+" ")
		if len(parts) != len(hints) {
			t.Fatalf("screen %v: hint line %q has %d hints, key map has %d", s, last, len(parts), len(hints))
		}
		for i, part := range parts {
			want := hints[i].key + " " + hints[i].desc
			if part != want {
				t.Errorf("screen %v: hint %d = %q, want %q from the key map", s, i, part, want)
			}
		}
		for _, h := range hints {
			found := false
			for _, b := range km {
				if b.desc == h.desc {
					found = true
				}
			}
			if !found {
				t.Errorf("screen %v: hint %q has no binding", s, h.desc)
			}
		}
	}
}

const wantTagline = "Nothing passes without evidence."
