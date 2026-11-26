package relay

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestReadUsage(t *testing.T) {
	f, err := os.Open("testdata/usage.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	turns := ReadUsage(f)
	if len(turns) != 3 {
		t.Fatalf("got %d turns, want 3 (malformed and usage-less lines skipped): %+v", len(turns), turns)
	}
	want := []Turn{
		{Tokens: 68668, At: time.Date(2026, 11, 26, 10, 0, 5, 0, time.UTC)},
		{Tokens: 10003, At: time.Date(2026, 11, 26, 10, 1, 0, 0, time.UTC), Sidechain: true},
		{Tokens: 70105, At: time.Date(2026, 11, 26, 10, 3, 0, 0, time.UTC)},
	}
	for i, w := range want {
		if turns[i].Tokens != w.Tokens || !turns[i].At.Equal(w.At) || turns[i].Sidechain != w.Sidechain {
			t.Errorf("turn %d = %+v, want %+v", i, turns[i], w)
		}
	}
}

func TestUsageFirstAndLast(t *testing.T) {
	f, _ := os.Open("testdata/usage.jsonl")
	defer f.Close()
	turns := ReadUsage(f)
	if got, ok := FirstTurn(turns, false); !ok || got.Tokens != 68668 {
		t.Errorf("first main = %+v %v", got, ok)
	}
	if got, ok := FirstTurn(turns, true); !ok || got.Tokens != 10003 {
		t.Errorf("first sidechain = %+v %v", got, ok)
	}
	if got, ok := LastTurn(turns, false); !ok || got.Tokens != 70105 {
		t.Errorf("last main = %+v %v", got, ok)
	}
	if _, ok := LastTurn(nil, false); ok {
		t.Error("last of none reported ok")
	}
}

func TestReadUsage_BadTimestampKeepsTokens(t *testing.T) {
	turns := ReadUsage(strings.NewReader(`{"type":"assistant","timestamp":"nope","message":{"usage":{"input_tokens":1}}}` + "\n"))
	if len(turns) != 1 || turns[0].Tokens != 1 || !turns[0].At.IsZero() {
		t.Fatalf("got %+v", turns)
	}
}
