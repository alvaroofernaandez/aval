package generation_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

// A bare rollback goes back to the generation active before the current one,
// and a rollback is an activation, so two of them toggle (owner: G7 = A).
func TestPreviousFollowsTheActivationHistory(t *testing.T) {
	home := t.TempDir()
	if _, err := generation.Previous(home); !errors.Is(err, generation.ErrNoPrevious) {
		t.Errorf("Previous with no history = %v, want ErrNoPrevious", err)
	}
	a, _ := build(t, home, cfg("one"), t0)
	b, _ := build(t, home, cfg("two"), t0)
	steps := []struct{ activate, previous string }{{a.ID, ""}, {b.ID, a.ID}, {b.ID, a.ID}, {a.ID, b.ID}, {b.ID, a.ID}}
	for i, s := range steps {
		if err := generation.AppendHistory(home, s.activate, t0.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
		got, err := generation.Previous(home)
		if s.previous == "" {
			if !errors.Is(err, generation.ErrNoPrevious) {
				t.Errorf("step %d: Previous = %q, %v; want ErrNoPrevious", i, got, err)
			}
			continue
		}
		if got != s.previous || err != nil {
			t.Errorf("step %d: Previous = %q, %v; want %s", i, got, err, s.previous)
		}
	}
	b2, _ := os.ReadFile(filepath.Join(generation.Root(home), generation.HistoryFile))
	lines := strings.Split(strings.TrimSpace(string(b2)), "\n")
	if len(lines) != len(steps) || lines[1] != b.ID+" "+t0.Add(time.Hour).UTC().Format(time.RFC3339) {
		t.Errorf("history =\n%s\nwant one line per activation: <id> <RFC3339>", b2)
	}
}
