package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

func TestGenerationsPinAndUnpin(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	genRepo(t, repo, "a")
	id := buildID(t, repo, home)
	if out, err := runGenIn(t, repo, home, "generations", "pin", id); err != nil || !strings.Contains(out, "pinned "+id) || !generation.Pinned(home, id) {
		t.Fatalf("pin: %q, %v, Pinned = %v", out, err, generation.Pinned(home, id))
	}
	if out, err := runGenIn(t, repo, home, "generations", "unpin", id); err != nil || !strings.Contains(out, "unpinned "+id) || generation.Pinned(home, id) {
		t.Fatalf("unpin: %q, %v, Pinned = %v", out, err, generation.Pinned(home, id))
	}
	if _, err := runGenIn(t, repo, home, "generations", "pin", "gen-0000000000000000"); !errors.Is(err, generation.ErrUnknown) {
		t.Errorf("pin unknown = %v, want ErrUnknown", err)
	}
}

func TestGenerationsPruneReportsWhatItRemoved(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	genRepo(t, repo, "a")
	buildID(t, repo, home)
	if out, err := runGenIn(t, repo, home, "generations", "prune"); err != nil || !strings.Contains(out, "nothing to prune") {
		t.Errorf("prune of one generation = %q, %v", out, err)
	}
	for i := range generation.Retain {
		genRepo(t, repo, string(rune('b'+i)))
		buildID(t, repo, home)
	}
	out, err := runGenIn(t, repo, home, "generations", "prune")
	if err != nil || strings.Count(out, "removed gen-") != 1 {
		t.Errorf("prune = %q, %v; want one removed generation", out, err)
	}
}

// A Pi consumer boots from its own writable copy of the pi/ tree, linked to
// the owner's subscription login, and is pinned to the generation.
func TestGenerationsConsumerAppliesPiAndPins(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	genRepo(t, repo, "a")
	id := buildID(t, repo, home)
	login := filepath.Join(home, ".pi", "agent", "auth.json")
	if runtime.GOOS != "windows" { // a symlink needs a privilege there
		if err := os.MkdirAll(filepath.Dir(login), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(login, []byte(`{"token":"x"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runGenIn(t, repo, home, "generations", "consumer", "pen"); !errors.Is(err, generation.ErrNoActive) {
		t.Errorf("consumer with no id and nothing active = %v, want ErrNoActive", err)
	}
	out, err := runGenIn(t, repo, home, "generations", "consumer", "pen", id)
	if err != nil || !strings.Contains(out, id) {
		t.Fatalf("consumer: %q, %v", out, err)
	}
	pi := filepath.Join(generation.ConsumerDir(home, "pen"), "pi")
	if b, err := os.ReadFile(filepath.Join(pi, "AGENTS.md")); err != nil || len(b) == 0 {
		t.Errorf("pi/AGENTS.md: %v", err)
	}
	if runtime.GOOS != "windows" {
		if got, err := os.Readlink(filepath.Join(pi, "auth.json")); err != nil || got != login {
			t.Errorf("auth.json -> %q, %v; want %s", got, err, login)
		}
	}
	if pin, _ := generation.ConsumerPin(home, "pen"); pin != id {
		t.Errorf("pin = %q, want %s", pin, id)
	}
	if info, err := os.Stat(filepath.Join(pi, "AGENTS.md")); err != nil || info.Mode().Perm()&0o200 == 0 {
		t.Errorf("the consumer copy must be writable: %v %v", info, err)
	}
}
