package usage

import (
	"strconv"
	"strings"
)

// PricesEffective is the date the embedded price table was written. aval never
// downloads prices: a model the table does not list shows no cost.
const PricesEffective = "2026-11-28"

// Price is what a model charges per million tokens, in cents.
type Price struct{ Input, Output, CacheWrite, CacheRead int64 }

// Of is the cost of the tokens at this price: tokens times cents per million
// tokens is exactly the Cost unit (1e-8 USD).
func (p Price) Of(t Tokens) Cost {
	return Cost(t.Input*p.Input + t.Output*p.Output + t.CacheWrite*p.CacheWrite + t.CacheRead*p.CacheRead)
}

// prices is Anthropic's public list price per million tokens, in cents, per
// model family as it appears in a transcript once the date suffix is dropped.
// A family not listed here has no price: newer models are added by a release of
// aval, never guessed.
var prices = map[string]Price{
	"claude-opus-4":     {1500, 7500, 1875, 150},
	"claude-opus-4-1":   {1500, 7500, 1875, 150},
	"claude-opus-4-5":   {500, 2500, 625, 50},
	"claude-3-opus":     {1500, 7500, 1875, 150},
	"claude-sonnet-4":   {300, 1500, 375, 30},
	"claude-sonnet-4-5": {300, 1500, 375, 30},
	"claude-3-7-sonnet": {300, 1500, 375, 30},
	"claude-3-5-sonnet": {300, 1500, 375, 30},
	"claude-haiku-4-5":  {100, 500, 125, 10},
	"claude-3-5-haiku":  {80, 400, 100, 8},
	"claude-3-haiku":    {25, 125, 30, 3},
}

// family drops the "-latest" alias and a trailing -YYYYMMDD release date.
func family(model string) string {
	model = strings.TrimSuffix(model, "-latest")
	if i := strings.LastIndexByte(model, '-'); i >= 0 && len(model)-i-1 == 8 {
		if _, err := strconv.Atoi(model[i+1:]); err == nil {
			model = model[:i]
		}
	}
	return model
}

// PriceFor looks a model up in the table.
func PriceFor(model string) (Price, bool) {
	p, ok := prices[family(model)]
	return p, ok
}

// EntryCost is the cost of one response: the price table first; for a model
// the table does not know, the cost the runtime itself reported; otherwise
// none. It is never estimated.
func EntryCost(e Entry) (Cost, bool) {
	if p, ok := PriceFor(e.Model); ok {
		return p.Of(e.Tokens), true
	}
	return e.Reported, e.HasReported
}
