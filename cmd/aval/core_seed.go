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
//
// This is the CLI entry: it runs after the program has left the alt screen,
// so it prints completeInstall's warnings to stderr, as it always has.
func finishInstall(repo, home string, dryRun bool) error {
	warnings, err := completeInstall(repo, home, dryRun)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	return err
}

// completeInstall runs the post-install steps and returns its warnings
// instead of writing them: the hub calls it inside the alt screen, where a
// stderr write would corrupt the frame. It backs Services.FinishInstall.
func completeInstall(repo, home string, dryRun bool) ([]string, error) {
	if err := seedClaudeSettings(repo, home, dryRun); err != nil {
		return nil, err
	}
	if dryRun {
		return nil, nil
	}
	_ = runSync(repo, home, nil, false, io.Discard)
	var warnings []string
	if w := recordRepo(repo, home); w != "" {
		warnings = append(warnings, w)
	}
	return warnings, configureHooksPath(repo, dryRun)
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

// recordRepo writes the repository location to aval's config file. Failing
// only warns: it returns the warning, empty when the location was recorded.
func recordRepo(repo, home string) string {
	abs, err := filepath.Abs(repo)
	if err == nil {
		err = repoloc.Save(repoloc.ConfigPath(os.Getenv, home, runtime.GOOS), abs)
	}
	if err != nil {
		return fmt.Sprintf("could not record the repository location: %v", err)
	}
	return ""
}
