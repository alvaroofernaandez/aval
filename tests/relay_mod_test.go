package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// The relay mod for Claude Code (odd/tasks/relay.md, T10): installed by the
// manifest, its logic tested with node, and off unless a person enables it.

const relayMod = "config/claude/plugins/aval-relay"

func TestRelayModIsLinkedByTheManifest(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repo, "install.manifest"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(relayMod) + `\s+\.claude/aval-relay$`).Match(b) {
		t.Errorf("install.manifest does not link %s to .claude/aval-relay", relayMod)
	}
}

func TestRelayModLogic(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "--test", filepath.Join(repo, relayMod, "hooks", "logic.test.js")).CombinedOutput()
	if err != nil {
		t.Fatalf("node --test failed: %v\n%s", err, out)
	}
}

// The repo settings never turn the relay on: aval sync would enable it for
// everyone. `aval relay enable` writes these keys into the live file only.
func TestRepoSettingsLeaveTheRelayOff(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repo, "config", "claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"AVAL_RELAY", "CLAUDE_CODE_AUTO_COMPACT_WINDOW", "CLAUDE_CODE_PLUGIN_DIRS"} {
		if v, ok := s.Env[k]; ok {
			t.Errorf("config/claude/settings.json sets %s=%q; the relay is off by default", k, v)
		}
	}
}
