package usage

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// ccusage 20.1.2 `blocks --json` on the same corpus gives these three blocks
// (09:00 with 2 entries, 15:00 with 1, and 10:00 on the 21st with 3).
func TestBlocksFollowCcusageSemantics(t *testing.T) {
	bs := Blocks(fixtureEntries(t))
	type want struct {
		start, end string
		messages   int
		tokens     int64
	}
	ws := []want{
		{"2026-11-20T09:00:00Z", "2026-11-20T14:00:00Z", 2, 28500},
		{"2026-11-20T15:00:00Z", "2026-11-20T20:00:00Z", 1, 6750},
		{"2026-11-21T10:00:00Z", "2026-11-21T15:00:00Z", 3, 25750},
	}
	if len(bs) != len(ws) {
		t.Fatalf("blocks = %d, want %d", len(bs), len(ws))
	}
	for i, w := range ws {
		b := bs[i]
		if !b.Start.Equal(at(w.start)) || !b.End.Equal(at(w.end)) || b.Messages != w.messages || b.Tokens.Total() != w.tokens {
			t.Errorf("block %d = %v..%v, %d messages, %d tokens; want %+v", i, b.Start, b.End, b.Messages, b.Tokens.Total(), w)
		}
	}
}

func TestActiveBlockAndTimeRemainingUseTheInjectedClock(t *testing.T) {
	bs := Blocks(fixtureEntries(t))
	b, ok := Active(bs, at("2026-11-21T13:00:00Z"))
	if !ok || !b.Start.Equal(at("2026-11-21T10:00:00Z")) {
		t.Fatalf("active at 13:00 = %v, %v; want the 10:00 block", b.Start, ok)
	}
	if got := b.Remaining(at("2026-11-21T13:00:00Z")); got != 2*time.Hour {
		t.Errorf("remaining = %v, want 2h", got)
	}
	if _, ok := Active(bs, at("2026-11-21T16:00:00Z")); ok {
		t.Error("a block that ended at 15:00 is active at 16:00")
	}
}

func TestInBlockAndInLast(t *testing.T) {
	es := fixtureEntries(t)
	bs := Blocks(es)
	if got := len(InBlock(es, bs[2])); got != 3 {
		t.Errorf("entries in the third block = %d, want 3", got)
	}
	now := at("2026-11-27T12:00:00Z")
	if got := len(InLast(es, WeekLength, now)); got != 4 {
		t.Errorf("entries in the 7 d before 11-27 12:00 = %d, want 4 (the 15:00 one of the 20th and the three of the 21st)", got)
	}
	if got := len(InLast(es, WeekLength, at("2026-11-20T10:00:00Z"))); got != 2 {
		t.Errorf("entries up to 11-20 10:00 = %d, want 2 (later ones are not yet written)", got)
	}
}
