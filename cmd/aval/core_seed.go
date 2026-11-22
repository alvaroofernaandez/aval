package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/alvaroofernaandez/aval/internal/core/repoloc"
)

// finishInstall is what install.sh did after linking: seed the Claude settings,
// let `aval sync` merge the repo-managed keys into them, and turn the versioned
// git hooks on. The merge is best-effort (the install already worked); the
// seed is not. It also records where the repository is, so later commands
// find it from any directory; failing to record it only warns.
func finishInstall(repo, home string, dryRun bool) error {
	if err := seedClaudeSettings(repo, home, dryRun); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	_ = runSync(repo, home, nil, false, io.Discard)
	recordRepo(repo, home)
	return configureHooksPath(repo, dryRun)
}

// configureHooksPath points a git checkout at its versioned hooks:
// pre-push refuses employer material and secrets in outgoing commits. Skipped
// on --dry-run, outside a git checkout, and when the repo has no .githooks.
func configureHooksPath(repo string, dryRun bool) error {
	if dryRun {
		return nil
	}
	if fi, err := os.Stat(filepath.Join(repo, ".githooks")); err != nil || !fi.IsDir() {
		return nil
	}
	if err := exec.Command("git", "-C", repo, "rev-parse", "--git-dir").Run(); err != nil {
		return nil
	}
	return exec.Command("git", "-C", repo, "config", "core.hooksPath", ".githooks").Run()
}

// seedClaudeSettings creates ~/.claude/settings.json from the repo when it is
// missing, or replaces a link an older install left behind. An existing real
// file is kept: `aval sync` merges the repo-managed keys into it. It is never a
// link, because sync would then write personal keys and the private overlay
// into this repository.
func seedClaudeSettings(repo, home string, dryRun bool) error {
	dst := filepath.Join(home, ".claude", "settings.json")
	if fi, err := os.Lstat(dst); err == nil && fi.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	if dryRun {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(repo, "config", "claude", "settings.json"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_ = os.Remove(dst) // an old link; the target stays in the repo
	return os.WriteFile(dst, b, 0o644)
}

// recordRepo writes the repository location to aval's config file.
func recordRepo(repo, home string) {
	abs, err := filepath.Abs(repo)
	if err == nil {
		err = repoloc.Save(repoloc.ConfigPath(os.Getenv, home, runtime.GOOS), abs)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not record the repository location: %v\n", err)
	}
}
