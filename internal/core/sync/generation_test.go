package sync

import (
	"path/filepath"
	"strings"
	"testing"
)

// With Options.Generation, the managed settings come from the generation's
// rendering, merged with the personal keys of the live file as always; the
// rules mirror still comes from the repo (owner: G5 = A).
func TestSyncFromAGenerationTakesItsSettings(t *testing.T) {
	opt := fakeEnv(t)
	gen := t.TempDir()
	write(t, filepath.Join(gen, "settings.json"), `{"env":{"A":"from-generation"}}`)
	write(t, filepath.Join(opt.Home, ".claude/settings.json"), `{"model":"mine","env":{"A":"old"}}`)
	write(t, filepath.Join(opt.Home, ".codex/AGENTS.md"), "mine\n")
	opt.Generation = gen
	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	live := decode(t, read(t, filepath.Join(opt.Home, ".claude/settings.json")))
	if env, _ := live["env"].(map[string]any); env["A"] != "from-generation" {
		t.Errorf("env = %v, want the generation's", live["env"])
	}
	if live["model"] != "mine" {
		t.Errorf("personal key lost: %v", live)
	}
	if _, ok := live["hooks"]; ok {
		t.Errorf("hooks = %v: the repo's settings leaked into a generation sync", live["hooks"])
	}
	if got := read(t, filepath.Join(opt.Home, ".codex/AGENTS.md")); !strings.Contains(got, "rule one") {
		t.Errorf("rules mirror = %q, want the repo's rules", got)
	}
	if keys, err := SettingsDrift(opt); err != nil || len(keys) != 0 {
		t.Errorf("drift against the generation = %v, %v; want none", keys, err)
	}
	if s, err := RepoSettings(opt); err != nil || s["hooks"] != nil {
		t.Errorf("RepoSettings with a generation = %v, %v; want the generation's settings", s, err)
	}
}
