package deck

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type gateRepo struct {
	t     *testing.T
	root  string
	cache string
	now   time.Time
}

func newGateRepo(t *testing.T) *gateRepo {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &gateRepo{t: t, root: root, cache: t.TempDir(), now: time.Now()}
}

func (g *gateRepo) plan(name, body string, age time.Duration) string {
	g.t.Helper()
	p := filepath.Join(g.root, "odd", "tasks", name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		g.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		g.t.Fatal(err)
	}
	mt := g.now.Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		g.t.Fatal(err)
	}
	return p
}

// edit runs the gate for a tool call on rel and returns the deny reason, or "".
func (g *gateRepo) edit(tool, rel string, off bool) string {
	g.t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"tool_name":  tool,
		"cwd":        g.root,
		"tool_input": map[string]string{"file_path": filepath.Join(g.root, rel)},
	})
	var out bytes.Buffer
	Gate(bytes.NewReader(payload), &out, GateOptions{Off: off, Cache: g.cache, Now: g.now})
	if out.Len() == 0 {
		return ""
	}
	var resp struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		g.t.Fatalf("gate wrote invalid JSON %q: %v", out.String(), err)
	}
	if resp.HookSpecificOutput.PermissionDecision != "deny" {
		g.t.Fatalf("unexpected decision %q", resp.HookSpecificOutput.PermissionDecision)
	}
	return resp.HookSpecificOutput.PermissionDecisionReason
}

const openPlan = "# Feature\n\n## T1 — core\n- [ ] RED\n- [ ] GREEN\n\n## T2 — cli\n- [ ] RED\n"

func TestPlanGateBlocksSourceEditsUntilTheDeckExists(t *testing.T) {
	g := newGateRepo(t)
	plan := g.plan("feature.md", openPlan, time.Hour)

	reason := g.edit("Edit", "internal/x/x.go", false)
	if reason == "" {
		t.Fatal("a source edit with an open plan and no deck must be denied")
	}
	for _, want := range []string{"odd/tasks/feature.md", "plan-deck", "aval deck build", "--plan", "AVAL_PLAN_DECK_OFF"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason does not mention %q: %s", want, reason)
		}
	}

	if err := AckPlan(g.cache, plan, g.now); err != nil {
		t.Fatal(err)
	}
	if r := g.edit("Write", "internal/x/x.go", false); r != "" {
		t.Errorf("after the deck the edit must pass, got %s", r)
	}
}

func TestPlanGateAllows(t *testing.T) {
	cases := []struct {
		name  string
		setup func(g *gateRepo)
		tool  string
		file  string
		off   bool
	}{
		{"no plan at all", func(g *gateRepo) {}, "Edit", "main.go", false},
		{"editing the plan itself", func(g *gateRepo) { g.plan("f.md", openPlan, time.Hour) }, "Edit", "odd/tasks/f.md", false},
		{"writing the deck spec", func(g *gateRepo) { g.plan("f.md", openPlan, time.Hour) }, "Write", "docs/decks/f.deck.json", false},
		{"a finished plan", func(g *gateRepo) { g.plan("f.md", "## T1\n- [x] done\n", time.Hour) }, "Edit", "main.go", false},
		{"a stale plan", func(g *gateRepo) { g.plan("f.md", openPlan, 100*time.Hour) }, "Edit", "main.go", false},
		{"a tool that does not edit", func(g *gateRepo) { g.plan("f.md", openPlan, time.Hour) }, "Read", "main.go", false},
		{"bypass", func(g *gateRepo) { g.plan("f.md", openPlan, time.Hour) }, "Edit", "main.go", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newGateRepo(t)
			c.setup(g)
			if r := g.edit(c.tool, c.file, c.off); r != "" {
				t.Errorf("want allowed, got %s", r)
			}
		})
	}
}

// Ticking boxes is progress, not a new plan; a new section is.
func TestPlanGateFollowsTheHeadings(t *testing.T) {
	g := newGateRepo(t)
	plan := g.plan("f.md", openPlan, time.Hour)
	if err := AckPlan(g.cache, plan, g.now); err != nil {
		t.Fatal(err)
	}
	g.plan("f.md", strings.Replace(openPlan, "- [ ] RED", "- [x] RED", 1), time.Hour)
	if r := g.edit("Edit", "main.go", false); r != "" {
		t.Errorf("ticking a box must not need a new deck: %s", r)
	}
	g.plan("f.md", openPlan+"\n## T3 — new part\n- [ ] RED\n", time.Hour)
	if r := g.edit("Edit", "main.go", false); !strings.Contains(r, "changed") {
		t.Errorf("a new section needs a new deck, got %q", r)
	}
}

// The newest open plan is the active one; older open plans do not block.
func TestPlanGateUsesTheNewestOpenPlan(t *testing.T) {
	g := newGateRepo(t)
	g.plan("old.md", openPlan, 10*time.Hour)
	newer := g.plan("new.md", openPlan, time.Hour)
	if err := AckPlan(g.cache, newer, g.now); err != nil {
		t.Fatal(err)
	}
	if r := g.edit("Edit", "main.go", false); r != "" {
		t.Errorf("only the newest open plan counts, got %s", r)
	}
}

func TestPlanGateFailsOpen(t *testing.T) {
	var out bytes.Buffer
	for _, in := range []string{"", "not json", `{"tool_name":"Edit"}`, `{"tool_name":"Edit","tool_input":{"file_path":"/nowhere/x.go"}}`} {
		out.Reset()
		Gate(strings.NewReader(in), &out, GateOptions{Cache: t.TempDir(), Now: time.Now()})
		if out.Len() != 0 {
			t.Errorf("%q: the gate must fail open, wrote %s", in, out.String())
		}
	}
}

func TestAckPlanNeedsAPlanFile(t *testing.T) {
	if err := AckPlan(t.TempDir(), filepath.Join(t.TempDir(), "missing.md"), time.Now()); err == nil {
		t.Fatal("want an error for a missing plan")
	}
}
