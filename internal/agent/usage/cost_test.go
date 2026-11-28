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

// Every row is Anthropic's pricing page, in cents per million tokens:
// input, output, 5 minute cache write, cache read.
func TestThePriceTableMatchesThePricingPage(t *testing.T) {
	for model, want := range map[string]Price{
		"claude-opus-4-1":   {1500, 7500, 1875, 150},
		"claude-opus-4-5":   {500, 2500, 625, 50},
		"claude-opus-4-6":   {500, 2500, 625, 50},
		"claude-opus-4-7":   {500, 2500, 625, 50},
		"claude-opus-4-8":   {500, 2500, 625, 50},
		"claude-opus-5":     {500, 2500, 625, 50},
		"claude-opus-5-5":   {400, 2000, 500, 20},
		"claude-sonnet-4-6": {300, 1500, 375, 30},
		"claude-sonnet-5":   {200, 1000, 250, 20},
		"claude-sonnet-5-5": {200, 1000, 250, 10},
		"claude-haiku-4-5":  {100, 500, 125, 10},
		"claude-fable-5":    {1000, 5000, 1250, 100},
		"claude-fable-5-1":  {1000, 5000, 1250, 25},
		"claude-fable-5-2":  {800, 4000, 1000, 20},
	} {
		got, ok := PriceFor(model)
		if !ok || got != want {
			t.Errorf("PriceFor(%q) = %+v, %v, want %+v", model, got, ok, want)
		}
	}
}

// What the pricing page does not list stays out of the table, so it shows n/a.
func TestWhatThePricingPageDoesNotListHasNoPrice(t *testing.T) {
	for _, model := range []string{
		"claude-3-opus-20240229", "claude-3-7-sonnet-20250219", "claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022", "claude-3-haiku-20240307",
		"claude-haiku-5-5", // priced by prompt length, which the table does not model
	} {
		if _, ok := PriceFor(model); ok {
			t.Errorf("PriceFor(%q) has a price the pricing page does not give as one rate", model)
		}
	}
}
