package claude

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// repoSettingsFile is the real settings.json the repo ships: the generated
// Windows commands are checked against it, not against a hand-made fixture.
func repoSettingsFile(t *testing.T) map[string]any {
	t.Helper()
	m, _, err := readJSONFile(filepath.Join("..", "..", "..", "..", "config", "claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

type hookRef struct {
	event string
	h     map[string]any
}

func eachHook(m map[string]any) []hookRef {
	var out []hookRef
	hooks, _ := m["hooks"].(map[string]any)
	for ev, groups := range hooks {
		for _, g := range groups.([]any) {
			for _, h := range g.(map[string]any)["hooks"].([]any) {
				out = append(out, hookRef{ev, h.(map[string]any)})
			}
		}
	}
	return out
}

const winBin = `C:\Users\dev\.local\bin\aval.exe`

func TestForOSLeavesUnixSettingsAlone(t *testing.T) {
	repo := repoSettingsFile(t)
	for _, goos := range []string{"darwin", "linux"} {
		got := ForOS(repo, goos, "", "/home/dev")
		if !reflect.DeepEqual(got, repo) {
			t.Errorf("%s: settings changed", goos)
		}
		for _, r := range eachHook(got) {
			if _, has := r.h["args"]; has {
				t.Errorf("%s: %s hook gained args", goos, r.event)
			}
		}
		sl := got["statusLine"].(map[string]any)["command"].(string)
		if !strings.Contains(sl, `"$a" statusline`) {
			t.Errorf("%s: status line rewritten: %s", goos, sl)
		}
	}
}

func TestForOSWindowsHooksUseExecForm(t *testing.T) {
	repo := repoSettingsFile(t)
	got := ForOS(repo, "windows", winBin, `C:\Users\dev`)

	adapted := 0
	for _, r := range eachHook(got) {
		cmd := r.h["command"].(string)
		args, hasArgs := r.h["args"].([]any)
		if strings.Contains(cmd, `$a`) || strings.Contains(cmd, `[ -x`) {
			t.Errorf("%s: POSIX shell syntax survived: %s", r.event, cmd)
		}
		if !hasArgs {
			continue // a non-aval hook (code-review-graph): untouched
		}
		adapted++
		if cmd != "C:/Users/dev/.local/bin/aval.exe" {
			t.Errorf("%s: command = %q, want the forward-slash aval.exe path", r.event, cmd)
		}
		if len(args) == 0 || args[0] != "hook" {
			t.Errorf("%s: args = %v, want hook <name>", r.event, args)
		}
	}
	if adapted < 4 {
		t.Errorf("only %d aval hooks were adapted; the repo has at least lessons, design-gate, repo-profile, design-shot-stop", adapted)
	}

	want := []string{"lessons", "design-gate", "repo-profile", "design-shot-stop", "fmt"}
	found := map[string]bool{}
	for _, r := range eachHook(got) {
		args, ok := r.h["args"].([]any)
		if !ok || len(args) != 2 {
			continue
		}
		found[args[1].(string)] = true
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("hook %q was not adapted", name)
		}
	}

	// Other fields of the hook (timeout, async...) survive.
	for _, r := range eachHook(repo) {
		if c, _ := r.h["command"].(string); strings.Contains(c, `"$a" hook design-gate`) {
			for k, v := range r.h {
				if k == "command" {
					continue
				}
				for _, g := range eachHook(got) {
					if a, ok := g.h["args"].([]any); ok && len(a) == 2 && a[1] == "design-gate" && !reflect.DeepEqual(g.h[k], v) {
						t.Errorf("design-gate field %q changed: %v -> %v", k, v, g.h[k])
					}
				}
			}
		}
	}
}

func TestForOSWindowsStatusLine(t *testing.T) {
	repo := repoSettingsFile(t)
	got := ForOS(repo, "windows", winBin, "")
	sl := got["statusLine"].(map[string]any)
	if sl["command"] != "C:/Users/dev/.local/bin/aval.exe statusline" {
		t.Errorf("statusLine command = %q", sl["command"])
	}
	if sl["type"] != "command" {
		t.Errorf("statusLine type = %v", sl["type"])
	}

	spaced := ForOS(repo, "windows", `C:\Users\Ana Maria\.local\bin\aval.exe`, "")
	if c := spaced["statusLine"].(map[string]any)["command"]; c != `"C:/Users/Ana Maria/.local/bin/aval.exe" statusline` {
		t.Errorf("a path with a space must be quoted, got %q", c)
	}
}

func TestForOSDoesNotMutateItsInput(t *testing.T) {
	repo := repoSettingsFile(t)
	before := clone(repo)
	ForOS(repo, "windows", winBin, "")
	if !reflect.DeepEqual(repo, before) {
		t.Error("ForOS mutated the repo settings")
	}
}

func TestAvalArgs(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
		ok   bool
	}{
		{`a="$HOME/.local/bin/aval"; [ -x "$a" ] || a="$(command -v aval)"; [ -x "$a" ] && "$a" hook lessons || true`, []string{"hook", "lessons"}, true},
		{`a="$HOME/.local/bin/aval"; [ -x "$a" ] || a="$(command -v aval)"; [ -x "$a" ] && "$a" statusline || echo claude`, []string{"statusline"}, true},
		{`a="$HOME/.local/bin/aval"; [ -x "$a" ] || a="$(command -v aval)"; [ -x "$a" ] && "$a" hook fmt || true`, []string{"hook", "fmt"}, true},
		{`code-review-graph status`, nil, false},
		{`rm -f /tmp/dk_* 2>/dev/null`, nil, false},
	}
	for _, c := range cases {
		got, ok := avalArgs(c.cmd)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("avalArgs(%q) = %v, %v; want %v, %v", c.cmd, got, ok, c.want, c.ok)
		}
	}
}

func TestResolveAvalBinPrefersTheInstalledCopy(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin", "aval.exe")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveAvalBin("windows", home); got != bin {
		t.Errorf("ResolveAvalBin = %q, want %q", got, bin)
	}
}
