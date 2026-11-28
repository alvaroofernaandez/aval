package fake_test

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/contract"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/fake"
)

// sample exercises every capability of the model.
func sample() agentcfg.Config {
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

func TestAdapterContract(t *testing.T) {
	contract.Run(t, fake.New(), sample())
}

func TestAdapterContractWithLosses(t *testing.T) {
	limited := &fake.Adapter{Caps: agentcfg.Capabilities{
		agentcfg.CapRules: agentcfg.Full, agentcfg.CapSkills: agentcfg.Full, agentcfg.CapCommands: agentcfg.Full,
	}}
	contract.Run(t, limited, sample())
}

func TestUnsupportedCapabilityWarnsPerElement(t *testing.T) {
	limited := &fake.Adapter{Caps: agentcfg.Capabilities{agentcfg.CapRules: agentcfg.Full}}
	r, err := limited.Render(sample())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]agentcfg.Capability{}
	for _, w := range r.Warnings {
		got[w.ID] = w.Capability
	}
	for _, id := range []string{"hook.PreToolUse.0", "permission.deny.Read(~/.ssh/**)", "mcp.denied.chrome-devtools", "skill.strict-tdd"} {
		if _, ok := got[id]; !ok {
			t.Errorf("no warning for %s; warnings: %v", id, r.Warnings)
		}
	}
	if _, ok := got["rule.agent-rules"]; ok {
		t.Errorf("a supported rule was reported as lost: %v", r.Warnings)
	}
}
