package claudehook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHandoffs is an in-memory Handoffs.
type fakeHandoffs struct {
	cwd, text string
	at        time.Time
}

func (f *fakeHandoffs) PutHandoff(cwd, text string, at time.Time) error {
	f.cwd, f.text, f.at = cwd, text, at
	return nil
}

func (f *fakeHandoffs) TakeHandoff(cwd string, now time.Time, maxAge time.Duration) (string, bool) {
	if f.cwd != cwd || f.text == "" || now.Sub(f.at) > maxAge {
		return "", false
	}
	text := f.text
	f.text = ""
	return text, true
}

const clearTranscript = `{"type":"user","message":{"role":"user","content":"Add retries to the upload client."}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done: retries are in client.go."}]}}
{"type":"user","message":{"role":"user","content":"Cap the backoff."}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/w/backoff.go"}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Capped at 30 s."}]}}
`

func hookIn(t *testing.T, fields map[string]string) *bytes.Reader {
	t.Helper()
	b, _ := json.Marshal(fields)
	return bytes.NewReader(b)
}

func writeTranscript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var clearAt = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

func TestSessionEnd_ClearStoresTheCapsuleWithTheLastExchange(t *testing.T) {
	h := &fakeHandoffs{}
	tr := writeTranscript(t, clearTranscript)
	SessionEnd(hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr, "reason": "clear"}), Options{Handoff: h, Now: clearAt, CapsuleTokens: 8000})
	if h.cwd != "/w" || !h.at.Equal(clearAt) {
		t.Fatalf("stored for %q at %v", h.cwd, h.at)
	}
	for _, want := range []string{"Add retries to the upload client.", "Cap the backoff.", "Capped at 30 s.", "/w/backoff.go"} {
		if !strings.Contains(h.text, want) {
			t.Errorf("handoff lacks %q:\n%s", want, h.text)
		}
	}
}

func TestSessionEnd_OtherReasonsAndBadInputStoreNothing(t *testing.T) {
	tr := writeTranscript(t, clearTranscript)
	for name, in := range map[string]*bytes.Reader{
		"logout":        hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr, "reason": "logout"}),
		"no reason":     hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr}),
		"no transcript": hookIn(t, map[string]string{"cwd": "/w", "transcript_path": "/nope", "reason": "clear"}),
		"garbage":       bytes.NewReader([]byte("x")),
	} {
		h := &fakeHandoffs{}
		SessionEnd(in, Options{Handoff: h, Now: clearAt})
		if h.text != "" {
			t.Errorf("%s: stored %q", name, h.text)
		}
	}
	h := &fakeHandoffs{}
	SessionEnd(hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr, "reason": "clear"}), Options{Handoff: h, Now: clearAt, Paused: func(string) bool { return true }})
	if h.text != "" {
		t.Error("stored for a paused repo")
	}
}

func TestSessionStart_ClearInjectsTheHandoffOnce(t *testing.T) {
	h := &fakeHandoffs{cwd: "/w", text: "CAPSULE", at: clearAt}
	run := func(now time.Time) string {
		var out bytes.Buffer
		SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "clear"}), &out, Options{Handoff: h, Now: now})
		return out.String()
	}
	var o struct {
		H struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	got := run(clearAt.Add(time.Minute))
	if err := json.Unmarshal([]byte(got), &o); err != nil || o.H.Event != "SessionStart" || o.H.Context != "CAPSULE" {
		t.Fatalf("got %q (%v)", got, err)
	}
	if again := run(clearAt.Add(2 * time.Minute)); again != "" {
		t.Errorf("second start printed %q", again)
	}
}

func TestSessionStart_ClearIgnoresStaleAndForeignHandoffs(t *testing.T) {
	var out bytes.Buffer
	h := &fakeHandoffs{cwd: "/w", text: "CAPSULE", at: clearAt}
	SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "clear"}), &out, Options{Handoff: h, Now: clearAt.Add(11 * time.Minute)})
	SessionStart(hookIn(t, map[string]string{"cwd": "/other", "source": "clear"}), &out, Options{Handoff: &fakeHandoffs{cwd: "/w", text: "C", at: clearAt}, Now: clearAt})
	SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "startup"}), &out, Options{Handoff: &fakeHandoffs{cwd: "/w", text: "C", at: clearAt}, Now: clearAt})
	if out.Len() != 0 {
		t.Errorf("printed %q, want nothing", out.String())
	}
}
