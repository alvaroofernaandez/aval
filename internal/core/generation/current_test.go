package generation_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

func TestNoGenerationIsActiveAtFirst(t *testing.T) {
	if id, err := generation.Current(tempHome(t)); id != "" || err != nil {
		t.Errorf("Current = %q, %v; want none", id, err)
	}
}

func TestSetCurrentSwitchesTheActiveGeneration(t *testing.T) {
	home := tempHome(t)
	a, _ := build(t, home, cfg("one"), t0)
	b, _ := build(t, home, cfg("two"), t0.Add(time.Hour))
	for _, id := range []string{a.ID, b.ID, a.ID} {
		if err := generation.SetCurrent(home, id); err != nil {
			t.Fatal(err)
		}
		if got, err := generation.Current(home); got != id || err != nil {
			t.Fatalf("Current = %q, %v; want %s", got, err, id)
		}
	}
	if runtime.GOOS != "windows" { // a symlink where the OS allows one (owner: G4 = A)
		if dest, err := os.Readlink(filepath.Join(generation.Root(home), generation.CurrentName)); err != nil || dest != a.ID {
			t.Errorf("current -> %q, %v; want a relative link to %s", dest, err, a.ID)
		}
	}
	if err := generation.SetCurrent(home, "gen-0000000000000000"); !errors.Is(err, generation.ErrUnknown) {
		t.Errorf("SetCurrent(unknown) = %v, want ErrUnknown", err)
	}
	if got, _ := generation.Current(home); got != a.ID {
		t.Errorf("a refused switch moved current to %q", got)
	}
}

// A switch that died before its rename leaves the old generation active, and
// the next switch is not blocked by its leftovers.
func TestAnInterruptedSwitchLeavesTheOldGenerationActive(t *testing.T) {
	home := tempHome(t)
	a, _ := build(t, home, cfg("one"), t0)
	b, _ := build(t, home, cfg("two"), t0)
	if err := generation.SetCurrent(home, a.ID); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(generation.Root(home), ".current-crashed")
	if err := os.WriteFile(leftover, []byte(b.ID), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := generation.Current(home); got != a.ID {
		t.Fatalf("Current = %q after a crashed switch, want %s", got, a.ID)
	}
	if err := generation.SetCurrent(home, b.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := generation.Current(home); got != b.ID {
		t.Errorf("Current = %q, want %s", got, b.ID)
	}
	gs, _ := generation.List(home)
	if len(gs) != 2 {
		t.Errorf("list = %d generations, want 2 (pointer and leftovers are not generations)", len(gs))
	}
}

func TestActiveDirIsTheCurrentGenerationsAdapterTree(t *testing.T) {
	home := tempHome(t)
	if d, err := generation.ActiveDir(home, "fake"); d != "" || err != nil {
		t.Errorf("ActiveDir with none active = %q, %v; want empty", d, err)
	}
	a, _ := build(t, home, cfg("one"), t0)
	if err := generation.SetCurrent(home, a.ID); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(generation.Root(home), a.ID, "fake")
	if d, err := generation.ActiveDir(home, "fake"); d != want || err != nil {
		t.Errorf("ActiveDir = %q, %v; want %s", d, err, want)
	}
	if d := generation.Dir(home, a.ID, "fake"); d != want {
		t.Errorf("Dir = %q, want %s", d, want)
	}
}
