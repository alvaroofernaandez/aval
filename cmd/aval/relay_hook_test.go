package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

const relayTranscript = `{"type":"user","message":{"role":"user","content":"Add retries to the upload client."}}
{"type":"user","message":{"role":"user","content":"Cap the backoff."}}
`

// relayHookRun runs one relay hook with a transcript in a scratch dir, a
// scratch state dir and the given AVAL_RELAY.
func relayHookRun(t *testing.T, name, state, relayOn, source string) string {
	t.Helper()
	dir := t.TempDir()
	tr := filepath.Join(dir, "t.jsonl")
	writeFile(t, tr, relayTranscript)
	in, _ := json.Marshal(map[string]string{"session_id": "s", "cwd": dir, "transcript_path": tr, "source": source})
	env := map[string]string{"XDG_STATE_HOME": state, "AVAL_RELAY": relayOn}
	var out bytes.Buffer
	if err := runHook([]string{name}, cmdIO{stdin: bytes.NewReader(in), stdout: &out, stderr: &bytes.Buffer{}, env: func(k string) string { return env[k] }}); err != nil {
		t.Fatalf("hook %s: %v", name, err)
	}
	return out.String()
}

func TestRelayHook_PreCompactSteersWhenOn(t *testing.T) {
	got := relayHookRun(t, "relay-precompact", t.TempDir(), "1", "")
	if !strings.Contains(got, "Add retries to the upload client.") {
		t.Fatalf("got %q, want the steer text with the first request", got)
	}
}

func TestRelayHook_SessionStartAddsTheCapsuleWhenOn(t *testing.T) {
	got := relayHookRun(t, "relay-sessionstart", t.TempDir(), "1", "compact")
	var o struct {
		H struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(got), &o); err != nil || !strings.Contains(o.H.Context, "Cap the backoff.") {
		t.Fatalf("got %q (%v), want additionalContext with the capsule", got, err)
	}
}

func TestRelayHook_PrintsNothing(t *testing.T) {
	recent := t.TempDir()
	store := statefs.Store{Dir: filepath.Join(recent, "aval", "relay"), Alive: func(int, int64) bool { return false }}
	if err := store.Prepare("relay-abc", statefs.Entry{Trigger: "auto", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, hook, state, on, source string }{
		{"pre compact, relay off", "relay-precompact", t.TempDir(), "", ""},
		{"pre compact, AVAL_RELAY=0", "relay-precompact", t.TempDir(), "0", ""},
		{"session start, relay off", "relay-sessionstart", t.TempDir(), "", "compact"},
		{"session start after a mod relay", "relay-sessionstart", recent, "1", "compact"},
	} {
		if got := relayHookRun(t, c.hook, c.state, c.on, c.source); got != "" {
			t.Errorf("%s: printed %q, want nothing", c.name, got)
		}
	}
}

func TestRelayHook_PausedRepoPrintsNothing(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	store, _ := relayStore(func(k string) string { return map[string]string{"XDG_STATE_HOME": state}[k] })
	if err := store.SetPaused(dir, true); err != nil {
		t.Fatal(err)
	}
	tr := filepath.Join(dir, "t.jsonl")
	if err := os.WriteFile(tr, []byte(relayTranscript), 0o600); err != nil {
		t.Fatal(err)
	}
	in, _ := json.Marshal(map[string]string{"cwd": dir, "transcript_path": tr})
	env := map[string]string{"XDG_STATE_HOME": state, "AVAL_RELAY": "1"}
	var out bytes.Buffer
	_ = runHook([]string{"relay-precompact"}, cmdIO{stdin: bytes.NewReader(in), stdout: &out, stderr: &bytes.Buffer{}, env: func(k string) string { return env[k] }})
	if out.Len() != 0 {
		t.Fatalf("paused: printed %q, want nothing", out.String())
	}
}

// promptHookRun runs relay-prompt on a transcript whose last turn is from
// 2020 (a cold cache) with tokens in context.
func promptHookRun(t *testing.T, state, relayOn string, tokens int, prompt string) string {
	t.Helper()
	dir := t.TempDir()
	tr := filepath.Join(dir, "t.jsonl")
	writeFile(t, tr, `{"type":"assistant","timestamp":"2020-01-01T10:00:00Z","message":{"usage":{"input_tokens":`+strconv.Itoa(tokens)+`}}}`+"\n")
	in, _ := json.Marshal(map[string]string{"session_id": "s", "cwd": dir, "transcript_path": tr, "prompt": prompt})
	env := map[string]string{"XDG_STATE_HOME": state, "AVAL_RELAY": relayOn, "AVAL_LANG": "en"}
	var out bytes.Buffer
	if err := runHook([]string{"relay-prompt"}, cmdIO{stdin: bytes.NewReader(in), stdout: &out, stderr: &bytes.Buffer{}, env: func(k string) string { return env[k] }}); err != nil {
		t.Fatalf("hook: %v", err)
	}
	return out.String()
}

func TestRelayPromptHook_BlocksAColdBigContextThenLetsAResendThrough(t *testing.T) {
	state := t.TempDir()
	got := promptHookRun(t, state, "1", 150_000, "next")
	var o struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(got), &o); err != nil || o.Decision != "block" || !strings.Contains(o.Reason, "/clear") {
		t.Fatalf("got %q (%v), want a block that names /clear", got, err)
	}
	if again := promptHookRun(t, state, "1", 150_000, "next"); again != "" {
		t.Errorf("resend printed %q, want nothing", again)
	}
}

func TestRelayPromptHook_Silent(t *testing.T) {
	if got := promptHookRun(t, t.TempDir(), "", 150_000, "p"); got != "" {
		t.Errorf("relay off printed %q", got)
	}
	if got := promptHookRun(t, t.TempDir(), "1", 50_000, "p"); got != "" {
		t.Errorf("small context printed %q", got)
	}
}
