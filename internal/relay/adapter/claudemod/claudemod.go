// Package claudemod translates what the aval relay mod for Claude Code sends
// into a relay protocol request. The mod forwards the session.compact payload
// and the last session.measure usage as is; the policy stays in Go.
package claudemod

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// ErrMalformed is returned for input that is not a mod envelope.
var ErrMalformed = errors.New("claudemod: malformed envelope")

// Envelope is what the mod writes to aval relay compact's stdin.
type Envelope struct {
	Version   int       `json:"version"`
	Event     string    `json:"event"`
	Trigger   string    `json:"trigger"`
	SessionID string    `json:"session_id"`
	Model     string    `json:"model,omitempty"`
	Usage     Usage     `json:"usage"`
	Messages  []Message `json:"messages"`
}

// Usage is session.measure's context reading (S3 of spike #63: percent and
// window are the model window, never the compaction window).
type Usage struct {
	Tokens int `json:"tokens"`
	Window int `json:"window"`
}

// Message is one message of the session.compact payload.
type Message struct {
	Handle   string `json:"handle"`
	Role     string `json:"role"`
	Text     string `json:"text"`
	ToolUses []any  `json:"toolUses"`
}

// charsPerToken matches the relay estimator (E1 of spike #63).
const charsPerToken = 1.75

// Decode turns a mod envelope into a relay request. Tokens per message are
// estimated from the text; the context total is the exact session.measure
// reading. The progress fingerprint is the last message handle: a relay with
// no new message since the last one makes no progress.
func Decode(raw []byte) (relay.Request, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return relay.Request{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	req := relay.Request{
		Version: env.Version, Event: env.Event, Trigger: env.Trigger,
		SessionID: env.SessionID, Model: env.Model,
		Reading:  relay.WireReading{Tokens: env.Usage.Tokens, Window: env.Usage.Window, Source: "exact"},
		Messages: messages(env.Messages),
	}
	var prompts []string
	for _, m := range env.Messages {
		if m.Role == "user" {
			prompts = append(prompts, m.Text)
		}
	}
	req.Turn = len(prompts)
	if n := len(prompts); n > 0 {
		req.Capsule.FirstPrompt, req.Capsule.LastPrompt = prompts[0], prompts[n-1]
		if n > 2 {
			req.Capsule.Prompts = prompts[1 : n-1]
		}
	}
	if n := len(env.Messages); n > 0 {
		req.Fingerprint = env.Messages[n-1].Handle
	}
	return req, nil
}

// messages converts the payload. No recorded payload of spike #63 holds a
// tool call, so how a tool result is marked is unverified: until it is, the
// message after one with tool calls is taken as their result.
func messages(in []Message) []relay.WireMessage {
	out := make([]relay.WireMessage, len(in))
	for i, m := range in {
		out[i] = relay.WireMessage{Handle: m.Handle, Role: m.Role, Tokens: tokens(m)}
		for j, u := range m.ToolUses {
			out[i].ToolUses = append(out[i].ToolUses, toolID(u, m.Handle, j))
		}
		if i > 0 && len(out[i-1].ToolUses) > 0 {
			out[i].ToolResults = out[i-1].ToolUses
		}
	}
	return out
}

func toolID(u any, handle string, i int) string {
	if obj, ok := u.(map[string]any); ok {
		if id, ok := obj["id"].(string); ok && id != "" {
			return id
		}
	}
	return fmt.Sprintf("%s#%d", handle, i)
}

func tokens(m Message) int {
	n := len(m.Text)
	if len(m.ToolUses) > 0 {
		raw, _ := json.Marshal(m.ToolUses)
		n += len(raw)
	}
	return int(float64(n)/charsPerToken + 0.999)
}
