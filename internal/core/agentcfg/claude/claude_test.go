package claude_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/claude"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/contract"
	"github.com/alvaroofernaandez/aval/internal/core/manifest"
)

const winBin = `C:\Users\dev\.local\bin\aval.exe`

func repoSettings(t *testing.T) agentcfg.Config {
	t.Helper()
	cfg, err := agentcfg.LoadSettings(filepath.Join("..", "..", "..", "..", "config", "claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func settingsOf(t *testing.T, r agentcfg.Rendered) map[string]any {
	t.Helper()
	for _, f := range r.Files {
		if f.Path == "settings.json" {
			m, err := agentcfg.DecodeJSON(f.Data)
			if err != nil {
				t.Fatal(err)
			}
			return m
		}
	}
	t.Fatalf("no settings.json in %d rendered files", len(r.Files))
	return nil
}

func canonical(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// The model loaded from the repo renders the repo's settings back, unchanged
// on Unix and in exec form on Windows: parity with what sync wrote before.
func TestRenderIsTheRepoSettings(t *testing.T) {
	cfg := repoSettings(t)
	for _, goos := range []string{"linux", "darwin", "windows"} {
		r, err := claude.New(claude.Options{GOOS: goos, AvalBin: winBin}).Render(cfg)
		if err != nil {
			t.Fatal(err)
		}
		want := claude.ForOS(cfg.Native[agentcfg.ClaudeCode], goos, winBin, "")
		if got := settingsOf(t, r); canonical(t, got) != canonical(t, want) {
			t.Errorf("%s: the rendered settings differ from the repo's", goos)
		}
	}
}

// When the model differs from the native spelling, the model wins: a hook,
// deny or denied server changed in the Config shows in the rendering.
func TestRenderFollowsTheModel(t *testing.T) {
	cfg := repoSettings(t)
	cfg.Hooks = []agentcfg.HookGroup{{Event: "Stop", Handlers: []agentcfg.HookHandler{{Type: "command", Command: "aval hook done", Timeout: 3}}}}
	cfg.Permissions.Deny = append(cfg.Permissions.Deny, "Bash(shred:*)")
	cfg.MCP.Denied = []string{"evil"}
	r, err := claude.New(claude.Options{GOOS: "linux"}).Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := canonical(t, settingsOf(t, r))
	for _, want := range []string{
		`"hooks":{"Stop":[{"hooks":[{"command":"aval hook done","timeout":3,"type":"command"}]}]}`,
		`"Bash(shred:*)"`,
		`"deniedMcpServers":[{"serverName":"evil"}]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the rendering does not carry %s", want)
		}
	}
	if strings.Contains(got, "PreToolUse") {
		t.Error("a hook the model dropped is still rendered")
	}
}

func TestSettingsArePrivate(t *testing.T) {
	r, err := claude.New(claude.Options{GOOS: "linux"}).Render(repoSettings(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Files) != 1 || r.Files[0].Mode != 0o600 {
		t.Errorf("files = %+v, want settings.json alone, mode 0600", r.Files)
	}
}

func TestAdapterContract(t *testing.T) {
	contract.Run(t, claude.New(claude.Options{GOOS: "linux"}), repoSettings(t))
}

// Apply keeps the personal keys of the live file and lets the private overlay
// win over the repo, except for an allow the repo denies.
func TestApplyMergesLiveAndOverlay(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".claude")
	private := filepath.Join(home, "private")
	write(t, filepath.Join(target, "settings.json"), `{"model":"sonnet","hooks":{"Old":[]}}`)
	write(t, filepath.Join(private, "settings.json"), `{"env":{"MINE":"1"},"permissions":{"allow":["Read(~/.ssh/id_rsa)"]}}`)
	a := claude.New(claude.Options{GOOS: "linux", PrivateDir: private})
	r, err := a.Render(repoSettings(t))
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.ApplySettings(target, r, agentcfg.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written || len(res.Warnings) != 1 {
		t.Errorf("result = %+v, want written with one dropped allow", res)
	}
	b, _ := os.ReadFile(filepath.Join(target, "settings.json"))
	live := string(b)
	for _, want := range []string{`"model": "sonnet"`, `"MINE": "1"`, `"PreToolUse"`} {
		if !strings.Contains(live, want) {
			t.Errorf("live settings lost %s", want)
		}
	}
	if strings.Contains(live, `"Old"`) || strings.Contains(live, `id_rsa)"`) && !strings.Contains(live, `"Read(~/.ssh/**)"`) {
		t.Error("a stale hook or a denied allow survived")
	}
	if d, err := a.Diff(target, r); err != nil || len(d) != 0 {
		t.Errorf("Diff after Apply = %v, %v", d, err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The whole repo model passes the contract: rules, skills, commands and agents
// are delivered by the install links, each one a link install.manifest makes.
func TestAdapterContractOnTheRepo(t *testing.T) {
	repo := filepath.Join("..", "..", "..", "..")
	cfg, err := agentcfg.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	a := claude.New(claude.Options{GOOS: "linux"})
	contract.Run(t, a, cfg)
	r, err := a.Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(filepath.Join(repo, "install.manifest"))
	if err != nil {
		t.Fatal(err)
	}
	installed := map[string]string{}
	for _, g := range m.Groups {
		for _, e := range g.Entries {
			installed[e.Dest] = e.Source
		}
		for _, d := range g.FanoutDests {
			for _, s := range cfg.Skills {
				installed[d+"/"+s.Name] = g.FanoutSource + "/" + s.Name
			}
		}
	}
	if len(r.Linked) == 0 {
		t.Fatal("no element is linked")
	}
	for _, l := range r.Linked {
		dest, src := l.Dest, l.Source
		for dest != "." && installed[dest] == "" {
			dest, src = path.Dir(dest), path.Dir(src)
		}
		if installed[dest] != src {
			t.Errorf("%s: link %s -> %s is not one install.manifest makes", l.ID, l.Source, l.Dest)
		}
	}
}

// An agent whose front matter names it differently from its file is linked by
// its file: the link is what install makes, the name is what the agent says.
func TestAgentIsLinkedByItsFile(t *testing.T) {
	cfg := agentcfg.Config{Agents: []agentcfg.Agent{{Name: "planner", File: "plan-agent.md", Description: "Plans"}}}
	r, err := claude.New(claude.Options{GOOS: "linux"}).Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Linked) != 1 || r.Linked[0].Source != "config/claude/agents/plan-agent.md" || r.Linked[0].Dest != ".claude/agents/plan-agent.md" {
		t.Errorf("linked = %+v", r.Linked)
	}
}
