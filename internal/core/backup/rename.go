package backup

import (
	"os"
	"runtime"
	"time"
)

// renameAttempts and renameBackoff bound the retry loop for transient
// failures on Windows.
const (
	renameAttempts = 6
	renameBackoff  = 25 * time.Millisecond
)

// renameReplace renames src over dst. On Unix that is os.Rename. On Windows
// Go's os.Rename already replaces an existing file, but the call fails with a
// sharing or access violation while another process (an editor, an antivirus
// scan, Claude Code reading settings.json) holds dst open for a moment, so it
// is retried a few times before giving up.
func renameReplace(src, dst string) error {
	return renameRetry(os.Rename, src, dst, runtime.GOOS == "windows", time.Sleep)
}

// renameRetry is renameReplace with the OS, the rename and the sleep injected,
// so the retry policy is testable on any platform.
func renameRetry(rename func(src, dst string) error, src, dst string, retry bool, sleep func(time.Duration)) error {
	err := rename(src, dst)
	if err == nil || !retry {
		return err
	}
	for i := 1; i < renameAttempts && err != nil; i++ {
		sleep(renameBackoff * time.Duration(i))
		err = rename(src, dst)
	}
	return err
}

// SameContent reports whether dst holds exactly what src holds: the same
// tree of names, file contents and permission bits. A symlink counts as
// different, which is what copy-mode installs need: a copy is "ours" only
// while it still equals the repo.
func SameContent(src, dst string) bool {
	if fi, err := os.Lstat(dst); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	a, err := hashTree(src)
	if err != nil {
		return false
	}
	b, err := hashTree(dst)
	return err == nil && a == b
}
