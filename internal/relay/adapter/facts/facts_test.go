package facts

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRepo_BranchFromHEAD_FromASubdirectory(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/feat/upload-retry\n")
	sub := filepath.Join(root, "internal", "upload")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := Repo(sub)
	if got == nil || got.Branch != "feat/upload-retry" {
		t.Fatalf("got %+v, want branch feat/upload-retry", got)
	}
}

func TestRepo_DetachedHEADShowsTheShortCommit(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "90cb33f1234567890abcdef1234567890abcdef0\n")
	if got := Repo(root); got == nil || got.Branch != "detached at 90cb33f" {
		t.Fatalf("got %+v", got)
	}
}

func TestRepo_WorktreeGitFile(t *testing.T) {
	root := t.TempDir()
	gitdir := filepath.Join(root, "main", ".git", "worktrees", "wt")
	write(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/wt-branch\n")
	write(t, filepath.Join(root, "wt", ".git"), "gitdir: "+gitdir+"\n")
	if got := Repo(filepath.Join(root, "wt")); got == nil || got.Branch != "wt-branch" {
		t.Fatalf("got %+v", got)
	}
}

func TestRepo_NoRepository(t *testing.T) {
	if got := Repo(t.TempDir()); got != nil {
		t.Fatalf("got %+v, want nil outside a repository", got)
	}
}

func TestRoot(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Root(sub); got != root {
		t.Fatalf("Root(%s) = %q, want %q", sub, got, root)
	}
	if got := Root(t.TempDir()); got != "" {
		t.Fatalf("outside a repository: %q, want empty", got)
	}
}
