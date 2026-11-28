package usage

import "testing"

func TestPriceOfIsExactForAKnownModel(t *testing.T) {
	p, ok := PriceFor("claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("claude-sonnet-4-20250514 is not in the table")
	}
	// m1 of the fixture: ccusage prices sonnet 4 at $3 in, $15 out, $3.75 cache write, $0.30 cache read per million tokens.
	got := p.Of(Tokens{Input: 1000, Output: 500, CacheWrite: 2000, CacheRead: 10000})
	if got != 2_100_000 {
		t.Errorf("cost = %d, want 2100000 (0.021 USD)", got)
	}
}

func TestPriceForNormalisesTheModelName(t *testing.T) {
	for model, want := range map[string]bool{
		"claude-sonnet-4-20250514":   true,
		"claude-sonnet-4-5-20250929": true,
		"claude-opus-4-1-20250805":   true,
		"claude-haiku-4-5":           true,
		"claude-opus-9":              false, // a model released after the table
		"claude-mystery-9":           false,
		"llama-x":                    false,
		"":                           false,
	} {
		if _, ok := PriceFor(model); ok != want {
			t.Errorf("PriceFor(%q) known = %v, want %v", model, ok, want)
		}
	}
}

func TestEntryCostNeverInventsOne(t *testing.T) {
	tok := Tokens{Input: 1000, Output: 500}
	if _, ok := EntryCost(Entry{Model: "claude-mystery-9", Tokens: tok}); ok {
		t.Error("unknown model with no reported cost has a cost")
	}
	c, ok := EntryCost(Entry{Model: "local-x", Tokens: tok, Reported: 123, HasReported: true})
	if !ok || c != 123 {
		t.Errorf("reported cost = %d, %v; want the runtime's 123", c, ok)
	}
	c, ok = EntryCost(Entry{Model: "claude-sonnet-4-20250514", Tokens: tok, Reported: 1, HasReported: true})
	if !ok || c != 1000*300+500*1500 {
		t.Errorf("table cost = %d, %v; want the table's, not the reported", c, ok)
	}
}
