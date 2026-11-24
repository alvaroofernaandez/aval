// Package claudemod translates what the aval relay mod for Claude Code sends
// into a relay protocol request. The mod forwards the session.compact payload
// and the last session.measure usage as is; the policy stays in Go.
package claudemod

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// Runtime names Claude Code in machine output.
const Runtime = "claude-code"

// Source is where the mod's token reading comes from: session.measure
// reports the context tokens exactly (E1 of the #63 spike).
const Source = "exact"

// Capabilities is what the mods API lets the relay do on Claude Code:
// replace the context (S1), steer the native summary (P1) and clear then
// resubmit at a turn boundary (S6).
var Capabilities = relay.Capabilities{Compaction: relay.CompactionReplace, Inject: true, Session: relay.SessionRestart}

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
	// Prompt is the latest prompt the person submitted, as the mod saw it on
	// prompt.submit. An auto compaction runs before that prompt joins the
	// messages, so without it the capsule loses the request in flight.
	Prompt string `json:"prompt,omitempty"`
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
		Reading:  relay.WireReading{Tokens: env.Usage.Tokens, Window: env.Usage.Window, Source: Source},
		Messages: messages(env.Messages),
	}
	first, prompts := userPrompts(env.Messages)
	prompts = withInFlight(prompts, env.Prompt)
	req.Turn = len(prompts)
	if first == "" && len(prompts) > 0 {
		first, prompts = prompts[0], prompts[1:]
	}
	req.Capsule.FirstPrompt = first
	if n := len(prompts); n > 0 {
		req.Capsule.LastPrompt = prompts[n-1]
		req.Capsule.Prompts = prompts[:n-1]
	}
	if n := len(env.Messages); n > 0 {
		req.Fingerprint = env.Messages[n-1].Handle
	}
	return req, nil
}

// userPrompts returns the user requests in order. A capsule from an earlier
// relay is not a request: it only gives back the original first request,
// returned as first (empty when there is no capsule).
func userPrompts(msgs []Message) (first string, prompts []string) {
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		if f, ok := relay.CapsuleFirst(m.Text); ok {
			if first == "" {
				first = f
			}
			continue
		}
		if relay.IsRequest(m.Text) {
			prompts = append(prompts, m.Text)
		}
	}
	return first, prompts
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

// withInFlight appends the prompt in flight when the messages do not hold it
// yet. A slash command or a capsule the mod submitted is not a request.
func withInFlight(prompts []string, prompt string) []string {
	if !relay.IsRequest(prompt) || strings.HasPrefix(prompt, "/") {
		return prompts
	}
	if n := len(prompts); n > 0 && prompts[n-1] == prompt {
		return prompts
	}
	return append(prompts, prompt)
}
