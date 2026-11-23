package update

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", name)
	gitIn(t, dir, "commit", "-m", name)
}

// cloneWithUpstreamAhead returns a clone whose already-fetched upstream is
// two commits ahead of HEAD.
func cloneWithUpstreamAhead(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	gitIn(t, root, "init", "--bare", "-b", "main", bare)
	a := filepath.Join(root, "a")
	gitIn(t, root, "clone", bare, a)
	gitIn(t, a, "checkout", "-b", "main")
	commitFile(t, a, "one")
	gitIn(t, a, "push", "-u", "origin", "main")
	b := filepath.Join(root, "b")
	gitIn(t, root, "clone", bare, b)
	commitFile(t, b, "two")
	commitFile(t, b, "three")
	gitIn(t, b, "push", "origin", "main")
	gitIn(t, a, "fetch", "origin")
	return a
}

func hashGitDir(t *testing.T, repo string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(filepath.Join(repo, ".git"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %v %d %d\n", p, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if d.Type().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			h.Write(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestLocalStatusCountsFetchedCommitsBehind(t *testing.T) {
	repo := cloneWithUpstreamAhead(t)
	behind, known, dirty := LocalStatus(context.Background(), NewGit(repo))
	if behind != 2 || !known || dirty {
		t.Errorf("got behind=%d known=%v dirty=%v, want 2 true false", behind, known, dirty)
	}
}

func TestLocalStatusUnknownWithoutUpstream(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	commitFile(t, repo, "one")
	behind, known, _ := LocalStatus(context.Background(), NewGit(repo))
	if known || behind != 0 {
		t.Errorf("got behind=%d known=%v, want 0 false", behind, known)
	}
}

func TestLocalStatusDirtyOnModifiedOrUntrackedFile(t *testing.T) {
	for name, edit := range map[string]func(dir string) error{
		"untracked": func(d string) error { return os.WriteFile(filepath.Join(d, "new"), []byte("x"), 0o644) },
		"modified":  func(d string) error { return os.WriteFile(filepath.Join(d, "one"), []byte("changed"), 0o644) },
	} {
		t.Run(name, func(t *testing.T) {
			repo := cloneWithUpstreamAhead(t)
			if err := edit(repo); err != nil {
				t.Fatal(err)
			}
			_, _, dirty := LocalStatus(context.Background(), NewGit(repo))
			if !dirty {
				t.Error("dirty = false, want true")
			}
		})
	}
}

func TestLocalStatusLeavesGitDirByteIdentical(t *testing.T) {
	repo := cloneWithUpstreamAhead(t)
	// A touched file with a stale index entry is what makes a plain
	// `git status` rewrite the index.
	if err := os.WriteFile(filepath.Join(repo, "one"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := hashGitDir(t, repo)
	LocalStatus(context.Background(), NewGit(repo))
	if after := hashGitDir(t, repo); after != before {
		t.Error(".git changed after LocalStatus")
	}
}

func TestLocalStatusNeverFetchesAndNeverTakesLocks(t *testing.T) {
	g := &fakeGit{answers: map[string]string{
		"rev-list --count HEAD..@{upstream}":     "3",
		"--no-optional-locks status --porcelain": " M x",
	}}
	behind, known, dirty := LocalStatus(context.Background(), g)
	if behind != 3 || !known || !dirty {
		t.Errorf("got %d %v %v, want 3 true true", behind, known, dirty)
	}
	if len(g.calls) == 0 {
		t.Fatal("no git calls")
	}
	for _, c := range g.calls {
		for _, banned := range []string{"fetch", "pull", "remote update", "ls-remote", "push"} {
			if strings.Contains(c, banned) {
				t.Errorf("network or write command: git %s", c)
			}
		}
		if strings.Contains(c, "status") && !strings.HasPrefix(c, "--no-optional-locks ") {
			t.Errorf("status without --no-optional-locks: git %s", c)
		}
	}
}
