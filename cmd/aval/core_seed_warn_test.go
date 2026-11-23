package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	w.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

// unrecordable is a repo and home where recording the repository fails:
// the config directory sits under a regular file.
func unrecordable(t *testing.T) (repo, home string) {
	t.Helper()
	repo, home = t.TempDir(), t.TempDir()
	blocker := filepath.Join(home, "blocker")
	writeFile(t, blocker, "x")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(blocker, "cfg"))
	writeFile(t, filepath.Join(repo, "config/claude/settings.json"), `{"hooks":{}}`+"\n")
	return repo, home
}

// The hub runs the post-install steps inside the alt screen, so a warning
// must come back as a value: a stderr write would corrupt the frame.
func TestCompleteInstallReturnsWarningsInsteadOfWritingStderr(t *testing.T) {
	repo, home := unrecordable(t)
	var warnings []string
	var err error
	out := captureStderr(t, func() { warnings, err = completeInstall(repo, home, false) })
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("completeInstall wrote to stderr: %q", out)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "could not record the repository location") {
		t.Errorf("warnings = %q, want the repository record warning", warnings)
	}
}

// `aval install` prints the same warning as before, after the program exits.
func TestFinishInstallStillWarnsOnStderrForTheCLI(t *testing.T) {
	repo, home := unrecordable(t)
	out := captureStderr(t, func() {
		if err := finishInstall(repo, home, false); err != nil {
			t.Error(err)
		}
	})
	if !strings.HasPrefix(out, "warning: could not record the repository location: ") || strings.Count(out, "\n") != 1 {
		t.Errorf("stderr = %q, want the one warning line", out)
	}
}
