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
		{"hook", "relay-prompt"},
		{"relay", "compact"},
		{"relay", "commit"},
		{"relay", "notify"},
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

// Hooks and the statusline are machine output and hot paths: --lang,
// AVAL_LANG, a Spanish locale and a language in the config change nothing,
// not even a bad --lang.
func TestHooksIgnoreTheLanguage(t *testing.T) {
	cases := [][]string{{"statusline"}, {"hook", "design-gate"}, {"hook", "plan-gate"}, {"hook", "fmt"}, {"relay", "compact"}, {"relay", "commit"}, {"relay", "notify"}}
	spanish := map[string]string{"AVAL_LANG": "es", "LANG": "es_ES.UTF-8", "LC_ALL": "es_ES.UTF-8"}
	for _, args := range cases {
		base := langRun(t, "", nil, args...)
		for _, extra := range [][]string{{"--lang", "es"}, {"--lang", "klingon"}, nil} {
			got := langRun(t, "language = es\n", spanish, append(extra, args...)...)
			if got != base {
				t.Errorf("%v %v: %+v, want the same as without a language: %+v", extra, args, got, base)
			}
		}
	}
}

// langRun runs args in process with cfg as the config file and env on top of
// a scratch HOME.
func langRun(t *testing.T, cfg string, env map[string]string, args ...string) invocation {
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
	t.Chdir(t.TempDir())
	all := map[string]string{"XDG_CONFIG_HOME": xdg, "HOME": home, "USERPROFILE": home, "AVAL_NO_UPDATE_CHECK": "1"}
	for k, v := range env {
		all[k] = v
	}
	return runIn(t, all, "{}", args...)
}

// machineCase is one machine-facing command and the input it reads.
type machineCase struct {
	name  string
	stdin string
	args  []string
}

// askForSpanish is one way to select Spanish: a flag, extra environment, or a
// language key in the config file.
type askForSpanish struct {
	name  string
	flag  []string
	env   map[string]string
	inCfg bool
}

// Machine output is English whatever the language: --json, the deny reasons of
// the hooks and the status line. Each case runs once with no language and then
// under every way to ask for Spanish; the exit code, stdout and stderr must be
// byte for byte the same (ADR 0006).
func TestMachineOutputIgnoresTheLanguage(t *testing.T) {
	repo, home := fakeMachine(t)
	proj := t.TempDir()
	design := `{"session_id":"s","cwd":` + q(proj) + `,"tool_name":"Edit","tool_input":{"file_path":` + q(filepath.Join(proj, "Button.tsx")) + `}}`
	cases := []machineCase{
		{"doctor --json", "", []string{"doctor", "--json", "--repo", repo}},
		{"doctor --json --security", "", []string{"doctor", "--json", "--security", "--repo", repo}},
		{"the rate-guard deny", rateGuardBatch, []string{"hook", "rate-guard"}},
		{"the design-gate deny", design, []string{"hook", "design-gate"}},
		{"the statusline", `{"model":{"display_name":"Opus"},"workspace":{"current_dir":"/tmp/x"}}`, []string{"statusline"}},
	}
	asks := []askForSpanish{
		{"--lang es", []string{"--lang", "es"}, nil, false},
		{"AVAL_LANG=es", nil, map[string]string{"AVAL_LANG": "es"}, false},
		{"a Spanish locale", nil, map[string]string{"LANG": "es_ES.UTF-8", "LC_ALL": "es_ES.UTF-8"}, false},
		{"language = es in the file", nil, nil, true},
	}
	cacheDir := t.TempDir()
	t.Chdir(t.TempDir())
	runWith := func(c machineCase, a askForSpanish) invocation {
		cfgHome := t.TempDir()
		if a.inCfg {
			writeFile(t, filepath.Join(cfgHome, "aval", "config"), "language = es\n")
		}
		env := map[string]string{
			"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": cfgHome, "XDG_CACHE_HOME": cacheDir,
			"AVAL_PRIVATE_DIR": filepath.Join(home, "private"), "AVAL_NO_UPDATE_CHECK": "1",
		}
		for k, v := range a.env {
			env[k] = v
		}
		return runIn(t, env, c.stdin, append(append([]string{}, a.flag...), c.args...)...)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := runWith(c, askForSpanish{})
			if base.stdout == "" {
				t.Fatalf("the English run printed nothing (exit %d, stderr %q)", base.code, base.stderr)
			}
			for _, a := range asks {
				if got := runWith(c, a); got != base {
					t.Errorf("%s changed the output.\n got: %+v\nwant: %+v", a.name, got, base)
				}
			}
		})
	}
}
