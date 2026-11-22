package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/deck"
)

func fakeDeckDeps(opened *string) deckDeps {
	return deckDeps{
		browser: func() (string, error) { return "/fake/chrome", nil },
		run: func(name string, args ...string) ([]byte, error) {
			for _, a := range args {
				if a == "--dump-dom" {
					return []byte(`<html data-overflow="">`), nil
				}
				if p, ok := strings.CutPrefix(a, "--print-to-pdf="); ok {
					return nil, os.WriteFile(p, []byte("%PDF-1.7"), 0o644)
				}
			}
			return nil, nil
		},
		open: func(p string) error { *opened = p; return nil },
	}
}

const tinySpec = `{"title":"Plan","lang":"es","slides":[{"layout":"cover","title":"Un plan"},{"layout":"closing","title":"Fin"}]}`

func TestRunDeckBuild(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "work", "plan.json")
	writeFile(t, spec, tinySpec)
	pdf := filepath.Join(dir, "decks", "plan.pdf")

	var opened string
	var out bytes.Buffer
	if err := runDeck([]string{"build", spec, "-o", pdf, "--open"}, &out, fakeDeckDeps(&opened)); err != nil {
		t.Fatalf("build: %v", err)
	}
	if b, err := os.ReadFile(pdf); err != nil || !strings.HasPrefix(string(b), "%PDF") {
		t.Fatalf("no PDF: %v", err)
	}
	// The spec travels with the PDF so the deck can be rebuilt or edited later.
	if b, err := os.ReadFile(filepath.Join(dir, "decks", "plan.deck.json")); err != nil || !strings.Contains(string(b), "Un plan") {
		t.Fatalf("spec not kept beside the PDF: %v", err)
	}
	if opened != pdf {
		t.Errorf("--open opened %q, want %q", opened, pdf)
	}
	if !strings.Contains(out.String(), pdf) {
		t.Errorf("output does not name the PDF: %s", out.String())
	}
}

func TestRunDeckBuildNeedsOutput(t *testing.T) {
	spec := filepath.Join(t.TempDir(), "plan.json")
	writeFile(t, spec, tinySpec)
	var opened string
	err := runDeck([]string{"build", spec}, &bytes.Buffer{}, fakeDeckDeps(&opened))
	if err == nil || !strings.Contains(err.Error(), "-o") {
		t.Fatalf("want an error asking for -o, got %v", err)
	}
	err = runDeck([]string{"build", spec, "-o", filepath.Join(t.TempDir(), "x.png")}, &bytes.Buffer{}, fakeDeckDeps(&opened))
	if err == nil || !strings.Contains(err.Error(), ".pdf") {
		t.Fatalf("want an error asking for a .pdf, got %v", err)
	}
}

func TestRunDeckCheck(t *testing.T) {
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good.json"), filepath.Join(dir, "bad.json")
	writeFile(t, good, tinySpec)
	writeFile(t, bad, `{"title":"x","slides":[{"layout":"cards","title":"t"}]}`)
	var opened string
	if err := runDeck([]string{"check", good}, &bytes.Buffer{}, fakeDeckDeps(&opened)); err != nil {
		t.Fatalf("check good: %v", err)
	}
	if err := runDeck([]string{"check", bad}, &bytes.Buffer{}, fakeDeckDeps(&opened)); err == nil || !strings.Contains(err.Error(), "slide 1") {
		t.Fatalf("check bad: %v", err)
	}
}

// The example is what agents copy: it must be valid and use every layout.
func TestRunDeckExampleIsValid(t *testing.T) {
	var out bytes.Buffer
	var opened string
	if err := runDeck([]string{"example"}, &out, fakeDeckDeps(&opened)); err != nil {
		t.Fatal(err)
	}
	d, err := deck.Parse(strings.NewReader(out.String()))
	if err != nil {
		t.Fatalf("the example does not validate: %v", err)
	}
	used := map[string]bool{}
	for _, s := range d.Slides {
		used[s.Layout] = true
	}
	for _, l := range deck.Layouts() {
		if !used[l] {
			t.Errorf("the example never uses layout %q", l)
		}
	}
}

func TestRunDeckUsage(t *testing.T) {
	var opened string
	for _, args := range [][]string{nil, {"nope"}, {"build"}, {"check"}} {
		if err := runDeck(args, &bytes.Buffer{}, fakeDeckDeps(&opened)); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("%v: want a usage error, got %v", args, err)
		}
	}
}

// --plan records the deck for the plan, which is what unlocks the plan gate.
func TestRunDeckBuildPlanUnlocksTheGate(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(repo, "odd", "tasks", "f.md")
	writeFile(t, plan, "## T1\n- [ ] RED\n")
	spec := filepath.Join(repo, "odd", "f.json")
	writeFile(t, spec, tinySpec)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	payload := `{"tool_name":"Edit","cwd":"` + repo + `","tool_input":{"file_path":"` + filepath.Join(repo, "main.go") + `"}}`
	gate := func() string {
		var out bytes.Buffer
		deck.Gate(strings.NewReader(payload), &out, deck.GateOptions{Cache: cache})
		return out.String()
	}
	if gate() == "" {
		t.Fatal("precondition: the gate must block before the deck")
	}
	var opened string
	if err := runDeck([]string{"build", spec, "-o", filepath.Join(t.TempDir(), "f.pdf"), "--plan", plan}, &bytes.Buffer{}, fakeDeckDeps(&opened)); err != nil {
		t.Fatal(err)
	}
	if g := gate(); g != "" {
		t.Errorf("after build --plan the gate must allow, got %s", g)
	}
}
