package claudemod

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "compact-auto.json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDecode_CompactFixture(t *testing.T) {
	req, err := Decode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if req.Version != relay.ProtocolVersion || req.Event != "compact" || req.Trigger != "auto" || req.SessionID != "session-a" {
		t.Fatalf("header: %+v", req)
	}
	if req.Reading.Tokens != 63_795 || req.Reading.Window != 1_000_000 || req.Reading.Source != "exact" {
		t.Fatalf("reading: %+v", req.Reading)
	}
	if len(req.Messages) != 6 || req.Messages[0].Handle != "h-01" || req.Messages[0].Tokens == 0 {
		t.Fatalf("messages: %+v", req.Messages)
	}
	if req.Capsule.FirstPrompt != "Turn 1. Write about a lighthouse." || req.Capsule.LastPrompt != "Turn 3. Write about a glacier." {
		t.Fatalf("prompts: %q / %q", req.Capsule.FirstPrompt, req.Capsule.LastPrompt)
	}
	if !slices.Equal(req.Capsule.Prompts, []string{"Turn 2. Write about a river."}) {
		t.Fatalf("earlier prompts: %v", req.Capsule.Prompts)
	}
	if req.Turn != 3 || req.Fingerprint != "h-06" {
		t.Fatalf("turn %d fingerprint %q, want 3 and the last handle h-06", req.Turn, req.Fingerprint)
	}
}

// No recorded payload of spike #63 holds a tool call, so how a tool result
// is marked is unverified. Until it is, a message with tool calls is paired
// with the message after it, so the tail never keeps a result without its call.
func TestDecode_ToolCallPairsWithTheNextMessage(t *testing.T) {
	req, err := Decode([]byte(`{"version":1,"event":"compact","messages":[
		{"handle":"a","role":"assistant","text":"x","toolUses":[{"id":"t1"}]},
		{"handle":"b","role":"user","text":"result"},
		{"handle":"c","role":"assistant","text":"done","toolUses":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	a, b := req.Messages[0], req.Messages[1]
	if len(a.ToolUses) != 1 || !slices.Equal(a.ToolUses, b.ToolResults) {
		t.Fatalf("pair not linked: uses %v, results %v", a.ToolUses, b.ToolResults)
	}
	if len(req.Messages[2].ToolUses) != 0 {
		t.Fatalf("an empty toolUses made a call: %+v", req.Messages[2])
	}
}

func TestDecode_Malformed(t *testing.T) {
	for _, in := range []string{``, `{`, `[]`, `"x"`} {
		if _, err := Decode([]byte(in)); !errors.Is(err, ErrMalformed) {
			t.Errorf("Decode(%q): got %v, want ErrMalformed", in, err)
		}
	}
}

// After a relay the first user message is aval's own capsule. A second relay
// must not take it for the first request (found in the T10 sandbox run: the
// capsules nested).
func TestDecode_APreviousCapsuleIsNotAPrompt(t *testing.T) {
	capsule := "# aval relay capsule\n\n## Objective\nFirst request: Turn 1. Find OTTER1.\nLatest request: Turn 3.\n"
	in, err := json.Marshal(Envelope{Version: 1, Event: "compact", Messages: []Message{
		{Handle: "c", Role: "user", Text: capsule},
		{Handle: "u3", Role: "user", Text: "Turn 3."},
		{Handle: "a3", Role: "assistant", Text: "done"},
		{Handle: "u4", Role: "user", Text: "Turn 4."},
	}})
	if err != nil {
		t.Fatal(err)
	}
	req, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	if req.Capsule.FirstPrompt != "Turn 1. Find OTTER1." || req.Capsule.LastPrompt != "Turn 4." {
		t.Fatalf("first %q last %q, want the original first request and Turn 4.", req.Capsule.FirstPrompt, req.Capsule.LastPrompt)
	}
	if !slices.Equal(req.Capsule.Prompts, []string{"Turn 3."}) || req.Turn != 2 {
		t.Fatalf("prompts %v turn %d: the capsule counted as a request", req.Capsule.Prompts, req.Turn)
	}
}
