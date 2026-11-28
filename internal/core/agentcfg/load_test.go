package agentcfg_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

func put(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const settings = `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "aval hook guard", "timeout": 5}]},
      {"hooks": [{"type": "command", "command": "aval hook lessons", "async": true}]}
    ],
    "Stop": [{"hooks": [{"type": "command", "command": "aval hook done"}]}]
  },
  "permissions": {"allow": ["mcp__pencil"], "deny": ["Read(~/.ssh/**)"], "ask": ["Read(.env)"], "defaultMode": "bypassPermissions"},
  "deniedMcpServers": [{"serverName": "chrome-devtools"}],
  "statusLine": {"type": "command", "command": "aval statusline"}
}
`

// fixtureRepo is a small aval repository in today's format.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	put(t, repo, "config/claude/CLAUDE.md", "# Global\n\n<!-- shared:agent-rules -->\n## Rules\n\n- Strict TDD.\n<!-- /shared:agent-rules -->\n\nPersonal tail.\n")
	put(t, repo, "skills/strict-tdd/SKILL.md", "---\nname: strict-tdd\ndescription: >\n  RED first,\n  then GREEN.\n---\n\nBody.\n")
	put(t, repo, "skills/strict-tdd/references/levels.md", "levels")
	put(t, repo, "skills/_shared/notes.md", "not a skill")
	put(t, repo, "skills/PROVENANCE.md", "not a skill")
	put(t, repo, "config/claude/commands/debug.md", "---\ndescription: Find the cause\nargument-hint: \"<log>\"\n---\nDebug `$ARGUMENTS`.\n")
	put(t, repo, "config/claude/agents/planner.md", "---\ndescription: Plans work\nmodel: sonnet\nskills: [strict-tdd]\n---\n\nYou plan.\n")
	put(t, repo, "config/claude/settings.json", settings)
	return repo
}

func TestLoadReadsEveryPartOfTheRepo(t *testing.T) {
	cfg, err := agentcfg.Load(fixtureRepo(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []agentcfg.Rule{{ID: "agent-rules", Body: "## Rules\n\n- Strict TDD."}}; !reflect.DeepEqual(cfg.Rules, want) {
		t.Errorf("rules = %#v, want %#v", cfg.Rules, want)
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0].Name != "strict-tdd" || cfg.Skills[0].Description != "RED first, then GREEN." ||
		!reflect.DeepEqual(cfg.Skills[0].Files, []string{"SKILL.md", "references/levels.md"}) || cfg.Skills[0].Hash == "" {
		t.Errorf("skills = %#v", cfg.Skills)
	}
	if len(cfg.Commands) != 1 || cfg.Commands[0].Name != "debug" || cfg.Commands[0].Description != "Find the cause" ||
		cfg.Commands[0].ArgumentHint != "<log>" || cfg.Commands[0].Body != "Debug `$ARGUMENTS`.\n" {
		t.Errorf("commands = %#v", cfg.Commands)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "planner" || cfg.Agents[0].Model != "sonnet" ||
		!reflect.DeepEqual(cfg.Agents[0].Skills, []string{"strict-tdd"}) || cfg.Agents[0].Body != "\nYou plan.\n" {
		t.Errorf("agents = %#v", cfg.Agents)
	}
	wantHooks := []agentcfg.HookGroup{
		{Event: "PreToolUse", Matcher: "Bash", Handlers: []agentcfg.HookHandler{{Type: "command", Command: "aval hook guard", Timeout: 5}}},
		{Event: "PreToolUse", Handlers: []agentcfg.HookHandler{{Type: "command", Command: "aval hook lessons", Extra: map[string]any{"async": true}}}},
		{Event: "Stop", Handlers: []agentcfg.HookHandler{{Type: "command", Command: "aval hook done"}}},
	}
	if !reflect.DeepEqual(cfg.Hooks, wantHooks) {
		t.Errorf("hooks = %#v\nwant %#v", cfg.Hooks, wantHooks)
	}
	wantPerms := agentcfg.Permissions{Allow: []string{"mcp__pencil"}, Deny: []string{"Read(~/.ssh/**)"}, Ask: []string{"Read(.env)"}, DefaultMode: "bypassPermissions"}
	if !reflect.DeepEqual(cfg.Permissions, wantPerms) {
		t.Errorf("permissions = %#v", cfg.Permissions)
	}
	if !reflect.DeepEqual(cfg.MCP.Denied, []string{"chrome-devtools"}) {
		t.Errorf("mcp = %#v", cfg.MCP)
	}
	if _, ok := cfg.Native["claude-code"]["statusLine"]; !ok {
		t.Errorf("Native claude-code settings missing the status line: %v", cfg.Native)
	}
}

func TestLoadTheRealRepo(t *testing.T) {
	repo, _ := filepath.Abs("../../..")
	cfg, err := agentcfg.Load(repo)
	if err != nil {
		t.Fatalf("Load(repo): %v", err)
	}
	if len(cfg.Rules) != 1 || len(cfg.Skills) == 0 || len(cfg.Commands) == 0 || len(cfg.Agents) == 0 || len(cfg.Hooks) == 0 || len(cfg.Permissions.Deny) == 0 {
		t.Errorf("the repo's model is incomplete: %d rules, %d skills, %d commands, %d agents, %d hooks, %d denies",
			len(cfg.Rules), len(cfg.Skills), len(cfg.Commands), len(cfg.Agents), len(cfg.Hooks), len(cfg.Permissions.Deny))
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("the repo's model does not validate: %v", err)
	}
}

func TestSameRepoSameHash(t *testing.T) {
	repo := fixtureRepo(t)
	a, _ := agentcfg.Load(repo)
	b, _ := agentcfg.Load(repo)
	if a.Hash() != b.Hash() || a.Hash() == "" {
		t.Errorf("two loads of the same repo hash differently: %s %s", a.Hash(), b.Hash())
	}
	put(t, repo, "skills/strict-tdd/references/levels.md", "changed")
	c, _ := agentcfg.Load(repo)
	if c.Hash() == a.Hash() {
		t.Error("a changed skill file did not change the model hash")
	}
}
