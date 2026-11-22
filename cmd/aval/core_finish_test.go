package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/repoloc"
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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
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

// install records where the repository is, so a release binary outside the
// checkout finds it from any directory afterwards. A dry run records nothing.
func TestFinishInstallRecordsRepo(t *testing.T) {
	for _, dry := range []bool{false, true} {
		repo, home := t.TempDir(), t.TempDir()
		cfg := filepath.Join(home, "cfg")
		t.Setenv("XDG_CONFIG_HOME", cfg)
		writeFile(t, filepath.Join(repo, "config/claude/settings.json"), `{"hooks":{}}`+"\n")

		if err := finishInstall(repo, home, dry); err != nil {
			t.Fatal(err)
		}
		got, err := repoloc.Load(filepath.Join(cfg, "aval", "config"))
		if err != nil {
			t.Fatal(err)
		}
		want := repo
		if dry {
			want = ""
		}
		if got != want {
			t.Errorf("dry=%v: recorded repo = %q, want %q", dry, got, want)
		}
	}
}
