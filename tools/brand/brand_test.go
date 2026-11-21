package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoDocs(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := findRepoRoot(wd)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "docs")
}

func TestGoldenMatchesCommittedAssets(t *testing.T) {
	docs := repoDocs(t)
	out := t.TempDir()
	var so, se bytes.Buffer
	if err := run([]string{"--docs", docs, "--out", out}, &so, &se); err != nil {
		t.Fatal(err)
	}
	gen, _ := filepath.Glob(filepath.Join(out, "*.svg"))
	if len(gen) != 10 {
		t.Fatalf("want 10 generated svgs, got %d", len(gen))
	}
	for _, g := range gen {
		want, err := os.ReadFile(filepath.Join(docs, "assets", filepath.Base(g)))
		if err != nil {
			t.Errorf("%s not committed: %v", filepath.Base(g), err)
			continue
		}
		got, _ := os.ReadFile(g)
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from committed asset", filepath.Base(g))
		}
	}
}

func TestStaleVersionsAreRemoved(t *testing.T) {
	docs := repoDocs(t)
	out := t.TempDir()
	stale := filepath.Join(out, "hero-light.deadbeef.svg")
	plain := filepath.Join(out, "mark-dark.svg")
	os.WriteFile(stale, []byte("x"), 0o644)
	os.WriteFile(plain, []byte("x"), 0o644)
	var so, se bytes.Buffer
	if err := run([]string{"--docs", docs, "--out", out}, &so, &se); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{stale, plain} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s should have been removed", p)
		}
	}
}

func TestList(t *testing.T) {
	var so, se bytes.Buffer
	if err := run([]string{"--docs", repoDocs(t), "--list"}, &so, &se); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(so.String(), "\n"), "\n")
	if len(lines) != 33 {
		t.Fatalf("want 33 tools, got %d", len(lines))
	}
	if lines[0] != "Ghostty" || lines[len(lines)-1] != "Windows" {
		t.Errorf("unexpected order: %q ... %q", lines[0], lines[len(lines)-1])
	}
}

func TestNoLogo(t *testing.T) {
	var so, se bytes.Buffer
	if err := run([]string{"--docs", repoDocs(t), "--no-logo"}, &so, &se); err != nil {
		t.Fatal(err)
	}
	if so.String() != "ripgrep\ndelta\n" {
		t.Errorf("got %q", so.String())
	}
}

func TestFindRepoRoot(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644)
	sub := filepath.Join(root, "a", "b")
	os.MkdirAll(sub, 0o755)
	got, err := findRepoRoot(sub)
	if err != nil || got != root {
		t.Errorf("got %q, %v; want %q", got, err, root)
	}
}
