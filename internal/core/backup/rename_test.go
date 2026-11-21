package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenameRetryRecoversFromATransientFailureOnWindows(t *testing.T) {
	calls, slept := 0, 0
	flaky := func(src, dst string) error {
		calls++
		if calls < 3 {
			return errors.New("The process cannot access the file because it is being used by another process")
		}
		return nil
	}
	err := renameRetry(flaky, "a", "b", true, func(time.Duration) { slept++ })
	if err != nil || calls != 3 || slept != 2 {
		t.Errorf("err=%v calls=%d slept=%d, want nil, 3, 2", err, calls, slept)
	}
}

func TestRenameRetryGivesUpAndReturnsTheLastError(t *testing.T) {
	boom := errors.New("locked")
	calls := 0
	err := renameRetry(func(string, string) error { calls++; return boom }, "a", "b", true, func(time.Duration) {})
	if !errors.Is(err, boom) || calls != renameAttempts {
		t.Errorf("err=%v calls=%d, want %v after %d attempts", err, calls, boom, renameAttempts)
	}
}

func TestRenameRetryIsASingleAttemptOffWindows(t *testing.T) {
	calls := 0
	_ = renameRetry(func(string, string) error { calls++; return errors.New("x") }, "a", "b", false, func(time.Duration) { t.Error("slept") })
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRenameReplaceOverwritesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	for p, c := range map[string]string{src: "new", dst: "old"} {
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := renameReplace(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "new" {
		t.Errorf("dst = %q, want new", b)
	}
}

func TestSameContent(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, file, body string) string {
		root := filepath.Join(dir, name)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}
	a, b := mk("a", "f", "x"), mk("b", "f", "x")
	if !SameContent(a, b) {
		t.Error("identical trees must match")
	}
	if SameContent(a, mk("c", "f", "y")) || SameContent(a, mk("d", "g", "x")) {
		t.Error("different content or names must not match")
	}
	if SameContent(a, filepath.Join(dir, "missing")) {
		t.Error("a missing destination is not the same")
	}
}
