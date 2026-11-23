package tests

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// Fake-agent end to end: play a scripted Claude Code session against the REAL
// `aval hook ...` commands, wired the way config/claude/settings.json wires
// them. The design_pipeline tests call each hook directly; this one proves the
// wiring too: the matcher in settings.json decides whether a hook runs at all,
// the command string is executed as configured, and the JSON on stdout is the
// exact decision Claude Code would act on. No API keys, no network, no real
// $HOME/$XDG: everything lives in a temp dir.
//
// Stays a real process: the hooks run as the shell-form commands in
// settings.json, through the binary installed at ~/.local/bin/aval.
//
// Only the design-pipeline hooks (`aval hook design-*`) are fired; third-party
// hooks in the same settings (code-review-graph, rtk) are not ours to exercise.

type fakeAgent struct {
	t     *testing.T
	home  string
	proj  string
	env   []string
	hooks settingsDoc
}

func newFakeAgent(t *testing.T) *fakeAgent {
	needTool(t, "git", "sh")
	aval := avalBin(t)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	home := filepath.Join(root, "home")
	for _, d := range []string{".claude", ".local/bin", ".cache"} {
		_ = os.MkdirAll(filepath.Join(home, d), 0o755)
	}
	// What the installer puts on PATH.
	bin, _ := os.Open(aval)
	dst, err := os.OpenFile(filepath.Join(home, ".local/bin/aval"), os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(dst, bin)
	_ = dst.Close()
	_ = bin.Close()

	proj := filepath.Join(root, "app")
	_ = os.MkdirAll(filepath.Join(proj, "src/components"), 0o755)
	git(t, proj, "init", "-q")
	write(t, proj+"/.agents/DESIGN.md", "# Design\nTokens only.\n")
	proj, _ = filepath.EvalSymlinks(proj)

	return &fakeAgent{t: t, home: home, proj: proj, hooks: repoSettings(t),
		env: isoEnv(home, "PATH="+filepath.Join(home, ".local/bin")+":"+os.Getenv("PATH"))}
}

// fire runs every repo hook configured for the event whose matcher matches the
// tool, exactly like Claude Code, and returns their combined stdout.
func (a *fakeAgent) fire(event, tool, payload string) string {
	var out strings.Builder
	for _, g := range a.hooks.Hooks[event] {
		for _, h := range g.Hooks {
			if !strings.Contains(h.Command, "hook design-") {
				continue
			}
			if g.Matcher != "" && !regexp.MustCompile("^("+g.Matcher+")$").MatchString(tool) {
				continue
			}
			out.WriteString(run(a.proj, a.env, payload, "sh", "-c", h.Command).Stdout)
		}
	}
	return out.String()
}

func (a *fakeAgent) prePayload(tool, file, session, transcript string) string {
	if transcript == "" {
		transcript = filepath.Join(a.home, "main.jsonl")
	}
	b, _ := json.Marshal(map[string]any{
		"session_id": session, "transcript_path": transcript, "cwd": a.proj,
		"hook_event_name": "PreToolUse", "tool_name": tool, "tool_input": map[string]any{"file_path": file}})
	return string(b)
}

func (a *fakeAgent) pre(tool, file, session string) string {
	return a.fire("PreToolUse", tool, a.prePayload(tool, file, session, ""))
}

func (a *fakeAgent) stop(session string, active bool) string {
	b, _ := json.Marshal(map[string]any{"session_id": session, "transcript_path": "/x.jsonl", "cwd": a.proj,
		"hook_event_name": "Stop", "stop_hook_active": active})
	return a.fire("Stop", "", string(b))
}

func (a *fakeAgent) ack(surface string) {
	run(a.proj, a.env, "", filepath.Join(a.home, ".local/bin/aval"), "design", "ack", surface)
}

func jsonEq(t *testing.T, what, want, got string) {
	t.Helper()
	var w, g any
	_ = json.Unmarshal([]byte(want), &w)
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Errorf("%s: output is not JSON: %q", what, got)
		return
	}
	if !reflect.DeepEqual(w, g) {
		t.Errorf("%s:\n  want: %s\n  got:  %s", what, want, got)
	}
}

func denyJSON(file, why string) string {
	b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse", "permissionDecision": "deny",
		"permissionDecisionReason": "Blocked: " + file + " is a UI surface and " + why +
			". Do the design pipeline (invoke the design-pipeline skill if you have not: laws of UX, direction from .agents/DESIGN.md, gates), then run `aval design ack <surface>` and retry."}})
	return string(b)
}

func TestHooksE2ESettingsWiring(t *testing.T) {
	s := repoSettings(t)
	var matcher string
	for _, g := range s.Hooks["PreToolUse"] {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "hook design-gate") {
				matcher = g.Matcher
				break
			}
		}
		if matcher != "" {
			break
		}
	}
	eq(t, "settings.json wires the gate on PreToolUse", "Edit|Write|NotebookEdit", matcher)
	isTrue(t, "settings.json wires the shot hook on Stop", hasHook(s, "Stop", "hook design-shot-stop"))
}

func TestHooksE2EScriptedSession(t *testing.T) {
	testenv.SkipWindows(t, "runs the shell-form hook commands through sh; Windows uses exec-form hooks (internal/core/sync/winhooks_test.go)")
	t.Parallel()
	a := newFakeAgent(t)
	btn := filepath.Join(a.proj, "src/components/Button.tsx")
	const main = "s-main"
	stamp := filepath.Join(a.home, ".cache/design-gate/ui-edited."+main)

	// turn 1: the agent edits a UI file before the pipeline
	jsonEq(t, "Edit of a .tsx with no design-ack is denied (exact JSON)",
		denyJSON("Button.tsx", "no design-ack for this project yet"), a.pre("Edit", btn, main))
	eq(t, "a Bash tool call never reaches the gate (matcher)", "", a.pre("Bash", btn, main))

	// the agent runs design-ack and retries
	a.ack("pricing")
	ents, _ := os.ReadDir(filepath.Join(a.home, ".cache/design-gate"))
	isTrue(t, "design-ack wrote a marker under the temp cache", len(ents) > 0)
	eq(t, "after design-ack the same Edit is allowed (no output)", "", a.pre("Edit", btn, main))
	eq(t, "...and the session is stamped for the Stop hook", a.proj, strings.TrimSpace(readAbsOrEmpty(stamp)))

	// a sub-agent: another session and transcript
	sub := a.fire("PreToolUse", "Write", a.prePayload("Write", btn, "agent-7", filepath.Join(a.home, "subagents/agent-7.jsonl")))
	eq(t, "a sub-agent edit (different session + transcript) is allowed", "", sub)

	// non-UI edits
	eq(t, "a non-UI edit is allowed", "", a.pre("Edit", filepath.Join(a.proj, "src/util.ts"), main))
	eq(t, "a test file is allowed (TDD writes it first)", "", a.pre("Write", filepath.Join(a.proj, "src/components/Button.test.tsx"), main))

	// editing the design contract invalidates the ack
	write(t, a.proj+"/.agents/DESIGN.md", "# Design\nTokens only.\nNew rule.\n")
	jsonEq(t, "changing DESIGN.md revokes the ack (exact JSON)",
		denyJSON("Button.tsx", ".agents/DESIGN.md changed since the last design-ack"), a.pre("Edit", btn, main))
	a.ack("pricing")
	eq(t, "a fresh design-ack allows it again", "", a.pre("Edit", btn, main))

	// end of turn
	stopBlock := `{"decision":"block","reason":"UI files were edited this session but .design/shots has no render newer than the last edit. Run ` + "`node ~/.claude/skills/design-pipeline/scripts/shot.mjs <url|file.html>`" + ` and quote its paths and numbers, or say \"not visually verified\" in your reply."}`
	jsonEq(t, "Stop after a UI edit with no shots blocks (exact JSON)", stopBlock, a.stop(main, false))
	eq(t, "the second Stop is allowed (blocks once)", "", a.stop(main, false))
	eq(t, "Stop with stop_hook_active never blocks", "", a.stop(main, true))
	eq(t, "Stop in a session with no UI edit is silent", "", a.stop("s-untouched", false))

	// A later UI edit re-arms the block; a render newer than the edit satisfies it.
	// The stamp must outrank the "blocked once" record by mtime. Age the record
	// rather than waiting out the filesystem's timestamp granularity.
	blocked, _ := filepath.Glob(filepath.Join(a.home, ".cache/design-gate/*.blocked"))
	isTrue(t, "the first Stop left a blocked record", len(blocked) > 0)
	past := time.Now().Add(-time.Hour)
	for _, f := range blocked {
		_ = os.Chtimes(f, past, past)
	}
	eq(t, "a later UI edit is still allowed", "", a.pre("Edit", btn, main))
	eq(t, "...and re-arms the Stop block", "block", stopDecision(a.stop(main, false)))
	shots := filepath.Join(a.proj, ".design/shots")
	_ = os.MkdirAll(shots, 0o755)
	write(t, shots+"/home-390.png", "")
	future := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = os.Chtimes(shots+"/home-390.png", future, future)
	eq(t, "Stop with a render newer than the edit is allowed", "", a.stop(main, false))
}

// A non-UI edit runs before every Edit/Write, so its cost is paid constantly.
func TestHooksE2ENonUIEditCostsUnder150msThroughTheGate(t *testing.T) {
	testenv.SkipWindows(t, "runs the shell-form hook commands through sh; Windows uses exec-form hooks (internal/core/sync/winhooks_test.go)")
	slow(t)
	a := newFakeAgent(t)
	payload := a.prePayload("Edit", filepath.Join(a.proj, "src/util.ts"), "s-main", "")
	bin := filepath.Join(a.home, ".local/bin/aval")
	start := time.Now()
	for i := 0; i < 5; i++ {
		run(a.proj, a.env, payload, bin, "hook", "design-gate")
	}
	per := time.Since(start) / 5
	isTrue(t, "a non-UI edit costs under 150 ms through the gate (was "+per.String()+")", per < 150*time.Millisecond)
}
