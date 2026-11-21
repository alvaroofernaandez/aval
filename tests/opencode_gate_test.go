package tests

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The OpenCode port of the design-pipeline gate.
//
// The Claude Code gate is a PreToolUse hook (`aval hook design-gate`); OpenCode
// has no JSON hook system, so only the contract is shared, and the contract is
// a MARKER: `aval design ack <surface>` writes
// ${XDG_CACHE_HOME:-~/.cache}/design-gate/<sha1 of project root>, and a UI edit
// is allowed while that marker is younger than DESIGN_ACK_TTL seconds
// (default 4 h) and .agents/DESIGN.md still hashes the same. The plugin reads
// the same file, so one `aval design ack` opens both agents.

type ocGate struct {
	*dgEnv
	probe string
}

func newOCGate(t *testing.T) *ocGate {
	needTool(t, "bun")
	plugin := p("config", "opencode", "plugin", "design-gate.ts")
	isTrue(t, "the plugin exists", exists(plugin))
	d := newDG(t)
	// The fixture imports the plugin by a path relative to the repo; point it at
	// the repo under test.
	src := read(t, "tests/testdata/opencode-gate-probe.ts")
	src = regexp.MustCompile(`from "[^"]*design-gate\.ts"`).ReplaceAllString(src, `from "`+plugin+`"`)
	probe := filepath.Join(d.root, "probe", "opencode-gate-probe.ts")
	write(t, probe, src)
	return &ocGate{dgEnv: d, probe: probe}
}

func (g *ocGate) ui(path string) string {
	return strings.TrimSpace(run(g.root, g.env(), "", "bun", "run", g.probe, "ui", path).Stdout)
}

func (g *ocGate) edit(file, dir, session string, kv ...string) string {
	args := []string{"run", g.probe, "edit", file, dir}
	if session != "" {
		args = append(args, session)
	}
	return strings.TrimSpace(run(g.root, g.env(kv...), "", "bun", args...).Stdout)
}

func TestOpenCodeGateUISurfacePredicate(t *testing.T) {
	g := newOCGate(t)
	eq(t, "a .tsx is a UI surface", "true", g.ui("src/Button.tsx"))
	eq(t, "a .css is a UI surface", "true", g.ui("app/main.css"))
	eq(t, "a .go file is not", "false", g.ui("cmd/main.go"))
	eq(t, "a component test is not", "false", g.ui("src/Button.test.tsx"))
}

func TestOpenCodeGateDecisions(t *testing.T) {
	g := newOCGate(t)
	P := g.project("proj", "tokens v1\n")
	btn := P + "/src/Button.tsx"

	eq(t, "a UI write with no marker is denied, in one line", "deny 1", g.edit(btn, P, ""))

	g.ack(P, "component")
	eq(t, "a UI write with a fresh marker is allowed", "allow", g.edit(btn, P, ""))
	eq(t, "the session id is irrelevant (sub-agents)", "allow", g.edit(btn, P, "other-session"))

	g.rewriteEpoch(g.marker(P), time.Now().Unix()-18000)
	eq(t, "a marker older than 4 hours is denied", "deny 1", g.edit(btn, P, ""))
	eq(t, "DESIGN_ACK_TTL lengthens the window", "allow", g.edit(btn, P, "", "DESIGN_ACK_TTL=86400"))

	g.ack(P, "component")
	write(t, P+"/.agents/DESIGN.md", "tokens v2\n")
	eq(t, "DESIGN.md changed after the ack is denied", "deny 1", g.edit(btn, P, ""))

	eq(t, "a non-UI write needs no marker", "allow", g.edit(P+"/main.go", P, ""))
	eq(t, "a component test needs no marker", "allow", g.edit(P+"/src/Button.test.tsx", P, ""))
	eq(t, "DESIGN_PIPELINE_OFF bypasses", "allow", g.edit(btn, P, "", "DESIGN_PIPELINE_OFF=1"))
}

// Same marker file as the Go hook: the project root is the git toplevel found
// by walking up from the file, so a nested file resolves to the repo's marker.
func TestOpenCodeGateNestedFileResolvesToTheGitToplevelMarker(t *testing.T) {
	needTool(t, "git")
	g := newOCGate(t)
	G := g.project("repo", "")
	deep := G + "/src/deep"
	write(t, deep+"/.keep", "")
	git(t, G, "init", "-q")
	g.ack(G, "component")
	eq(t, "a nested file resolves to the git toplevel marker", "allow", g.edit(deep+"/Card.tsx", deep, ""))
}
