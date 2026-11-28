package txsync

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/claude"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/contract"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

// The four HCS-0008 invariants, as tests over every adapter aval ships.
// They claim the shape of the invariants only, never HCS conformance.

// everyElement is a config that exercises every capability of the model.
func everyElement() agentcfg.Config {
	return agentcfg.Config{
		Rules:    []agentcfg.Rule{{ID: "agent-rules", Body: "Strict TDD."}},
		Skills:   []agentcfg.Skill{{Name: "strict-tdd", Description: "RED first", Hash: "sha256:00"}},
		Commands: []agentcfg.Command{{Name: "debug", Description: "Find the cause", Body: "Debug $ARGUMENTS"}},
		Agents:   []agentcfg.Agent{{Name: "planner", Description: "Plans", Body: "You plan."}},
		Hooks: []agentcfg.HookGroup{{Event: "PreToolUse", Matcher: "Bash", Handlers: []agentcfg.HookHandler{
			{Type: "command", Command: "aval hook guard", Timeout: 5},
		}}},
		Permissions: agentcfg.Permissions{Deny: []string{"Read(~/.ssh/**)"}, Allow: []string{"mcp__pencil"}},
		MCP:         agentcfg.MCP{Denied: []string{"chrome-devtools"}},
	}
}

// 1. Projected or reported: every element of the config is delivered or
// warned about, by every adapter, on a config with every kind of element and
// on the repo's own.
func TestHCS0008ProjectedOrReportedForEveryAdapter(t *testing.T) {
	opt := env(t)
	repoCfg, err := agentcfg.Load(opt.Repo)
	if err != nil {
		t.Fatal(err)
	}
	adapters := Adapters(opt)
	if len(adapters) < 2 {
		t.Fatalf("Adapters = %d, want every shipped adapter", len(adapters))
	}
	for _, a := range adapters {
		for name, cfg := range map[string]agentcfg.Config{"every element": everyElement(), "repo": repoCfg} {
			t.Run(a.Name()+"/"+name, func(t *testing.T) { contract.Run(t, a, cfg) })
		}
	}
}

// 2. One generation per run: a run reads the generation it was given and no
// other, even when a newer one is stored and the repo has moved on.
func TestHCS0008ARunReadsExactlyOneGeneration(t *testing.T) {
	opt := env(t)
	live := filepath.Join(opt.Home, ".claude/settings.json")
	a, b := genWith(t, opt, "a", ""), genWith(t, opt, "b", "")
	put(t, filepath.Join(opt.Repo, "config/claude/settings.json"), `{"env":{"AVAL_GEN":"repo"}}`, 0o644)

	opt.Generation = a
	res, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, live)
	if res.Generation != a || !strings.Contains(got, `"AVAL_GEN": "a"`) || strings.Contains(got, `"b"`) || strings.Contains(got, `"repo"`) {
		t.Errorf("run of %s (stored: %s, repo moved on) reported %s and wrote\n%s", a, b, res.Generation, got)
	}
	hist := read(t, filepath.Join(generation.Root(opt.Home), generation.HistoryFile))
	if lines := strings.Fields(hist); len(lines) != 2 || lines[0] != a {
		t.Errorf("history = %q, want exactly one activation of %s", hist, a)
	}
}

// 3. Host settings win: the private overlay and the repo can add denies and
// never remove one the host already has.
func TestHCS0008TheOverlayNeverRemovesAHostDeny(t *testing.T) {
	opt := env(t)
	live := filepath.Join(opt.Home, ".claude/settings.json")
	put(t, live, `{"permissions":{"deny":["Bash(rm -rf:*)","Read(~/.aws/**)"]}}`, 0o600)
	put(t, filepath.Join(opt.PrivateDir, "settings.json"),
		`{"permissions":{"deny":[],"allow":["Bash(rm -rf:*)"]}}`, 0o600)
	res, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	got := read(t, live)
	for _, deny := range []string{`"Bash(rm -rf:*)"`, `"Read(~/.aws/**)"`} {
		if !strings.Contains(got, deny) {
			t.Errorf("the host deny %s was removed:\n%s", deny, got)
		}
	}
	if strings.Contains(got, `"allow"`) {
		t.Errorf("an allow the host denies was written:\n%s", got)
	}
}

// 4. No ambient config: rendering reads the config it is given, never the
// real HOME. A HOME full of someone else's settings changes nothing.
func TestHCS0008NoAmbientConfig(t *testing.T) {
	opt := env(t)
	cfg := everyElement()
	render := func() map[string][]byte {
		out := map[string][]byte{}
		for _, a := range Adapters(opt) {
			r, err := a.Render(cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range r.Files {
				out[a.Name()+"/"+f.Path] = f.Data
			}
		}
		return out
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clean := render()

	ambient := t.TempDir()
	for p, body := range map[string]string{
		".claude/settings.json":  `{"model":"ambient","permissions":{"deny":["Ambient"]}}`,
		".claude/CLAUDE.md":      "ambient rules",
		".pi/agent/auth.json":    `{"token":"ambient"}`,
		".pi/agent/AGENTS.md":    "ambient",
		".aval/generations/x":    "ambient",
		".config/aval/private/s": "ambient",
	} {
		put(t, filepath.Join(ambient, p), body, 0o644)
	}
	t.Setenv("HOME", ambient)
	t.Setenv("USERPROFILE", ambient)
	if got := render(); len(got) != len(clean) {
		t.Fatalf("%d files with an ambient HOME, %d without", len(got), len(clean))
	} else {
		for p, want := range clean {
			if !bytes.Equal(got[p], want) {
				t.Errorf("%s changed with the real HOME:\n%s\nwant\n%s", p, got[p], want)
			}
		}
	}
	_ = claude.New
}
