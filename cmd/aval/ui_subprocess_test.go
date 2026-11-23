package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets tests run the real main() in a child process: the test binary
// re-executes itself with AVAL_TEST_MAIN set.
func TestMain(m *testing.M) {
	if os.Getenv("AVAL_TEST_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runMain runs `aval args...` with a config file holding cfg and returns the
// exit code and stderr.
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
	cmd := exec.Command(os.Args[0], args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "AVAL_TEST_MAIN=1", "XDG_CONFIG_HOME="+xdg, "HOME="+home,
		"AVAL_REPO=", "AVAL_NO_UPDATE_CHECK=1", "NO_COLOR=")
	cmd.Stdin = strings.NewReader(stdin)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, errb.String()
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
