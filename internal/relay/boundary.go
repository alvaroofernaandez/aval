package relay

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Boundary clears (relay-autoclear T4). When the last main-thread turn ends
// on a natural boundary (a commit, a plan task done) and the context is big
// enough to be worth clearing, the mod clears the session and submits the
// text of the answer. The capsule itself reaches the new session through the
// SessionEnd and SessionStart hooks; the text only shows the last reply again,
// because /clear wipes it from the screen. Every doubt answers none: a
// boundary never clears on a guess (odd/tasks/relay-autoclear.md, T4).

// DefaultBoundaryTokens is the least context a boundary clear needs, a
// provisional value to be calibrated (odd/tasks/relay-autoclear.md, T4).
const DefaultBoundaryTokens = 100_000

// BoundaryReplyMax bounds the reply shown again, in bytes. It is the bound
// the handoff capsule applies to the same reply.
const BoundaryReplyMax = 4000

// AnswerClear is the Response.Action of a boundary that clears.
const AnswerClear = "clear"

// Reasons of a boundary answer beyond the signals.
const (
	ReasonDisabled       = "disabled"        // AVAL_RELAY is not 1
	ReasonBelowThreshold = "below_threshold" // the context is too small to be worth clearing
	ReasonNoSignal       = "no_signal"       // the turn did not end on a boundary
)

// Signals of a boundary, in the order they win.
const (
	SignalCommit   = "commit"
	SignalPlanTask = "plan-task"
)

// ToolInput is the part of a tool call's arguments the boundary reads.
type ToolInput struct {
	Command   string `json:"command,omitempty"`
	FilePath  string `json:"file_path,omitempty"`
	OldString string `json:"old_string,omitempty"`
	NewString string `json:"new_string,omitempty"`
	Content   string `json:"content,omitempty"`
}

// ToolCall is one tool call of the last turn and whether it errored.
type ToolCall struct {
	Tool    string    `json:"tool"`
	Input   ToolInput `json:"input"`
	IsError bool      `json:"isError,omitempty"`
}

// BoundaryTurn is the last main-thread turn: the tool calls made since the
// person's last prompt and the text of the last assistant reply.
type BoundaryTurn struct {
	Reply string     `json:"reply"`
	Tools []ToolCall `json:"tools"`
}

// BoundaryRequest is what the mod sends `aval relay boundary`.
type BoundaryRequest struct {
	Version int          `json:"version"`
	Tokens  int          `json:"tokens"`
	Turn    BoundaryTurn `json:"turn"`
}

// BoundaryAnswer is what `aval relay boundary` prints. Text is the prompt to
// submit after the clear; it is empty unless Action is AnswerClear.
type BoundaryAnswer struct {
	Version int    `json:"version"`
	Action  string `json:"action"` // AnswerClear or AnswerNone
	Reason  string `json:"reason,omitempty"`
	Text    string `json:"text,omitempty"`
}

var commitCmd = regexp.MustCompile(`(?:^|[\s;&|(])git(?:\s+-C\s+\S+)?\s+commit(?:\s|$)`)

// Signal returns the boundary the turn ended on ("commit", "plan-task") or "".
// A commit wins over a plan task. Calls that errored count for nothing.
func Signal(t BoundaryTurn) string {
	found := ""
	for _, c := range t.Tools {
		if c.IsError {
			continue
		}
		switch {
		case c.Tool == "Bash" && commitCmd.MatchString(c.Input.Command):
			return SignalCommit
		case planTaskDone(c):
			found = SignalPlanTask
		}
	}
	return found
}

// planTaskDone reports an Edit that turned a box of an odd/tasks plan to
// checked. A Write has no old text to compare, so any checked box counts.
func planTaskDone(c ToolCall) bool {
	if !isPlanFile(c.Input.FilePath) {
		return false
	}
	switch c.Tool {
	case "Edit", "MultiEdit":
		return countChecked(c.Input.NewString) > countChecked(c.Input.OldString)
	case "Write":
		return countChecked(c.Input.Content) > 0
	}
	return false
}

func countChecked(s string) int { return strings.Count(strings.ToLower(s), "- [x]") }

// isPlanFile reports a markdown file directly under an odd/tasks directory.
func isPlanFile(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasSuffix(path, ".md") {
		return false
	}
	i := strings.LastIndex(path, "odd/tasks/")
	if i < 0 || (i > 0 && path[i-1] != '/') {
		return false
	}
	return !strings.Contains(path[i+len("odd/tasks/"):], "/")
}

// DecideBoundary answers a boundary request. enabled is AVAL_RELAY=1.
func (p Policy) DecideBoundary(enabled bool, r BoundaryRequest) BoundaryAnswer {
	none := func(reason string) BoundaryAnswer {
		return BoundaryAnswer{Version: ProtocolVersion, Action: AnswerNone, Reason: reason}
	}
	switch {
	case !enabled:
		return none(ReasonDisabled)
	case r.Version != ProtocolVersion:
		return none(ReasonVersion)
	case r.Tokens < p.BoundaryTokens || r.Tokens <= 0:
		return none(ReasonBelowThreshold)
	}
	signal := Signal(r.Turn)
	if signal == "" {
		return none(ReasonNoSignal)
	}
	return BoundaryAnswer{
		Version: ProtocolVersion, Action: AnswerClear, Reason: signal,
		Text: boundaryText(signal, r.Tokens, r.Turn.Reply),
	}
}

func boundaryText(signal string, tokens int, reply string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "aval relay: context relayed after %s (%dk tokens).", signal, tokens/1000)
	if reply = strings.TrimSpace(reply); reply != "" {
		b.WriteString(" Your previous reply, shown again for the user:\n\n")
		b.WriteString(Redact(TruncateBytes(reply, BoundaryReplyMax)))
	}
	b.WriteString("\n\nReply only: relayed.")
	return b.String()
}

// TruncateBytes cuts s to at most n bytes, on a rune boundary, and marks the cut.
func TruncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + " [...]"
}
