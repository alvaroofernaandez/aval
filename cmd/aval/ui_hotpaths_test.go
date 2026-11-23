package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runMain runs `aval args...` in process with a config file holding cfg and
// returns the exit code and stderr.
func runMain(t *testing.T, cfg string, stdin string, args ...string) (int, string) {
	t.Helper()
	xdg, home := t.TempDir(), t.TempDir()
	if cfg != "" {
		p := filepath.Join(xdg, "aval", "config")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A working directory with no repository, as the child process had.
	t.Chdir(t.TempDir())
	env := map[string]string{"XDG_CONFIG_HOME": xdg, "HOME": home, "USERPROFILE": home, "AVAL_NO_UPDATE_CHECK": "1"}
	r := runIn(t, env, stdin, args...)
	return r.code, r.stderr
}

// A bad --theme is an error only where the UI is drawn: install and doctor.
func TestBadThemeFlagExitsOneOnCommandsThatDraw(t *testing.T) {
	for _, cmd := range []string{"doctor", "install"} {
		code, stderr := runMain(t, "", "", "--theme", "tokio-night", cmd)
		if code != 1 || !strings.Contains(stderr, "--theme") || !strings.Contains(stderr, "tokyo-night") {
			t.Errorf("%s: exit %d, stderr %q", cmd, code, stderr)
		}
	}
}

// Hooks, statusline and tmux are on the speed budget: a broken config file, a
// bad theme in it and a bad --theme flag must not change a thing, because none
// of them may read the config.
func TestHotPathsNeverReadConfig(t *testing.T) {
	broken := "theme = tokio-night\nmotion = sometimes\ncolour = red\n"
	cases := [][]string{
		{"statusline"},
		{"--theme", "tokio-night", "statusline"},
		{"hook", "design-gate"},
		{"hook", "plan-gate"},
	}
	for _, args := range cases {
		code, stderr := runMain(t, broken, "{}", args...)
		if code != 0 {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
		}
		if strings.Contains(stderr, "config") || strings.Contains(stderr, "theme") || strings.Contains(stderr, "warning") {
			t.Errorf("%v touched the config: %q", args, stderr)
		}
	}
}
