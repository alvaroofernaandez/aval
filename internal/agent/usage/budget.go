package usage

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Budget caps what one consumer may use in one window.
type Budget struct {
	Consumer Consumer
	Window   string // "5h" or "7d"
	Kind     BudgetKind
	// Limit is a token count, a percentage (50 for "50%") or dollars.
	Limit float64
}

// BudgetKind is the unit of a budget.
type BudgetKind string

const (
	BudgetTokens  BudgetKind = "tokens"
	BudgetShare   BudgetKind = "share"
	BudgetDollars BudgetKind = "dollars"
)

// ParseBudgets reads the usage.budget.<consumer>.<window> settings, as
// internal/core/config validated them.
// ErrBadBudget is a budget value that is not a token count, a share or dollars.
var ErrBadBudget = errors.New("usage: bad budget")

func ParseBudgets(settings map[string]string) ([]Budget, error) {
	var out []Budget
	for _, k := range slices.Sorted(maps.Keys(settings)) {
		rest, ok := strings.CutPrefix(k, "usage.budget.")
		if !ok {
			continue
		}
		consumer, window, ok := strings.Cut(rest, ".")
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrBadBudget, k)
		}
		b := Budget{Consumer: Consumer(consumer), Window: window}
		v := settings[k]
		var err error
		switch {
		case strings.HasSuffix(v, "%"):
			b.Kind = BudgetShare
			b.Limit, err = strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		case strings.HasPrefix(v, "$"):
			b.Kind = BudgetDollars
			b.Limit, err = strconv.ParseFloat(strings.TrimPrefix(v, "$"), 64)
		default:
			b.Kind = BudgetTokens
			b.Limit, err = strconv.ParseFloat(v, 64)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %s = %q", ErrBadBudget, k, v)
		}
		out = append(out, b)
	}
	return out, nil
}

// Verdict is one budget checked against what was used.
type Verdict struct {
	Budget
	Used float64 // in the budget's unit
	Over bool
	// Unknown is set for a dollar budget when cost n/a made the sum a lower bound
	// that is still under the limit.
	Unknown bool
}

// Check compares each budget with the usage of its consumer in its window.
// windows maps "5h" and "7d" to the entries inside that window.
func Check(budgets []Budget, windows map[string][]Entry) []Verdict {
	out := make([]Verdict, 0, len(budgets))
	for _, b := range budgets {
		es := windows[b.Window]
		var mine []Entry
		for _, e := range es {
			if e.Consumer == b.Consumer {
				mine = append(mine, e)
			}
		}
		all, own := Sum(Aggregate(es, ByConsumer, time.UTC)), Sum(Aggregate(mine, ByConsumer, time.UTC))
		v := Verdict{Budget: b}
		switch b.Kind {
		case BudgetTokens:
			v.Used = float64(own.Tokens.Total())
		case BudgetShare:
			if t := all.Tokens.Total(); t > 0 {
				v.Used = 100 * float64(own.Tokens.Total()) / float64(t)
			}
		case BudgetDollars:
			v.Used = own.Cost.Dollars()
			v.Unknown = own.Unpriced > 0
		}
		v.Over = v.Used > b.Limit
		v.Unknown = v.Unknown && !v.Over
		out = append(out, v)
	}
	return out
}
