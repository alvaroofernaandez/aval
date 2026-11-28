package generation_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/fake"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			b, _ := os.ReadFile(p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

// A consumer gets the adapter's tree in a writable dir of its own, so it can
// keep sessions there; switching generation drops what the new one no longer
// renders, and nothing else.
func TestApplyConsumerWritesAWritableCopyAndPins(t *testing.T) {
	home := t.TempDir()
	a, _ := build(t, home, cfg("one"), t0)
	target, err := generation.ApplyConsumer(home, "pen", a.ID, fake.New())
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(generation.ConsumerDir(home, "pen"), "fake"); target != want {
		t.Errorf("target = %s, want %s", target, want)
	}
	got := tree(t, target)
	if len(got) == 0 {
		t.Fatal("nothing applied")
	}
	if id, err := generation.ConsumerPin(home, "pen"); id != a.ID || err != nil {
		t.Errorf("pin = %q, %v; want %s", id, err, a.ID)
	}
	if err := os.WriteFile(filepath.Join(target, "session.log"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := build(t, home, cfg("two"), t0.Add(time.Hour))
	if _, err := generation.ApplyConsumer(home, "pen", b.ID, fake.New()); err != nil {
		t.Fatal(err)
	}
	after := tree(t, target)
	if after["session.log"] != "mine" {
		t.Error("the consumer's own file was removed")
	}
	delete(after, "session.log")
	if want := tree(t, generation.Dir(home, b.ID, "fake")); len(after) != len(want) {
		t.Errorf("tree = %v, want exactly generation %s: %v", after, b.ID, want)
	}
	if id, _ := generation.ConsumerPin(home, "pen"); id != b.ID {
		t.Errorf("pin = %s, want %s", id, b.ID)
	}
}

func TestApplyConsumerRefusesBadInput(t *testing.T) {
	home := t.TempDir()
	g, _ := build(t, home, cfg("one"), t0)
	for _, name := range []string{"", "..", "a/b", `a\b`, "."} {
		if _, err := generation.ApplyConsumer(home, name, g.ID, fake.New()); !errors.Is(err, generation.ErrConsumerName) {
			t.Errorf("name %q = %v, want ErrConsumerName", name, err)
		}
	}
	if _, err := generation.ApplyConsumer(home, "pen", "gen-0000000000000000", fake.New()); !errors.Is(err, generation.ErrUnknown) {
		t.Errorf("unknown generation = %v, want ErrUnknown", err)
	}
	if _, err := os.Stat(generation.ConsumerDir(home, "pen")); err == nil {
		t.Error("a refused apply left a consumer dir")
	}
}

// A consumer's pinned generation survives retention however old it is.
func TestPruneKeepsWhatAConsumerIsPinnedTo(t *testing.T) {
	home := t.TempDir()
	ids := buildN(t, home, generation.Retain+2)
	if _, err := generation.ApplyConsumer(home, "pen", ids[0], fake.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := generation.Prune(home); err != nil {
		t.Fatal(err)
	}
	if !exists(home, ids[0]) {
		t.Error("the generation Pen is pinned to was pruned")
	}
	if exists(home, ids[1]) {
		t.Error("an unprotected old generation survived")
	}
}
