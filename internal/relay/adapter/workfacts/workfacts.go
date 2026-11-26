// Package workfacts reads what the capsule says about the work in progress:
// the open tasks of the plans in play and the git working tree. Git runs with
// a short timeout; any failure omits what it covers.
package workfacts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// Plans returns the open tasks of the plan files under odd/tasks that the
// session edited (files are the edited paths) or, when it edited none, of the
// plan named like the branch suffix. A plan with nothing open is left out.
func Plans(cwd string, files []string, branch string) []relay.PlanFacts {
	var paths []string
	for _, f := range files {
		if filepath.Ext(f) == ".md" && filepath.Base(filepath.Dir(f)) == "tasks" &&
			filepath.Base(filepath.Dir(filepath.Dir(f))) == "odd" {
			paths = append(paths, f)
		}
	}
	if root := repoRoot(cwd); len(paths) == 0 && root != "" && branch != "" {
		name := branch[strings.LastIndex(branch, "/")+1:]
		paths = append(paths, filepath.Join(root, "odd", "tasks", name+".md"))
	}
	var out []relay.PlanFacts
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if plan := relay.ParsePlan(string(raw)); len(plan.Items) > 0 {
			out = append(out, plan)
		}
	}
	return out
}

// Tree returns the uncommitted files of cwd and the commits made since the
// session began, or nil when git answers neither.
func Tree(cwd string, since time.Time) *relay.TreeFacts { return tree(git, cwd, since) }

type runner func(dir string, args ...string) (string, error)

func tree(run runner, cwd string, since time.Time) *relay.TreeFacts {
	var t relay.TreeFacts
	ok := false
	if out, err := run(cwd, "status", "--porcelain"); err == nil {
		t.Dirty, ok = lines(out), true
	}
	if !since.IsZero() {
		if out, err := run(cwd, "log", "--oneline", "--no-decorate", "--since="+since.UTC().Format(time.RFC3339)); err == nil {
			t.Commits, ok = lines(out), true
		}
	}
	if !ok {
		return nil
	}
	return &t
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// git runs git in dir with a timeout that fits the SessionEnd deadline.
func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "core.quotepath=off"}, args...)...).Output()
	return string(out), err
}

// repoRoot walks up from dir to the first directory with a .git entry.
func repoRoot(dir string) string {
	for d := filepath.Clean(dir); ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}
