package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/update"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// upstreamPair returns a clone ("repo") that is one commit behind its origin,
// where that commit edits a managed file and adds a manifest entry.
func upstreamPair(t *testing.T) (repo, home string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	git(t, root, "init", "-q", "--bare", "-b", "main", origin)

	seed := filepath.Join(root, "seed")
	git(t, root, "clone", "-q", origin, seed)
	writeFile(t, filepath.Join(seed, "install.manifest"),
		"[claude]\nlabel = c\ndetail = d\nplatforms = all\n\nconfig/claude/agents .claude/agents\n")
	writeFile(t, filepath.Join(seed, "config/claude/agents/a.md"), "v1\n")
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-qm", "init")
	git(t, seed, "push", "-q", "origin", "HEAD:main")

	repo = filepath.Join(root, "repo")
	git(t, root, "clone", "-q", origin, repo)

	writeFile(t, filepath.Join(seed, "install.manifest"),
		"[claude]\nlabel = c\ndetail = d\nplatforms = all\n\nconfig/claude/agents .claude/agents\nconfig/claude/hooks .claude/hooks\n")
	writeFile(t, filepath.Join(seed, "config/claude/agents/a.md"), "v2\n")
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-qm", "next")
	git(t, seed, "push", "-q", "origin", "HEAD:main")

	return repo, t.TempDir()
}

func TestUpdateDryRunPreviewsWithoutChangingAnything(t *testing.T) {
	repo, home := upstreamPair(t)
	before := git(t, repo, "rev-parse", "HEAD")

	var out bytes.Buffer
	if err := runUpdate(repo, home, true, false, &out, io.Discard, englishTr); err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, out.String())
	}
	if after := git(t, repo, "rev-parse", "HEAD"); after != before {
		t.Fatalf("dry run moved HEAD: %s -> %s", before, after)
	}
	got, _ := os.ReadFile(filepath.Join(repo, "config/claude/agents/a.md"))
	if string(got) != "v1\n" {
		t.Errorf("dry run changed a managed file: %q", got)
	}
	for _, want := range []string{"config/claude/agents/a.md", "config/claude/hooks", "Dry run"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output misses %q:\n%s", want, out.String())
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude")); err == nil {
		t.Error("dry run touched HOME")
	}
}

func TestUpdateDryRunRefusesADirtyTree(t *testing.T) {
	repo, home := upstreamPair(t)
	writeFile(t, filepath.Join(repo, "config/claude/agents/a.md"), "local edit\n")
	var out bytes.Buffer
	err := runUpdate(repo, home, true, false, &out, io.Discard, englishTr)
	if !errors.Is(err, update.ErrDirtyTree) {
		t.Errorf("err = %v, want update.ErrDirtyTree", err)
	}
}

// The preview speaks the resolved language; the data in it (paths, entries,
// versions) is the same.
func TestUpdateDryRunSpeaksSpanish(t *testing.T) {
	repo, home := upstreamPair(t)
	es := i18n.New(i18n.Spanish)
	var out bytes.Buffer
	if err := runUpdate(repo, home, true, false, &out, io.Discard, es); err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{es.T("cli.update.paths", nil), es.T("cli.update.entry", i18n.Args{"entry": "config/claude/hooks"}), es.T("cli.update.dry_run", nil)} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Dry run") {
		t.Errorf("English left in the Spanish preview:\n%s", out.String())
	}
}
