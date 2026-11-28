package generation_test

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

func buildN(t *testing.T, home string, n int) []string {
	t.Helper()
	var ids []string
	for i := range n {
		g, _ := build(t, home, cfg(fmt.Sprintf("rule %d", i)), t0.Add(time.Duration(i)*time.Hour))
		ids = append(ids, g.ID)
	}
	return ids // oldest first
}

func exists(home, id string) bool {
	_, err := os.Stat(generation.Dir(home, id, "fake"))
	return err == nil
}

// Retention follows backup: the newest Retain unpinned generations stay, the
// active one, the one before it and every pinned one are never removed.
func TestPruneKeepsTheNewestRetainAndWhatIsProtected(t *testing.T) {
	home := t.TempDir()
	ids := buildN(t, home, generation.Retain+4)
	oldest, second, third := ids[0], ids[1], ids[2]
	if err := generation.Pin(home, oldest, true); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{second, third} {
		if err := generation.AppendHistory(home, id, t0); err != nil {
			t.Fatal(err)
		}
	}
	if err := generation.SetCurrent(home, third); err != nil {
		t.Fatal(err)
	}
	removed, err := generation.Prune(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{oldest, second, third} {
		if !exists(home, id) {
			t.Errorf("%s was pruned but is pinned, previous or active", id)
		}
	}
	for _, id := range ids[3:4] {
		if exists(home, id) {
			t.Errorf("%s survived, it is older than the newest %d unpinned", id, generation.Retain)
		}
		if !slices.Contains(removed, id) {
			t.Errorf("removed = %v, want it to list %s", removed, id)
		}
	}
	for _, id := range ids[4:] {
		if !exists(home, id) {
			t.Errorf("%s was pruned, it is among the newest %d", id, generation.Retain)
		}
	}
}

func TestPinAndUnpinAreVisibleAndIdempotent(t *testing.T) {
	home := t.TempDir()
	id := buildN(t, home, 1)[0]
	for range 2 {
		if err := generation.Pin(home, id, true); err != nil {
			t.Fatal(err)
		}
	}
	if !generation.Pinned(home, id) {
		t.Error("Pinned = false after Pin")
	}
	if err := generation.Pin(home, id, false); err != nil || generation.Pinned(home, id) {
		t.Errorf("unpin: %v, Pinned = %v", err, generation.Pinned(home, id))
	}
	if err := generation.Pin(home, "gen-0000000000000000", true); !errors.Is(err, generation.ErrUnknown) {
		t.Errorf("pin of an unknown generation = %v, want ErrUnknown", err)
	}
}
