package claudehook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

var lastTurnAt = time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)

// fakeBlocks is an in-memory BlockLog.
type fakeBlocks struct {
	key string
	at  time.Time
}

func (f *fakeBlocks) NoteBlock(_, key string, at time.Time) error { f.key, f.at = key, at; return nil }
func (f *fakeBlocks) TakeBlock(_, key string, within time.Duration, now time.Time) bool {
	ok := f.key == key && now.Sub(f.at) <= within
	if ok {
		f.key = ""
	}
	return ok
}

// promptRun runs Prompt against a transcript whose last main turn holds
// tokens and ended at lastTurnAt, with the clock idle after it.
func promptRun(t *testing.T, tokens int, idle time.Duration, mutate func(*PromptOptions)) string {
	t.Helper()
	dir := t.TempDir()
	tr := filepath.Join(dir, "t.jsonl")
	line := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"usage":{"input_tokens":%d}}}`+"\n", lastTurnAt.Format(time.RFC3339), tokens)
	if err := os.WriteFile(tr, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	return promptRunWith(t, tr, dir, "do the thing", lastTurnAt.Add(idle), mutate)
}

func promptRunWith(t *testing.T, tr, cwd, prompt string, now time.Time, mutate func(*PromptOptions)) string {
	t.Helper()
	opt := PromptOptions{
		Policy: relay.DefaultPolicy(), Now: now, CacheTTL: time.Hour, Blocks: &fakeBlocks{},
		Block: func(tokens int, idle time.Duration) string { return fmt.Sprintf("BLOCK %d %s", tokens, idle) },
		Warn:  func(tokens int, idle time.Duration) string { return fmt.Sprintf("WARN %d", tokens) },
	}
	if mutate != nil {
		mutate(&opt)
	}
	in, _ := json.Marshal(map[string]string{"session_id": "s", "cwd": cwd, "transcript_path": tr, "prompt": prompt})
	var out bytes.Buffer
	Prompt(bytes.NewReader(in), &out, opt)
	return out.String()
}

func TestPrompt_ColdAndBigBlocks(t *testing.T) {
	got := promptRun(t, 120_000, 2*time.Hour, nil)
	var o struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(got), &o); err != nil || o.Decision != "block" || !strings.Contains(o.Reason, "BLOCK 120000 2h0m0s") {
		t.Fatalf("got %q (%v), want a block with the reason", got, err)
	}
}

func TestPrompt_WarmAndBigWarnsWithoutBlocking(t *testing.T) {
	got := promptRun(t, 170_000, time.Minute, nil)
	var o struct {
		Decision string `json:"decision"`
		H        struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(got), &o); err != nil || o.Decision != "" || o.H.Event != "UserPromptSubmit" || o.H.Context != "WARN 170000" {
		t.Fatalf("got %q (%v), want a non-blocking additionalContext warning", got, err)
	}
}

func TestPrompt_Silent(t *testing.T) {
	for _, c := range []struct {
		name   string
		tokens int
		idle   time.Duration
	}{
		{"cold but small", 50_000, 2 * time.Hour},
		{"warm and small", 120_000, time.Minute},
		{"idle at exactly the ttl", 120_000, time.Hour},
	} {
		if got := promptRun(t, c.tokens, c.idle, nil); got != "" {
			t.Errorf("%s: printed %q, want nothing", c.name, got)
		}
	}
}

func TestPrompt_FailsOpen(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "t.jsonl")
	_ = os.WriteFile(good, []byte(`{"type":"assistant","timestamp":"2026-11-26T10:00:00Z","message":{"usage":{"input_tokens":500000}}}`+"\n"), 0o600)
	now := lastTurnAt.Add(5 * time.Hour)
	for name, got := range map[string]string{
		"missing transcript": promptRunWith(t, filepath.Join(dir, "none.jsonl"), dir, "p", now, nil),
		"paused":             promptRunWith(t, good, dir, "p", now, func(o *PromptOptions) { o.Paused = func(string) bool { return true } }),
	} {
		if got != "" {
			t.Errorf("%s: printed %q, want nothing", name, got)
		}
	}
	var out bytes.Buffer
	Prompt(strings.NewReader("not json"), &out, PromptOptions{Policy: relay.DefaultPolicy()})
	if out.Len() != 0 {
		t.Errorf("bad input printed %q", out.String())
	}
}

func TestPrompt_ResendingTheBlockedPromptGetsThrough(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "t.jsonl")
	_ = os.WriteFile(tr, []byte(`{"type":"assistant","timestamp":"2026-11-26T10:00:00Z","message":{"usage":{"input_tokens":150000}}}`+"\n"), 0o600)
	blocks := &fakeBlocks{}
	use := func(o *PromptOptions) { o.Blocks = blocks }
	now := lastTurnAt.Add(2 * time.Hour)
	if got := promptRunWith(t, tr, dir, "same", now, use); !strings.Contains(got, `"block"`) {
		t.Fatalf("first send: %q, want a block", got)
	}
	if got := promptRunWith(t, tr, dir, "different", now.Add(10*time.Second), use); !strings.Contains(got, `"block"`) {
		t.Errorf("a different prompt: %q, want a block", got)
	}
	_ = promptRunWith(t, tr, dir, "same", now.Add(20*time.Second), use) // blocked again, replaces the record
	if got := promptRunWith(t, tr, dir, "same", now.Add(40*time.Second), use); got != "" {
		t.Errorf("resend within 2 minutes: %q, want nothing", got)
	}
	if got := promptRunWith(t, tr, dir, "same", now.Add(50*time.Second), use); !strings.Contains(got, `"block"`) {
		t.Errorf("third send: %q, want a block again", got)
	}
}
