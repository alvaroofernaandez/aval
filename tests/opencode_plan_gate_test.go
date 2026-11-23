package tests

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// The OpenCode half of the plan gate. The plugin holds no gate logic: it hands
// the same PreToolUse payload Claude Code sends to `aval hook plan-gate`, so the
// binary under test is the oracle for every deny reason.
//
// Stays a real process: the bun plugin under test spawns the aval binary.

type ocPlanGate struct {
	t     *testing.T
	root  string
	home  string
	probe string
	aval  string
}

const ocPlan = "# Feature\n\n## T1 — core\n- [ ] RED\n"

func newOCPlanGate(t *testing.T) *ocPlanGate {
	t.Helper()
	needTool(t, "bun")
	plugin := p("config", "opencode", "plugin", "plan-gate.ts")
	isTrue(t, "the plugin exists", exists(plugin))
	home := t.TempDir()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "odd", "tasks", "x.md"), ocPlan)
	src := read(t, "tests/testdata/opencode-plan-gate-probe.ts")
	src = regexp.MustCompile(`from "[^"]*plan-gate\.ts"`).ReplaceAllString(src, `from "`+filepath.ToSlash(plugin)+`"`)
	probe := filepath.Join(home, "probe", "opencode-plan-gate-probe.ts")
	write(t, probe, src)
	return &ocPlanGate{t: t, root: root, home: home, probe: probe, aval: avalBin(t)}
}

func (g *ocPlanGate) env(kv ...string) []string {
	path := "PATH=" + filepath.Dir(g.aval) + string(os.PathListSeparator) + os.Getenv("PATH")
	return withEnv(isoEnv(g.home), append([]string{path}, kv...)...)
}

// fire runs the plugin for tool on rel and returns "allow" or "deny <reason>".
func (g *ocPlanGate) fire(env []string, tool, rel string) string {
	file := filepath.Join(g.root, filepath.FromSlash(rel))
	return strings.TrimSpace(run(g.home, env, "", "bun", "run", g.probe, tool, file, g.root).Stdout)
}

// oracle is the deny reason `aval hook plan-gate` gives for the same payload.
func (g *ocPlanGate) oracle(rel string) string {
	g.t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"cwd": g.root, "tool_name": "Edit",
		"tool_input": map[string]string{"file_path": filepath.Join(g.root, filepath.FromSlash(rel))},
	})
	out := run(g.home, g.env(), string(payload), g.aval, "hook", "plan-gate").Stdout
	var resp struct {
		HookSpecificOutput struct {
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil || resp.HookSpecificOutput.PermissionDecisionReason == "" {
		g.t.Fatalf("the binary did not deny %s: %q", rel, out)
	}
	return resp.HookSpecificOutput.PermissionDecisionReason
}

// ackDeck writes the marker `aval deck build --plan` writes, computed here
// independently: <cache>/plan-deck/<sha1 of the root>, one line per plan with
// the sha1 of its trimmed heading lines.
func (g *ocPlanGate) ackDeck() {
	g.t.Helper()
	b, err := os.ReadFile(filepath.Join(g.root, "odd", "tasks", "x.md"))
	if err != nil {
		g.t.Fatal(err)
	}
	h := sha1.New()
	for _, line := range strings.Split(string(b), "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "#") {
			h.Write([]byte(l + "\n"))
		}
	}
	root := sha1.Sum([]byte(g.root))
	write(g.t, filepath.Join(g.home, ".cache", "plan-deck", hex.EncodeToString(root[:])),
		"odd/tasks/x.md="+hex.EncodeToString(h.Sum(nil))+"\n")
}

func TestOpenCodePlanGateDeniesEditWhileThePlanHasNoDeck(t *testing.T) {
	g := newOCPlanGate(t)
	want := "deny " + g.oracle("src/main.go")
	eq(t, "an edit is denied with the binary's reason", want, g.fire(g.env(), "edit", "src/main.go"))
	eq(t, "a write is denied", want, g.fire(g.env(), "write", "src/main.go"))
	eq(t, "a patch is denied", want, g.fire(g.env(), "patch", "src/main.go"))
	eq(t, "bash is not gated", "allow", g.fire(g.env(), "bash", "src/main.go"))
}

func TestOpenCodePlanGateAllowsAfterTheDeckIsBuilt(t *testing.T) {
	g := newOCPlanGate(t)
	g.ackDeck()
	eq(t, "with a deck the edit passes", "allow", g.fire(g.env(), "edit", "src/main.go"))
	write(t, filepath.Join(g.root, "odd", "tasks", "x.md"), ocPlan+"\n## T2 — new\n- [ ] more\n")
	isTrue(t, "a new section asks for a new deck", strings.HasPrefix(g.fire(g.env(), "edit", "src/main.go"), "deny "))
}

func TestOpenCodePlanGateAllowsOddAndDeckJSON(t *testing.T) {
	g := newOCPlanGate(t)
	eq(t, "the plan itself", "allow", g.fire(g.env(), "edit", "odd/tasks/y.md"))
	eq(t, "the deck spec", "allow", g.fire(g.env(), "write", "docs/a.deck.json"))
}

func TestOpenCodePlanGateFailsOpenWithoutAval(t *testing.T) {
	g := newOCPlanGate(t)
	// A PATH without aval; see TestRateGuardOpenCodePlugin for Windows.
	noAvalPath := "/usr/bin:/bin"
	if runtime.GOOS == "windows" {
		noAvalPath = os.Getenv("PATH")
	}
	isTrue(t, "the scratch HOME has no ~/.local/bin/aval", !exists(filepath.Join(g.home, ".local", "bin", "aval")))
	eq(t, "with no aval binary the plugin fails open", "allow",
		g.fire(withEnv(isoEnv(g.home), "PATH="+noAvalPath), "edit", "src/main.go"))
}

func TestOpenCodePlanGateHonoursAvalPlanDeckOff(t *testing.T) {
	g := newOCPlanGate(t)
	eq(t, "AVAL_PLAN_DECK_OFF bypasses the gate", "allow", g.fire(g.env("AVAL_PLAN_DECK_OFF=1"), "edit", "src/main.go"))
}
