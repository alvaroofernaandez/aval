// Package testenv holds the small helpers tests use to run on every OS the
// project supports: skip what is genuinely Unix-only (with the reason on the
// record) and point a process at a throwaway home on Windows as well as Unix.
//
// It is imported only from _test.go files.
package testenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// SkipWindows skips the test on Windows. The reason is printed by `go test -v`
// and in the CI log, so a skip is never silent: say what Unix feature the test
// needs (a POSIX shell, tmux, chmod bits, launchd...).
func SkipWindows(t testing.TB, reason string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("not supported on windows: " + reason)
	}
}

// NeedSymlinks marks a test of symlink-mode behaviour. It is skipped on
// Windows even when the runner could create links (CI runs elevated): there
// the installer's contract is copy mode, covered by its own tests, and a
// symlink's meaning differs (file and directory links are distinct kinds,
// created with privileges). Elsewhere it skips only if links cannot be made.
func NeedSymlinks(t testing.TB) {
	t.Helper()
	SkipWindows(t, "symlink-mode behaviour; Windows installs in copy mode, tested separately")
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
}

// HomeEnv returns the KEY=VALUE pairs that make home the user's home for a
// child process on every OS: HOME for Unix tools, USERPROFILE for Go's
// os.UserHomeDir on Windows, LocalAppData/AppData for os.UserCacheDir and
// os.UserConfigDir there.
func HomeEnv(home string) []string {
	return []string{
		"HOME=" + home, "USERPROFILE=" + home,
		"LocalAppData=" + filepath.Join(home, "AppData", "Local"),
		"AppData=" + filepath.Join(home, "AppData", "Roaming"),
	}
}

// SetHome points the current process at home for the duration of the test, on
// every OS. t.Setenv("HOME", ...) alone does nothing for os.UserHomeDir on
// Windows.
func SetHome(t testing.TB, home string) {
	t.Helper()
	for _, kv := range HomeEnv(home) {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
}
