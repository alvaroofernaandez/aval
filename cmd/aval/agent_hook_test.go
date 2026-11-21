package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// q is a JSON string literal: Windows paths hold backslashes, so a path cannot
// be pasted between quotes.
func q(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestRunHookLessonsUsesEnvDirsAndXDGState(t *testing.T) {
	dir, state, home := t.TempDir(), t.TempDir(), t.TempDir()
	testenv.SetHome(t, home)
	t.Setenv("AVAL_LESSONS_DIR", dir)
	t.Setenv("XDG_STATE_HOME", state)
	writeFile(t, filepath.Join(dir, "commit.md"), "---\ncommands:\n  - 'git commit'\n---\nNo trailers.\n")

	var out bytes.Buffer
	in := `{"session_id":"abc","tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`
	if err := runHook([]string{"lessons"}, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No trailers.") {
		t.Errorf("output = %q, want the lesson", out.String())
	}
	if !fileExists(filepath.Join(state, "aval", "lessons", "abc.seen")) {
		t.Error("dedupe state must live under $XDG_STATE_HOME/aval/lessons")
	}
}

func TestRunHookLessonsDefaultsToHomeClaudeLessons(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	t.Setenv("AVAL_LESSONS_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	writeFile(t, filepath.Join(home, ".claude", "lessons", "x.md"), "---\ntriggers:\n  - '**/*.tsx'\n---\nHi.\n")
	var out bytes.Buffer
	in := `{"session_id":"s","tool_name":"Edit","tool_input":{"file_path":"/p/a.tsx"}}`
	if err := runHook([]string{"lessons"}, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi.") {
		t.Errorf("output = %q", out.String())
	}
	if !fileExists(filepath.Join(home, ".local", "state", "aval", "lessons", "s.seen")) {
		t.Error("default state dir should be ~/.local/state/aval/lessons")
	}
}

func TestRunHookUnknownHookIsAnError(t *testing.T) {
	if err := runHook([]string{"nope"}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Error("an unknown hook name must be reported")
	}
}

func TestRunHookDesignGateReadsEnvAndDenies(t *testing.T) {
	cache, proj := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("DESIGN_PIPELINE_OFF", "")
	t.Setenv("DESIGN_ACK_TTL", "")
	in := `{"session_id":"s","cwd":` + q(proj) + `,"tool_name":"Edit","tool_input":{"file_path":` + q(filepath.Join(proj, "Button.tsx")) + `}}`

	var out bytes.Buffer
	if err := runHook([]string{"design-gate"}, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) || !strings.Contains(out.String(), "`aval design ack <surface>`") {
		t.Errorf("want a deny naming `aval design ack <surface>`, got %q", out.String())
	}

	t.Setenv("DESIGN_PIPELINE_OFF", "1")
	out.Reset()
	if err := runHook([]string{"design-gate"}, strings.NewReader(in), &out); err != nil || out.Len() != 0 {
		t.Errorf("DESIGN_PIPELINE_OFF must silence the gate, got %q err=%v", out.String(), err)
	}
}

func TestRunHookDesignGateAllowsAfterAck(t *testing.T) {
	cache, proj := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("DESIGN_PIPELINE_OFF", "")
	t.Setenv("DESIGN_ACK_TTL", "")
	if err := runDesign([]string{"ack", "pricing"}, proj, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	in := `{"session_id":"s","cwd":` + q(proj) + `,"tool_name":"Write","tool_input":{"file_path":` + q(filepath.Join(proj, "app.css")) + `}}`
	var out bytes.Buffer
	if err := runHook([]string{"design-gate"}, strings.NewReader(in), &out); err != nil || out.Len() != 0 {
		t.Errorf("an acked project must be allowed, got %q err=%v", out.String(), err)
	}
	if !fileExists(filepath.Join(cache, "design-gate", "ui-edited.s")) {
		t.Error("an allowed UI edit must stamp the session")
	}
}

func TestRunHookDesignShotStopUsesCache(t *testing.T) {
	cache, proj := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("DESIGN_PIPELINE_OFF", "")
	writeFile(t, filepath.Join(cache, "design-gate", "ui-edited.s9"), proj)
	var out bytes.Buffer
	if err := runHook([]string{"design-shot-stop"}, strings.NewReader(`{"session_id":"s9"}`), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"decision":"block"`) {
		t.Errorf("want a block, got %q", out.String())
	}
}

func TestRunHookRtkRewriteWithoutRtkIsASilentNoOp(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	if err := runHook([]string{"rtk-rewrite"}, strings.NewReader(`{"tool_input":{"command":"git status"}}`), &out); err != nil || out.Len() != 0 {
		t.Errorf("no rtk on PATH must pass through, got %q err=%v", out.String(), err)
	}
}
