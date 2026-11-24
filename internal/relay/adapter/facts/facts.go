// Package facts reads repository facts for the relay capsule with file reads
// only: no git process and no network (the relay runs on a hook path).
package facts

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// Repo returns the facts of the repository that holds dir, or nil when dir is
// in none. It reads HEAD for the branch; a detached HEAD shows the short
// commit. The count of changed files needs the index or a git process, so it
// is left at zero and the capsule omits it.
func Repo(dir string) *relay.RepoFacts {
	_, gitdir, ok := findGitDir(dir)
	if !ok {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(gitdir, "HEAD"))
	if err != nil {
		return nil
	}
	head := strings.TrimSpace(string(raw))
	if ref, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
		return &relay.RepoFacts{Branch: ref}
	}
	return &relay.RepoFacts{Branch: "detached at " + head[:min(7, len(head))]}
}

// findGitDir walks up from dir to the first .git, a directory or a worktree's
// "gitdir:" file, and returns the directory that holds it and the git dir.
func findGitDir(dir string) (root, gitdir string, ok bool) {
	for d := filepath.Clean(dir); ; {
		p := filepath.Join(d, ".git")
		if fi, err := os.Stat(p); err == nil {
			if fi.IsDir() {
				return d, p, true
			}
			if raw, err := os.ReadFile(p); err == nil {
				if gd, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir: "); ok {
					if !filepath.IsAbs(gd) {
						gd = filepath.Join(d, gd)
					}
					return d, gd, true
				}
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", false
		}
		d = parent
	}
}

// Root returns the top directory of the repository that holds dir, or "".
func Root(dir string) string {
	root, _, _ := findGitDir(dir)
	return root
}
