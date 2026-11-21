package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func hooksPath(dir string) string {
	out, _ := exec.Command("git", "-C", dir, "config", "--get", "core.hooksPath").Output()
	return strings.TrimSpace(string(out))
}

// The installer turns the versioned git hooks on, as install.sh did.
func TestConfigureHooksPath(t *testing.T) {
	t.Run("git checkout with .githooks", func(t *testing.T) {
		repo := t.TempDir()
		gitIn(t, repo, "init", "-q")
		os.MkdirAll(filepath.Join(repo, ".githooks"), 0o755)
		if err := configureHooksPath(repo, false); err != nil {
			t.Fatal(err)
		}
		if got := hooksPath(repo); got != ".githooks" {
			t.Fatalf("core.hooksPath = %q", got)
		}
	})
	t.Run("dry run changes nothing", func(t *testing.T) {
		repo := t.TempDir()
		gitIn(t, repo, "init", "-q")
		os.MkdirAll(filepath.Join(repo, ".githooks"), 0o755)
		if err := configureHooksPath(repo, true); err != nil {
			t.Fatal(err)
		}
		if got := hooksPath(repo); got != "" {
			t.Fatalf("dry run set core.hooksPath = %q", got)
		}
	})
	t.Run("not a git checkout is skipped", func(t *testing.T) {
		repo := t.TempDir()
		os.MkdirAll(filepath.Join(repo, ".githooks"), 0o755)
		if err := configureHooksPath(repo, false); err != nil {
			t.Fatalf("a plain directory must be skipped silently: %v", err)
		}
	})
	t.Run("no .githooks directory is skipped", func(t *testing.T) {
		repo := t.TempDir()
		gitIn(t, repo, "init", "-q")
		if err := configureHooksPath(repo, false); err != nil {
			t.Fatal(err)
		}
		if got := hooksPath(repo); got != "" {
			t.Fatalf("core.hooksPath = %q", got)
		}
	})
}

// finishInstall seeds the settings, merges the repo-managed keys, and enables
// the hooks, in that order; a failing merge never fails the install.
func TestFinishInstall(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	gitIn(t, repo, "init", "-q")
	os.MkdirAll(filepath.Join(repo, ".githooks"), 0o755)
	writeFile(t, filepath.Join(repo, "config/claude/settings.json"), `{"hooks":{}}`+"\n")

	if err := finishInstall(repo, home, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude/settings.json")); err != nil {
		t.Fatalf("settings not seeded: %v", err)
	}
	if got := hooksPath(repo); got != ".githooks" {
		t.Fatalf("core.hooksPath = %q", got)
	}
}
