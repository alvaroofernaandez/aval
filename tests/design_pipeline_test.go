package tests

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The design-pipeline gate (`aval hook design-gate`), its marker
// (`aval design ack`) and the Stop hook (`aval hook design-shot-stop`),
// exercised through the real binary.
//
// The same decision tables are unit-tested in internal/hook and
// internal/ack; what only an end-to-end run proves is the process
// boundary: stdin parsing, exit codes, fail-open, a binary that runs with an
// empty PATH, and a marker path computed independently of the code under test.
//
// The gate reads a MARKER: `aval design ack <surface>` writes
// ${XDG_CACHE_HOME:-~/.cache}/design-gate/<sha1 of project root>, and a UI
// edit is allowed while it is fresh (4 h) and .agents/DESIGN.md still hashes
// the same. It is per project, so sub-agents and post-compaction turns work
// with no transcript.

type dgEnv struct {
	t     *testing.T
	root  string // temp root; every project lives under it
	cache string
	aval  string
}

func newDG(t *testing.T) *dgEnv {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &dgEnv{t: t, root: root, cache: filepath.Join(root, "cache"), aval: avalBin(t)}
}

func (d *dgEnv) env(kv ...string) []string {
	return isoEnv(d.root, append([]string{"XDG_CACHE_HOME=" + d.cache}, kv...)...)
}

func (d *dgEnv) project(name string, designMD string) string {
	d.t.Helper()
	dir := filepath.Join(d.root, name)
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		d.t.Fatal(err)
	}
	if designMD != "" {
		write(d.t, filepath.Join(dir, ".agents", "DESIGN.md"), designMD)
	}
	return dir
}

func sha1Of(s string) string { h := sha1.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

// marker is computed independently of the code under test.
func (d *dgEnv) marker(project string) string {
	real, err := filepath.EvalSymlinks(project)
	if err != nil {
		d.t.Fatal(err)
	}
	return filepath.Join(d.cache, "design-gate", sha1Of(real))
}

func (d *dgEnv) ack(project, surface string, kv ...string) result {
	args := []string{"design", "ack"}
	if surface != "" {
		args = append(args, surface)
	}
	return run(project, d.env(kv...), "", d.aval, args...)
}

func (d *dgEnv) rewriteEpoch(marker string, epoch int64) {
	b := readAbs(d.t, marker)
	b = regexp.MustCompile(`(?m)^epoch=.*$`).ReplaceAllString(b, "epoch="+strconv.FormatInt(epoch, 10))
	write(d.t, marker, b)
}

type preCall struct {
	tool, file, cwd, transcript, session string
	env                                  []string
}

func (d *dgEnv) hook(c preCall) string {
	if c.transcript == "" {
		c.transcript = filepath.Join(d.root, "transcript.jsonl")
	}
	if c.session == "" {
		c.session = "sess-1"
	}
	payload, _ := json.Marshal(map[string]any{
		"session_id": c.session, "transcript_path": c.transcript, "cwd": c.cwd,
		"hook_event_name": "PreToolUse", "tool_name": c.tool,
		"tool_input": map[string]any{"file_path": c.file},
	})
	return run(c.cwd, d.env(c.env...), string(payload), d.aval, "hook", "design-gate").Stdout
}

func decision(out string) string {
	if strings.TrimSpace(out) == "" {
		return "none"
	}
	var v struct {
		H struct {
			D string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	if json.Unmarshal([]byte(out), &v) != nil || v.H.D == "" {
		return "none"
	}
	return v.H.D
}

func edit(project, file string) preCall {
	return preCall{tool: "Edit", file: filepath.Join(project, file), cwd: project}
}

func TestDesignAckWritesTheProjectMarker(t *testing.T) {
	d := newDG(t)
	p1 := d.project("p1", "# DESIGN\n")
	r := d.ack(p1, "dashboard")
	m := d.marker(p1)
	isTrue(t, "design-ack writes the project marker", exists(m))
	body := readAbs(t, m)
	mustHave(t, `^surface=dashboard$`, body, "marker records the surface")
	mustHave(t, `^epoch=\d{10}$`, body, "marker records an epoch time")
	mustHave(t, `^design_sha1=`+sha1Of("# DESIGN\n")+`$`, body, "marker records the DESIGN.md sha1")
	for _, w := range []string{"laws", "direction", "gates"} {
		mustHave(t, `(?i)`+w, r.Out(), "design-ack reminder names "+w)
	}
	isTrue(t, "design-ack without a surface fails", d.ack(p1, "").Code != 0)
}

// The project root is the git toplevel of cwd, not cwd itself.
func TestDesignAckFromASubdirectoryMarksTheGitToplevel(t *testing.T) {
	needTool(t, "git")
	d := newDG(t)
	p2 := d.project("p2", "")
	deep := filepath.Join(p2, "src", "deep")
	_ = os.MkdirAll(deep, 0o755)
	git(t, p2, "init", "-q")
	d.ack(deep, "card")
	top := git(t, p2, "rev-parse", "--show-toplevel")
	isTrue(t, "design-ack from a subdirectory marks the git toplevel", exists(d.marker(top)))
}

func TestDesignGateEnforcement(t *testing.T) {
	d := newDG(t)
	p3 := d.project("p3", "")
	btn := edit(p3, "src/Button.tsx")

	eq(t, "UI file with no marker is denied", "deny", decision(d.hook(btn)))

	d.ack(p3, "component")
	eq(t, "UI file with a fresh marker is allowed", "none", decision(d.hook(btn)))
	css := preCall{tool: "Write", file: filepath.Join(p3, "src/app.css"), cwd: p3}
	eq(t, "a stylesheet is covered by the same marker", "none", decision(d.hook(css)))

	// inside the default window: three hours old is still fresh (guards the 4 h default).
	m3 := d.marker(p3)
	now := time.Now().Unix()
	d.rewriteEpoch(m3, now-3*3600)
	eq(t, "a marker three hours old is still inside the default window", "none", decision(d.hook(btn)))

	// stale: the epoch rewritten to five hours ago (the TTL default is four).
	d.rewriteEpoch(m3, now-18000)
	eq(t, "a marker older than 4 hours is denied", "deny", decision(d.hook(btn)))
	long := btn
	long.env = []string{"DESIGN_ACK_TTL=86400"}
	eq(t, "DESIGN_ACK_TTL can lengthen the window", "none", decision(d.hook(long)))
	d.rewriteEpoch(m3, now-30)
	short := btn
	short.env = []string{"DESIGN_ACK_TTL=5"}
	eq(t, "DESIGN_ACK_TTL can shorten the window", "deny", decision(d.hook(short)))

	// DESIGN.md edited after the ack means the direction changed under the code.
	p4 := d.project("p4", "tokens v1\n")
	d.ack(p4, "component")
	b4 := edit(p4, "src/Button.tsx")
	eq(t, "an unchanged DESIGN.md keeps the marker valid", "none", decision(d.hook(b4)))
	write(t, p4+"/.agents/DESIGN.md", "tokens v2\n")
	eq(t, "DESIGN.md changed after the ack is denied", "deny", decision(d.hook(b4)))
	d.ack(p4, "component")
	eq(t, "re-running design-ack reopens the gate", "none", decision(d.hook(b4)))

	// The marker is per project: acking p3 says nothing about p5.
	p5 := d.project("p5", "")
	d.rewriteEpoch(m3, time.Now().Unix())
	eq(t, "another project's marker does not open this one", "deny", decision(d.hook(edit(p5, "src/Button.tsx"))))

	// A sub-agent call carries another transcript (or none); the marker is all
	// that matters, which is why sub-agents need no special case.
	sub := btn
	sub.transcript, sub.session = filepath.Join(d.root, "agent-xyz/subagents/agent-1.jsonl"), "other-session"
	eq(t, "a sub-agent style call with another transcript is allowed", "none", decision(d.hook(sub)))
	gone := btn
	gone.transcript = "/nonexistent/t.jsonl"
	eq(t, "a call with a missing transcript is allowed too", "none", decision(d.hook(gone)))
}

func TestDesignGateWhatIsNotADesignSurface(t *testing.T) {
	d := newDG(t)
	p5 := d.project("p5", "")
	eq(t, "non-UI source with no marker is allowed", "none", decision(d.hook(edit(p5, "main.go"))))
	for _, f := range []string{"src/Button.test.tsx", "src/Button.spec.tsx", "src/__tests__/Button.tsx", "node_modules/x/y.css"} {
		c := preCall{tool: "Write", file: filepath.Join(p5, f), cwd: p5}
		eq(t, "exempt path "+f+" is allowed with no marker", "none", decision(d.hook(c)))
	}
	c := edit(p5, "src/Button.tsx")
	c.tool = "Bash"
	eq(t, "tools other than Edit/Write pass through", "none", decision(d.hook(c)))

	// A non-UI edit needs nothing but the binary: no jq, no git, no hash tool.
	body := `{"tool_name":"Edit","tool_input":{"file_path":"` + p5 + `/main.go"},"cwd":"/x"}`
	r := run(p5, withEnv(d.env(), "PATH=/nonexistent"), body, d.aval, "hook", "design-gate")
	eq(t, "a non-UI edit runs with an empty PATH (exit)", 0, r.Code)
	eq(t, "...and prints nothing", "", r.Out())
}

func TestDesignGateBypassAndFailOpen(t *testing.T) {
	d := newDG(t)
	p5 := d.project("p5", "")
	c := edit(p5, "src/Button.tsx")
	c.env = []string{"DESIGN_PIPELINE_OFF=1"}
	eq(t, "DESIGN_PIPELINE_OFF bypasses the gate", "none", decision(d.hook(c)))

	for _, bad := range []string{"", "not json at all", `{"tool_name":"Edit","tool_input":{"file_path":`, `{"tool_name":"Edit"}`, `{}`} {
		r := run(p5, d.env(), bad, d.aval, "hook", "design-gate")
		label := bad
		if label == "" {
			label = "<empty>"
		}
		eq(t, "malformed stdin fails open (exit): "+label, 0, r.Code)
		eq(t, "malformed stdin fails open (output): "+label, "", r.Stdout)
	}
}

// The ui-edited stamp the Stop hook reads.
func TestDesignGateStampsTheSessionOnAllowedUIEdits(t *testing.T) {
	d := newDG(t)
	p3 := d.project("p3", "")
	p5 := d.project("p5", "")
	d.ack(p3, "component")
	stamp := filepath.Join(d.cache, "design-gate", "ui-edited.sess-stamp")

	denied := edit(p5, "src/Button.tsx")
	denied.session = "sess-stamp"
	d.hook(denied)
	isTrue(t, "a denied UI edit leaves no stamp", !exists(stamp))
	nonUI := edit(p5, "main.go")
	nonUI.session = "sess-stamp"
	d.hook(nonUI)
	isTrue(t, "a non-UI edit leaves no stamp", !exists(stamp))
	allowed := edit(p3, "src/Button.tsx")
	allowed.session = "sess-stamp"
	d.hook(allowed)
	isTrue(t, "an allowed UI edit stamps the session", exists(stamp))
	eq(t, "the stamp names the project root", p3, strings.TrimSpace(readAbsOrEmpty(stamp)))
}

// The denial has to teach, in one line.
func TestDesignGateDenialTeaches(t *testing.T) {
	d := newDG(t)
	p5 := d.project("p5", "")
	var out struct {
		H struct {
			Reason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(d.hook(edit(p5, "src/Button.tsx"))), &out); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.H.Reason, "aval design ack <surface>", "denial reason names design-ack")
	mustContain(t, out.H.Reason, "design-pipeline", "denial reason names the design-pipeline skill")
	eq(t, "denial reason is a single line", 1, len(strings.Split(strings.TrimRight(out.H.Reason, "\n"), "\n")))
}

// --- the Stop hook that asks for a render before a UI turn ends ---------------
//
// Measured over 60 days: about one user message in five was a screenshot saying
// a "fixed" visual bug was not fixed. The gate stamps
// ${cache}/design-gate/ui-edited.<session_id> (content: project root) when it
// allows a UI edit; the Stop hook blocks ONCE per stamp when .design/shots of
// that project holds nothing newer.

type stopEnv struct {
	*dgEnv
	gate string
	proj string
}

func newStop(t *testing.T) *stopEnv {
	d := newDG(t)
	s := &stopEnv{dgEnv: d, gate: filepath.Join(d.cache, "design-gate")}
	_ = os.MkdirAll(s.gate, 0o755)
	s.proj = filepath.Join(d.root, "proj")
	_ = os.MkdirAll(filepath.Join(s.proj, ".design", "shots"), 0o755)
	return s
}

var oldTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func (s *stopEnv) stop(session string, active bool, kv ...string) string {
	payload, _ := json.Marshal(map[string]any{
		"session_id": session, "hook_event_name": "Stop", "stop_hook_active": active, "cwd": "/x"})
	return run(s.root, s.env(kv...), string(payload), s.aval, "hook", "design-shot-stop").Stdout
}

func (s *stopEnv) stamp(session string, at time.Time) {
	f := filepath.Join(s.gate, "ui-edited."+session)
	write(s.t, f, s.proj)
	if err := os.Chtimes(f, at, at); err != nil {
		s.t.Fatal(err)
	}
}

func stopDecision(out string) string {
	if strings.TrimSpace(out) == "" {
		return "none"
	}
	var v struct {
		Decision string `json:"decision"`
	}
	if json.Unmarshal([]byte(out), &v) != nil || v.Decision == "" {
		return "none"
	}
	return v.Decision
}

func TestDesignShotStop(t *testing.T) {
	s := newStop(t)
	eq(t, "no UI edit this session: silent", "none", stopDecision(s.stop("s-none", false)))

	s.stamp("s1", oldTime)
	out := s.stop("s1", false)
	eq(t, "UI edited and no shot: block", "block", stopDecision(out))
	var v struct{ Reason string }
	_ = json.Unmarshal([]byte(out), &v)
	mustContain(t, v.Reason, "shot.mjs", "the reason names shot.mjs")
	mustContain(t, v.Reason, "not visually verified", "the reason offers the honest alternative")
	eq(t, "it blocks only ONCE per stamp", "none", stopDecision(s.stop("s1", false)))

	// A later UI edit re-stamps; the block is allowed again, once.
	now := time.Now()
	_ = os.Chtimes(filepath.Join(s.gate, "ui-edited.s1"), now, now)
	_ = os.Chtimes(filepath.Join(s.gate, "ui-edited.s1.blocked"), oldTime, oldTime)
	eq(t, "a newer stamp blocks again", "block", stopDecision(s.stop("s1", false)))
	eq(t, "...and only once again", "none", stopDecision(s.stop("s1", false)))

	// A shot newer than the stamp satisfies it.
	s.stamp("s2", oldTime)
	write(t, filepath.Join(s.proj, ".design/shots/home-390.png"), "")
	eq(t, "a render newer than the stamp: silent", "none", stopDecision(s.stop("s2", false)))

	// A shot older than the stamp does not.
	shots := filepath.Join(s.proj, ".design/shots")
	ents, _ := os.ReadDir(shots)
	for _, e := range ents {
		_ = os.Remove(filepath.Join(shots, e.Name()))
	}
	write(t, shots+"/old-390.png", "")
	_ = os.Chtimes(shots+"/old-390.png", oldTime, oldTime)
	s.stamp("s3", oldTime.Add(24*time.Hour))
	eq(t, "a render older than the stamp still blocks", "block", stopDecision(s.stop("s3", false)))

	s.stamp("s4", oldTime)
	eq(t, "stop_hook_active never blocks (no loops)", "none", stopDecision(s.stop("s4", true)))
	s.stamp("s5", oldTime)
	eq(t, "DESIGN_PIPELINE_OFF is silent", "none", stopDecision(s.stop("s5", false, "DESIGN_PIPELINE_OFF=1")))
}

func TestDesignShotStopMalformedInputIsSilent(t *testing.T) {
	s := newStop(t)
	for _, bad := range []string{"", "garbage", `{"hook_event_name":"Stop"}`} {
		r := run(s.root, s.env(), bad, s.aval, "hook", "design-shot-stop")
		eq(t, "malformed stdin exits 0: "+bad, 0, r.Code)
		eq(t, "malformed stdin is silent: "+bad, "", strings.TrimSpace(r.Stdout))
	}
	// The common case (no UI edit this session) needs nothing but the binary.
	r := run(s.root, withEnv(s.env(), "PATH=/nonexistent"), `{"session_id":"s-none","stop_hook_active":false}`, s.aval, "hook", "design-shot-stop")
	eq(t, "the no-edit path runs with an empty PATH (exit)", 0, r.Code)
	eq(t, "...and prints nothing", "", strings.TrimSpace(r.Out()))
}
