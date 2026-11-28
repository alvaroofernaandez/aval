package pi_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/contract"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/pi"
)

func sample(t *testing.T) agentcfg.Config {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "strict-tdd")
	for name, body := range map[string]string{
		"SKILL.md":            "---\nname: strict-tdd\ndescription: RED first.\n---\nBody.\n",
		"references/level.md": "levels\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return agentcfg.Config{
		Rules:  []agentcfg.Rule{{ID: "agent-rules", Body: "## Rules\n\nStrict TDD."}},
		Skills: []agentcfg.Skill{{Name: "strict-tdd", Description: "RED first.", Dir: dir, Files: []string{"SKILL.md", "references/level.md"}}},
		Commands: []agentcfg.Command{
			{Name: "debug", Description: "Debug a log", ArgumentHint: "<log>", Body: "Debug `$ARGUMENTS`.\n",
				Raw: "---\ndescription: Debug a log\nargument-hint: <log>\n---\nDebug `$ARGUMENTS`.\n"},
			{Name: "ship", Description: "Ship it", Body: "Ship.\n",
				Raw: "---\ndescription: Ship it\nallowed-tools: Bash(git:*)\nmodel: opus\n---\nShip.\n"},
		},
		Agents:      []agentcfg.Agent{{Name: "planner", File: "planner.md", Description: "Plans"}},
		Hooks:       []agentcfg.HookGroup{{Event: "PreToolUse", Matcher: "Bash", Handlers: []agentcfg.HookHandler{{Type: "command", Command: "aval hook guard"}}}},
		Permissions: agentcfg.Permissions{Deny: []string{"Read(~/.ssh/**)"}},
		MCP: agentcfg.MCP{
			Servers: []agentcfg.MCPServer{{Name: "docs", Command: "docs-mcp", Args: []string{"--stdio"}}},
			Denied:  []string{"evil"},
		},
	}
}

func render(t *testing.T, cfg agentcfg.Config) (agentcfg.Rendered, map[string]string) {
	t.Helper()
	r, err := pi.New().Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range r.Files {
		files[f.Path] = string(f.Data)
	}
	return r, files
}

func TestRenderLaysOutAnAgentDir(t *testing.T) {
	_, files := render(t, sample(t))
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	want := []string{"AGENTS.md", "mcp.json", "prompts/debug.md", "prompts/ship.md", "settings.json", "skills/strict-tdd/SKILL.md", "skills/strict-tdd/references/level.md"}
	if !slices.Equal(paths, want) {
		t.Fatalf("files = %v\nwant    %v", paths, want)
	}
	if files["skills/strict-tdd/references/level.md"] != "levels\n" {
		t.Error("a skill file was not copied byte for byte")
	}
	if !strings.Contains(files["AGENTS.md"], "Strict TDD.") || strings.Count(files["AGENTS.md"], "## Rules") != 1 {
		t.Errorf("AGENTS.md = %q, want the shared rules once", files["AGENTS.md"])
	}
	if files["prompts/debug.md"] != "---\ndescription: Debug a log\nargument-hint: <log>\n---\nDebug `$ARGUMENTS`.\n" {
		t.Errorf("prompt = %q", files["prompts/debug.md"])
	}
	if files["mcp.json"] != "{\n  \"mcpServers\": {\n    \"docs\": {\n      \"args\": [\n        \"--stdio\"\n      ],\n      \"command\": \"docs-mcp\"\n    }\n  }\n}\n" {
		t.Errorf("mcp.json = %q", files["mcp.json"])
	}
}

// What Pi cannot express is reported, per element, with the reason.
func TestLossesAreReported(t *testing.T) {
	r, _ := render(t, sample(t))
	reasons := map[string]string{}
	for _, w := range r.Warnings {
		reasons[w.ID] = w.Reason
	}
	for id, word := range map[string]string{
		"hook.PreToolUse.0":               "extension",
		"permission.deny.Read(~/.ssh/**)": "permission",
		"agent.planner":                   "subagent",
		"command.ship":                    "allowed-tools",
	} {
		if !strings.Contains(reasons[id], word) {
			t.Errorf("%s: reason %q does not mention %q", id, reasons[id], word)
		}
	}
	if !slices.Contains(r.Projected, "command.ship") || !slices.Contains(r.Projected, "mcp.denied.evil") {
		t.Errorf("projected = %v: a partial command and a denied server are still delivered", r.Projected)
	}
}

// The billing guard: nothing aval renders for Pi carries an API key.
func TestNoAPIKeyIsRendered(t *testing.T) {
	for name, native := range map[string]map[string]any{
		"api key in settings": {"apiKey": "sk-ant-x"},
		"env key in settings": {"env": map[string]any{"ANTHROPIC_API_KEY": "sk-ant-x"}},
		"nested provider key": {"providers": map[string]any{"anthropic": map[string]any{"apiKey": "x"}}},
	} {
		cfg := sample(t)
		cfg.Native = map[string]map[string]any{pi.Name: native}
		if _, err := pi.New().Render(cfg); !errors.Is(err, pi.ErrAPIKey) {
			t.Errorf("%s: Render = %v, want ErrAPIKey", name, err)
		}
	}
	cfg := sample(t)
	cfg.MCP.Servers[0].Args = []string{"--key", "sk-ant-api03-abc"}
	if _, err := pi.New().Render(cfg); !errors.Is(err, pi.ErrAPIKey) {
		t.Errorf("an Anthropic key in an MCP server's args = %v, want ErrAPIKey", err)
	}
}

func TestNativeSettingsAreKept(t *testing.T) {
	cfg := sample(t)
	cfg.Native = map[string]map[string]any{pi.Name: {"defaultThinkingLevel": "high"}}
	_, files := render(t, cfg)
	if files["settings.json"] != "{\n  \"defaultThinkingLevel\": \"high\"\n}\n" {
		t.Errorf("settings.json = %q", files["settings.json"])
	}
}

// Env isolates the run: Pi reads only the agent dir aval rendered, and no
// API-billed credential reaches it.
func TestEnvIsolatesTheRunAndDropsAPIKeys(t *testing.T) {
	base := []string{"HOME=/h", "ANTHROPIC_API_KEY=sk", "ANTHROPIC_AUTH_TOKEN=t", "ANTHROPIC_OAUTH_TOKEN=o",
		"PI_CODING_AGENT_DIR=/elsewhere", "PATH=/bin"}
	got := pi.Env(base, "/gen/pi")
	want := []string{"HOME=/h", "PATH=/bin", "PI_CODING_AGENT_DIR=/gen/pi"}
	if !slices.Equal(got, want) {
		t.Errorf("Env = %v, want %v", got, want)
	}
}

func TestAdapterContract(t *testing.T) {
	contract.Run(t, pi.New(), sample(t))
}

func TestAdapterContractOnTheRepo(t *testing.T) {
	cfg, err := agentcfg.Load(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	contract.Run(t, pi.New(), cfg)
}

func TestDeniedServerIsLeftOut(t *testing.T) {
	cfg := sample(t)
	cfg.MCP.Denied = []string{"docs"}
	r, files := render(t, cfg)
	if files["mcp.json"] != "{\n  \"mcpServers\": {}\n}\n" {
		t.Errorf("mcp.json = %q, want the denied server left out", files["mcp.json"])
	}
	if slices.Contains(r.Projected, "mcp.docs") {
		t.Error("a denied server is listed as projected")
	}
}
