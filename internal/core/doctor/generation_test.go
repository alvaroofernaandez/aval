package doctor

import (
	"encoding/json"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/claude"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

// activeGeneration builds a generation whose Claude Code settings run cmd as
// the Stop hook, and makes it the active one.
func activeGeneration(t *testing.T, o Options, cmd string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"hooks": map[string]any{"Stop": []any{
		map[string]any{"hooks": []any{map[string]any{"type": "command", "command": cmd}}}}},
		"enabledPlugins": map[string]any{"p@m": true, "off@m": false}})
	settings, err := agentcfg.DecodeJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := generation.Build(generation.Options{
		Home: o.Home, Config: agentcfg.FromSettings(settings),
		Adapters: []agentcfg.Adapter{claude.New(claude.Options{Home: o.Home, GOOS: o.GOOS})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := generation.SetCurrent(o.Home, g.ID); err != nil {
		t.Fatal(err)
	}
}

// The active generation is the desired state (owner: G6 = A): a machine that
// matches it is healthy even where the repo moved on, and drift is measured
// against it, not against the repo.
func TestTheActiveGenerationIsTheDesiredState(t *testing.T) {
	o := healthy(t)
	liveWithCommand(t, o, "git status")
	activeGeneration(t, o, "git status")
	rs := Run(o)
	expect(t, rs, "hooks-drift", OK, false)
	expect(t, rs, "settings-drift", OK, false)

	liveWithCommand(t, o, `"$HOME/.claude/hooks/ok.sh"`) // the repo's, not the generation's
	rs = Run(o)
	expect(t, rs, "hooks-drift", Fail, true)
	expect(t, rs, "settings-drift", Fail, true)
}
