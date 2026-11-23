package main

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
// driven in process through run. Moved from tests/design_pipeline_test.go,
// which keeps one real-binary smoke test per entry point (argv, exit code,
// an empty PATH, the git toplevel seen from a real working directory).
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
}

func newDG(t *testing.T) *dgEnv {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &dgEnv{t: t, root: root, cache: filepath.Join(root, "cache")}
}

// env is the whole environment of one call: HOME and the cache confined to
// the temp root, plus KEY=VALUE overrides.
func (d *dgEnv) env(kv ...string) map[string]string {
	m := map[string]string{"HOME": d.root, "USERPROFILE": d.root, "XDG_CACHE_HOME": d.cache}
	for _, p := range kv {
		k, v, _ := strings.Cut(p, "=")
		m[k] = v
	}
	return m
}

func (d *dgEnv) project(name string, designMD string) string {
	d.t.Helper()
	dir := filepath.Join(d.root, name)
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		d.t.Fatal(err)
	}
	if designMD != "" {
		writeFile(d.t, filepath.Join(dir, ".agents", "DESIGN.md"), designMD)
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

// ack runs `aval design ack` from project: ack reads the working directory,
// so the test changes into it (t.Chdir restores it at the end).
func (d *dgEnv) ack(project, surface string, kv ...string) invocation {
	d.t.Helper()
	args := []string{"design", "ack"}
	if surface != "" {
		args = append(args, surface)
	}
	d.t.Chdir(project)
	return runIn(d.t, d.env(kv...), "", args...)
}

func (d *dgEnv) rewriteEpoch(marker string, epoch int64) {
	b := readFile(d.t, marker)
	b = regexp.MustCompile(`(?m)^epoch=.*$`).ReplaceAllString(b, "epoch="+strconv.FormatInt(epoch, 10))
	writeFile(d.t, marker, b)
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
	return runIn(d.t, d.env(c.env...), string(payload), "hook", "design-gate").stdout
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func eq[T comparable](t *testing.T, what string, want, got T) {
	t.Helper()
	if want != got {
		t.Errorf("%s:\n  want: %v\n  got:  %v", what, want, got)
	}
}

func TestDesignAckWritesTheProjectMarker(t *testing.T) {
	d := newDG(t)
	p1 := d.project("p1", "# DESIGN\n")
	r := d.ack(p1, "dashboard")
	m := d.marker(p1)
	if !fileExists(m) {
		t.Fatal("design-ack writes the project marker")
	}
	body := readFile(t, m)
	for what, re := range map[string]string{
		"marker records the surface":          `(?m)^surface=dashboard$`,
		"marker records an epoch time":        `(?m)^epoch=\d{10}$`,
		"marker records the DESIGN.md sha1":   `(?m)^design_sha1=` + sha1Of("# DESIGN\n") + `$`,
		"design-ack reminder names laws":      `(?i)laws`,
		"design-ack reminder names direction": `(?i)direction`,
		"design-ack reminder names gates":     `(?i)gates`,
	} {
		in := body
		if strings.HasPrefix(what, "design-ack reminder") {
			in = r.stdout + r.stderr
		}
		if !regexp.MustCompile(re).MatchString(in) {
			t.Errorf("%s: /%s/ not found", what, re)
		}
	}
	if d.ack(p1, "").code == 0 {
		t.Error("design-ack without a surface fails")
	}
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
	writeFile(t, p4+"/.agents/DESIGN.md", "tokens v2\n")
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
}

func TestDesignGateBypassAndFailOpen(t *testing.T) {
	d := newDG(t)
	p5 := d.project("p5", "")
	c := edit(p5, "src/Button.tsx")
	c.env = []string{"DESIGN_PIPELINE_OFF=1"}
	eq(t, "DESIGN_PIPELINE_OFF bypasses the gate", "none", decision(d.hook(c)))

	for _, bad := range []string{"", "not json at all", `{"tool_name":"Edit","tool_input":{"file_path":`, `{"tool_name":"Edit"}`, `{}`} {
		r := runIn(t, d.env(), bad, "hook", "design-gate")
		label := bad
		if label == "" {
			label = "<empty>"
		}
		eq(t, "malformed stdin fails open (exit): "+label, 0, r.code)
		eq(t, "malformed stdin fails open (output): "+label, "", r.stdout)
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
	if fileExists(stamp) {
		t.Error("a denied UI edit leaves no stamp")
	}
	nonUI := edit(p5, "main.go")
	nonUI.session = "sess-stamp"
	d.hook(nonUI)
	if fileExists(stamp) {
		t.Error("a non-UI edit leaves no stamp")
	}
	allowed := edit(p3, "src/Button.tsx")
	allowed.session = "sess-stamp"
	d.hook(allowed)
	if !fileExists(stamp) {
		t.Fatal("an allowed UI edit stamps the session")
	}
	eq(t, "the stamp names the project root", p3, strings.TrimSpace(readFile(t, stamp)))
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
	for _, want := range []string{"aval design ack <surface>", "design-pipeline"} {
		if !strings.Contains(out.H.Reason, want) {
			t.Errorf("denial reason %q does not name %q", out.H.Reason, want)
		}
	}
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
	return runIn(s.t, s.env(kv...), string(payload), "hook", "design-shot-stop").stdout
}

func (s *stopEnv) stamp(session string, at time.Time) {
	f := filepath.Join(s.gate, "ui-edited."+session)
	writeFile(s.t, f, s.proj)
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
	for _, want := range []string{"shot.mjs", "not visually verified"} {
		if !strings.Contains(v.Reason, want) {
			t.Errorf("the reason %q does not name %q", v.Reason, want)
		}
	}
	eq(t, "it blocks only ONCE per stamp", "none", stopDecision(s.stop("s1", false)))

	// A later UI edit re-stamps; the block is allowed again, once.
	now := time.Now()
	_ = os.Chtimes(filepath.Join(s.gate, "ui-edited.s1"), now, now)
	_ = os.Chtimes(filepath.Join(s.gate, "ui-edited.s1.blocked"), oldTime, oldTime)
	eq(t, "a newer stamp blocks again", "block", stopDecision(s.stop("s1", false)))
	eq(t, "...and only once again", "none", stopDecision(s.stop("s1", false)))

	// A shot newer than the stamp satisfies it.
	s.stamp("s2", oldTime)
	writeFile(t, filepath.Join(s.proj, ".design/shots/home-390.png"), "")
	eq(t, "a render newer than the stamp: silent", "none", stopDecision(s.stop("s2", false)))

	// A shot older than the stamp does not.
	shots := filepath.Join(s.proj, ".design/shots")
	ents, _ := os.ReadDir(shots)
	for _, e := range ents {
		_ = os.Remove(filepath.Join(shots, e.Name()))
	}
	writeFile(t, shots+"/old-390.png", "")
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
		r := runIn(t, s.env(), bad, "hook", "design-shot-stop")
		eq(t, "malformed stdin exits 0: "+bad, 0, r.code)
		eq(t, "malformed stdin is silent: "+bad, "", strings.TrimSpace(r.stdout))
	}
}
