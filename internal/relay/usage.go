package relay

import (
	"bufio"
	"encoding/json"
	"io"
	"time"
)

// Turn is one assistant turn of a transcript: the exact context it ran with
// and when it ended.
type Turn struct {
	Tokens    int       // input + cache creation + cache read
	At        time.Time // zero when the entry has no readable timestamp
	Sidechain bool      // the turn belongs to a sub-agent
}

// maxUsageLine bounds one transcript line; a longer one is skipped.
const maxUsageLine = 16 << 20

type usageEntry struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Timestamp   string `json:"timestamp"`
	Message     struct {
		Usage *struct {
			Input         int `json:"input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ReadUsage returns the assistant turns of a transcript in order. A line it
// cannot read, or an assistant entry with no usage, is skipped.
func ReadUsage(r io.Reader) []Turn {
	var turns []Turn
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxUsageLine)
	for sc.Scan() {
		var e usageEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Type != "assistant" || e.Message.Usage == nil {
			continue
		}
		u := e.Message.Usage
		at, _ := time.Parse(time.RFC3339, e.Timestamp)
		turns = append(turns, Turn{Tokens: u.Input + u.CacheCreation + u.CacheRead, At: at, Sidechain: e.IsSidechain})
	}
	return turns
}

// FirstTurn is the first turn of the main thread (sidechain false) or of a
// sub-agent (true).
func FirstTurn(turns []Turn, sidechain bool) (Turn, bool) {
	for _, t := range turns {
		if t.Sidechain == sidechain {
			return t, true
		}
	}
	return Turn{}, false
}

// LastTurn is the last turn of the main thread or of a sub-agent.
func LastTurn(turns []Turn, sidechain bool) (Turn, bool) {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Sidechain == sidechain {
			return turns[i], true
		}
	}
	return Turn{}, false
}
