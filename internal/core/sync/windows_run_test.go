package sync

import (
	"os"
	"path/filepath"
	"testing"
)

const winBin = `C:\Users\dev\.local\bin\aval.exe`

func TestRunWritesWindowsHooksAndDriftIsPerOS(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	write(t, filepath.Join(repo, "config", "claude", "settings.json"), mustRead(t, filepath.Join("..", "..", "..", "config", "claude", "settings.json")))
	write(t, filepath.Join(repo, "config", "claude", "CLAUDE.md"),
		"intro\n<!-- shared:agent-rules -->\nrule\n<!-- /shared:agent-rules -->\n")
	opt := Options{Repo: repo, Home: home, GOOS: "windows", AvalBin: winBin, Now: fixedNow}
	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	live := decode(t, read(t, filepath.Join(home, ".claude", "settings.json")))
	found := false
	hooks, _ := live["hooks"].(map[string]any)
	for _, groups := range hooks {
		for _, g := range groups.([]any) {
			for _, h := range g.(map[string]any)["hooks"].([]any) {
				if a, ok := h.(map[string]any)["args"].([]any); ok && len(a) == 2 && a[1] == "lessons" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("the live settings has no exec-form lessons hook")
	}
	if keys, err := SettingsDrift(opt); err != nil || len(keys) != 0 {
		t.Errorf("windows drift against its own output = %v, %v; want none", keys, err)
	}
	unix := opt
	unix.GOOS = "linux"
	if keys, _ := SettingsDrift(unix); len(keys) == 0 {
		t.Error("a linux check of a windows-generated file should report drift in hooks")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
