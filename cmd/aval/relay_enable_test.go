package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

// relayHome is a HOME with live settings and, when mod is true, the
// installed relay mod.
func relayHome(t *testing.T, settings string, mod bool) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if settings != "" {
		if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if mod {
		if err := os.MkdirAll(filepath.Join(home, ".claude", "aval-relay"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func relayCmd(home string, extra map[string]string, args ...string) error {
	env := map[string]string{"HOME": home, "USERPROFILE": home, "AVAL_LANG": "en"}
	for k, v := range extra {
		env[k] = v
	}
	var out bytes.Buffer
	return runRelay(args, uiFlags{}, cmdIO{stdin: strings.NewReader(""), stdout: &out, stderr: &out, env: envMap(env)})
}

func liveSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func liveEnv(m map[string]any) map[string]any {
	e, _ := m["env"].(map[string]any)
	return e
}

func TestRelayEnableSetsEnvKeys(t *testing.T) {
	home := relayHome(t, `{"env":{"KEEP":"1"},"model":"opus"}`, true)
	if err := relayCmd(home, nil, "enable"); err != nil {
		t.Fatal(err)
	}
	s := liveSettings(t, home)
	env := liveEnv(s)
	mod := filepath.Join(home, ".claude", "aval-relay")
	want := map[string]any{"KEEP": "1", "AVAL_RELAY": "1", "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "233000", "CLAUDE_CODE_PLUGIN_DIRS": mod}
	if len(env) != len(want) {
		t.Fatalf("env %v, want %v", env, want)
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("env[%s] = %v, want %v", k, env[k], v)
		}
	}
	if s["model"] != "opus" {
		t.Errorf("other settings changed: %v", s)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json.aval-relay.bak")); err != nil {
		t.Errorf("no backup of the live settings: %v", err)
	}
}

func TestRelayEnableHonoursTheHardThreshold(t *testing.T) {
	home := relayHome(t, `{}`, true)
	if err := relayCmd(home, map[string]string{"AVAL_RELAY_HARD_TOKENS": "150000"}, "enable"); err != nil {
		t.Fatal(err)
	}
	if got := liveEnv(liveSettings(t, home))["CLAUDE_CODE_AUTO_COMPACT_WINDOW"]; got != "183000" {
		t.Fatalf("window %v, want 183000 (hard tokens plus the 33k compaction buffer)", got)
	}
}

func TestRelayEnableKeepsOtherPluginDirsAndIsIdempotent(t *testing.T) {
	other := filepath.Join(string(filepath.Separator)+"opt", "mods")
	home := relayHome(t, `{"env":{"CLAUDE_CODE_PLUGIN_DIRS":`+jsonString(other)+`}}`, true)
	for range 2 {
		if err := relayCmd(home, nil, "enable"); err != nil {
			t.Fatal(err)
		}
	}
	want := other + string(os.PathListSeparator) + filepath.Join(home, ".claude", "aval-relay")
	if got := liveEnv(liveSettings(t, home))["CLAUDE_CODE_PLUGIN_DIRS"]; got != want {
		t.Fatalf("plugin dirs %q, want %q", got, want)
	}
}

func TestRelayDisableRemovesExactlyItsKeys(t *testing.T) {
	other := filepath.Join(string(filepath.Separator)+"opt", "mods")
	home := relayHome(t, `{"env":{"KEEP":"1","CLAUDE_CODE_PLUGIN_DIRS":`+jsonString(other)+`}}`, true)
	if err := relayCmd(home, nil, "enable"); err != nil {
		t.Fatal(err)
	}
	if err := relayCmd(home, nil, "disable"); err != nil {
		t.Fatal(err)
	}
	env := liveEnv(liveSettings(t, home))
	if len(env) != 2 || env["KEEP"] != "1" || env["CLAUDE_CODE_PLUGIN_DIRS"] != other {
		t.Fatalf("env after disable: %v", env)
	}
}

func TestRelayEnableNeedsTheInstalledMod(t *testing.T) {
	home := relayHome(t, `{}`, false)
	if err := relayCmd(home, nil, "enable"); !errors.Is(err, errRelayModMissing) {
		t.Fatalf("got %v, want errRelayModMissing", err)
	}
}

func TestRelayCommitRecordsTokensAfter(t *testing.T) {
	state := t.TempDir()
	compact(t, state, relayFixture(t), nil)
	r := runIn(t, map[string]string{"XDG_STATE_HOME": state}, `{"version":1,"session_id":"session-a","tokens":61000}`, "relay", "commit")
	if r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Fatalf("commit: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	store := statefs.Store{Dir: filepath.Join(state, "aval", "relay")}
	if e, err := store.Read("session-a"); err != nil || e.State != statefs.Committed || e.TokensAfter != 61_000 {
		t.Fatalf("journal: %+v, %v", e, err)
	}
	if sum, err := store.ReadSummary("session-a"); err != nil || sum.After != 61_000 || sum.Relays != 1 {
		t.Fatalf("summary: %+v, %v", sum, err)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestRelayDisableWithoutSettingsWritesNothing(t *testing.T) {
	home := t.TempDir()
	if err := relayCmd(home, nil, "disable"); err != nil {
		t.Fatalf("disable with no settings: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disable created %s (%v)", filepath.Join(home, ".claude"), err)
	}
}
