package main

import (
	"os"
	"path/filepath"
)

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
