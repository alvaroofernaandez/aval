package relay

import (
	"encoding/json"
	"time"
)

// ProtocolVersion is the version of the adapter protocol this core speaks.
const ProtocolVersion = 1

// Request is what a runtime shim sends, as JSON.
type Request struct {
	Version     int           `json:"version"`
	Event       string        `json:"event"`             // "compact" or "measure"
	Trigger     string        `json:"trigger,omitempty"` // "auto", "manual"
	SessionID   string        `json:"session_id"`
	Turn        int           `json:"turn"`
	Fingerprint string        `json:"fingerprint"`
	Model       string        `json:"model,omitempty"`
	Reading     WireReading   `json:"reading"`
	Cache       WireCache     `json:"cache"`
	Messages    []WireMessage `json:"messages"`
	Capsule     WireCapsule   `json:"capsule"`
	Chain       []Link        `json:"chain,omitempty"`
}

// WireReading is a Reading on the wire.
type WireReading struct {
	Tokens int    `json:"tokens"`
	Window int    `json:"window"`
	Floor  int    `json:"floor"`
	Source string `json:"source"` // "exact", "provider", "transcript", "heuristic"
}

// WireCache is the prompt cache state; zero means unknown.
type WireCache struct {
	AgeMS int64 `json:"age_ms"`
	TTLMS int64 `json:"ttl_ms"`
}

// WireMessage is a Message on the wire.
type WireMessage struct {
	Handle      string   `json:"handle"`
	Role        string   `json:"role"`
	Tokens      int      `json:"tokens"`
	ToolUses    []string `json:"tool_uses,omitempty"`
	ToolResults []string `json:"tool_results,omitempty"`
}

// WireCapsule carries the facts the capsule is built from.
type WireCapsule struct {
	FirstPrompt string     `json:"first_prompt"`
	LastPrompt  string     `json:"last_prompt"`
	Prompts     []string   `json:"prompts,omitempty"`
	Files       []string   `json:"files,omitempty"`
	Repo        *RepoFacts `json:"repo,omitempty"`
	Work        []string   `json:"work,omitempty"`
	Note        string     `json:"note,omitempty"`
}

// Response is what the core answers, as JSON.
type Response struct {
	Version int      `json:"version"`
	Action  string   `json:"action"` // see the Answer constants
	Capsule string   `json:"capsule,omitempty"`
	Keep    []string `json:"keep,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	// Instructions steer the runtime's native summary (AnswerSteer).
	Instructions string `json:"instructions,omitempty"`
}

// Answers in Response.Action.
const (
	AnswerPassthrough = "passthrough" // let the runtime do what it would do
	AnswerReplace     = "replace"     // replace the context with capsule plus keep
	AnswerSteer       = "steer"       // compact natively, steered by Instructions
	AnswerNone        = "none"        // nothing to do yet
	AnswerPrecompute  = "precompute"  // prepare the capsule
	AnswerRelay       = "relay"       // start a relay now
)

// Reasons in Response.Reason.
const (
	ReasonMalformed = "malformed"
	ReasonVersion   = "version"
	ReasonEvent     = "event"
	ReasonNoGain    = "no_gain" // the tail keeps every message
)

// Handle answers one protocol request. Any input it cannot use yields a
// passthrough, so a shim never breaks its runtime.
func Handle(in []byte, p Policy, b Brakes) []byte {
	var req Request
	if err := json.Unmarshal(in, &req); err != nil {
		return answer(Response{Action: AnswerPassthrough, Reason: ReasonMalformed})
	}
	if req.Version != ProtocolVersion {
		return answer(Response{Action: AnswerPassthrough, Reason: ReasonVersion})
	}
	brake := b.Check(Chain{Links: req.Chain}, Attempt{Fingerprint: req.Fingerprint, Turn: req.Turn})
	switch req.Event {
	case "compact":
		if brake != BrakeNone {
			return answer(Response{Action: AnswerPassthrough, Reason: "brake:" + brake.String()})
		}
		keep := SelectTail(req.messages(), p.TailTokens)
		if len(keep) == len(req.Messages) {
			return answer(Response{Action: AnswerSteer, Reason: ReasonNoGain, Instructions: steerText(req.Capsule.capsule())})
		}
		return answer(Response{
			Action:  AnswerReplace,
			Capsule: Render(req.Capsule.capsule(), p.CapsuleTokens),
			Keep:    keep,
		})
	case "measure":
		return answer(measure(p.Decide(req.snapshot()), brake))
	}
	return answer(Response{Action: AnswerPassthrough, Reason: ReasonEvent})
}

func measure(d Decision, brake BrakeReason) Response {
	switch {
	case d.Action == ActionRelay && brake != BrakeNone:
		return Response{Action: AnswerNone, Reason: "brake:" + brake.String()}
	case d.Action == ActionRelay:
		return Response{Action: AnswerRelay}
	case d.Action == ActionPrecompute:
		return Response{Action: AnswerPrecompute}
	}
	return Response{Action: AnswerNone}
}

func answer(r Response) []byte {
	r.Version = ProtocolVersion
	out, err := json.Marshal(r)
	if err != nil {
		return []byte(`{"version":1,"action":"passthrough"}`)
	}
	return out
}

var sources = map[string]Source{
	"exact": SourceExact, "provider": SourceProvider,
	"transcript": SourceTranscript, "heuristic": SourceHeuristic,
}

var errPcts = map[Source]float64{
	SourceExact: exactErrPct, SourceProvider: providerErrPct,
	SourceTranscript: transcriptErrPct, SourceHeuristic: unknownErrPct,
}

func (r Request) snapshot() Snapshot {
	src, ok := sources[r.Reading.Source]
	if !ok {
		src = SourceHeuristic
	}
	return Snapshot{
		Reading: Reading{
			Tokens: r.Reading.Tokens, Window: r.Reading.Window, FloorTokens: r.Reading.Floor,
			Source: src, ErrPct: errPcts[src],
		},
		CacheAge: time.Duration(r.Cache.AgeMS) * time.Millisecond,
		CacheTTL: time.Duration(r.Cache.TTLMS) * time.Millisecond,
	}
}

func (r Request) messages() []Message {
	out := make([]Message, len(r.Messages))
	for i, m := range r.Messages {
		role := RoleUser
		if m.Role == "assistant" {
			role = RoleAssistant
		}
		out[i] = Message{Handle: m.Handle, Role: role, Tokens: m.Tokens, ToolUses: m.ToolUses, ToolResults: m.ToolResults}
	}
	return out
}

func (c WireCapsule) capsule() Capsule {
	return Capsule{
		FirstPrompt: c.FirstPrompt, LastPrompt: c.LastPrompt, Prompts: c.Prompts,
		Files: c.Files, Repo: c.Repo, Work: c.Work, Note: c.Note,
	}
}

// steerMaxChars caps steer instructions at Claude Code's documented limit for
// summariser instructions from a PreCompact hook (10,000 characters).
const steerMaxChars = 10_000

// steerTokens is the capsule budget inside steer instructions, about 8,000
// characters, so the header fits under steerMaxChars.
const steerTokens = 2_000

const steerHeader = "Keep these facts in the summary, so work can continue without the earlier messages:\n\n"

// steerText is the capsule as summariser instructions. The capsule budget
// keeps it under steerMaxChars (TestSteerText_StaysUnderTheCap).
func steerText(c Capsule) string {
	return steerHeader + Render(c, steerTokens)
}
