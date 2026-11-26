package claudehook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// resendWindow is how long a blocked prompt may be resent to get through.
const resendWindow = 2 * time.Minute

// BlockLog remembers the prompt a hook refused, so the person can resend it.
type BlockLog interface {
	NoteBlock(session, key string, at time.Time) error
	TakeBlock(session, key string, within time.Duration, now time.Time) bool
}

// PromptOptions are what the UserPromptSubmit hook needs from its caller.
type PromptOptions struct {
	Policy   relay.Policy
	Now      time.Time
	CacheTTL time.Duration // prompt cache lifetime the hook assumes
	Paused   func(dir string) bool
	Blocks   BlockLog
	Block    func(tokens int, idle time.Duration) string // reason of a block
	Warn     func(tokens int, idle time.Duration) string // text of a warning
}

type promptInput struct {
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
	Prompt         string `json:"prompt"`
}

// Prompt is the UserPromptSubmit hook, the cold-cache advisor: the policy
// decides on the last main turn's tokens and the time since it. A cold cache
// with a big context blocks the prompt (the same prompt sent again within two
// minutes goes through); a warm one past the soft threshold only warns. On
// anything it cannot use it prints nothing: the prompt goes through.
func Prompt(in io.Reader, out io.Writer, opt PromptOptions) {
	var h promptInput
	if json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&h) != nil || h.TranscriptPath == "" {
		return
	}
	if opt.Paused != nil && opt.Paused(h.CWD) {
		return
	}
	last, ok := lastMainTurn(h.TranscriptPath)
	if !ok || last.At.IsZero() {
		return
	}
	idle := opt.Now.Sub(last.At)
	d := opt.Policy.Decide(relay.Snapshot{
		Reading:  relay.Reading{Tokens: last.Tokens},
		CacheAge: idle, CacheTTL: opt.CacheTTL,
	})
	if d.Action == relay.ActionNone {
		return
	}
	cold := opt.CacheTTL > 0 && idle > opt.CacheTTL
	if cold && last.Tokens >= opt.Policy.ColdMinTokens {
		sum := sha256.Sum256([]byte(h.Prompt))
		key := hex.EncodeToString(sum[:])
		if opt.Blocks != nil && h.SessionID != "" {
			if opt.Blocks.TakeBlock(h.SessionID, key, resendWindow, opt.Now) {
				return
			}
			_ = opt.Blocks.NoteBlock(h.SessionID, key, opt.Now)
		}
		write(out, map[string]string{"decision": "block", "reason": opt.Block(last.Tokens, idle)})
		return
	}
	if last.Tokens >= opt.Policy.SoftTokens {
		write(out, map[string]any{"hookSpecificOutput": map[string]string{
			"hookEventName": "UserPromptSubmit", "additionalContext": opt.Warn(last.Tokens, idle),
		}})
	}
}

func lastMainTurn(path string) (relay.Turn, bool) {
	f, err := os.Open(path)
	if err != nil {
		return relay.Turn{}, false
	}
	defer f.Close()
	return relay.LastTurn(relay.ReadUsage(f), false)
}

func write(out io.Writer, v any) {
	if b, err := json.Marshal(v); err == nil {
		_, _ = out.Write(append(b, '\n'))
	}
}
