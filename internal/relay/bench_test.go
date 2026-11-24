package relay

import (
	"encoding/json"
	"fmt"
	"testing"
)

// BenchmarkHandle2000Messages measures policy, tail and capsule for a
// 2,000-message compaction. Target: under 20 ms (#64); no assertion.
func BenchmarkHandle2000Messages(b *testing.B) {
	req := Request{
		Version: ProtocolVersion, Event: "compact", Trigger: "auto", Turn: 900, Fingerprint: "f",
		Reading: WireReading{Tokens: 201_000, Window: 1_000_000, Floor: 58_000, Source: "exact"},
		Capsule: WireCapsule{FirstPrompt: "start", LastPrompt: "continue"},
	}
	for i := range 2000 {
		m := WireMessage{Handle: fmt.Sprintf("m%d", i), Role: "user", Tokens: 100}
		if i%2 == 1 {
			m.Role, m.ToolUses = "assistant", []string{fmt.Sprintf("t%d", i)}
		} else if i > 0 {
			m.ToolResults = []string{fmt.Sprintf("t%d", i-1)}
		}
		req.Messages = append(req.Messages, m)
		req.Capsule.Prompts = append(req.Capsule.Prompts, fmt.Sprintf("prompt %d sk-ant-api03-abcdefghijklmnopqrstuvwxyz", i))
	}
	in, err := json.Marshal(req)
	if err != nil {
		b.Fatal(err)
	}
	p, br := DefaultPolicy(), DefaultBrakes()
	b.ResetTimer()
	for range b.N {
		Handle(in, p, br)
	}
}
