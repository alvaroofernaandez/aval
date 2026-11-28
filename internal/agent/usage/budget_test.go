package usage

import (
	"errors"
	"testing"
	"time"
)

func TestParseBudgetsReadsEachUnit(t *testing.T) {
	got, err := ParseBudgets(map[string]string{
		"usage.budget.headless.7d":    "50%",
		"usage.budget.daemon.5h":      "$4.5",
		"usage.budget.interactive.7d": "2000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Budget{
		"headless/7d":    {Headless, "7d", BudgetShare, 50},
		"daemon/5h":      {Daemon, "5h", BudgetDollars, 4.5},
		"interactive/7d": {Interactive, "7d", BudgetTokens, 2000000},
	}
	if len(got) != 3 {
		t.Fatalf("budgets = %v", got)
	}
	for _, b := range got {
		if w := want[string(b.Consumer)+"/"+b.Window]; b != w {
			t.Errorf("budget = %+v, want %+v", b, w)
		}
	}
	if _, err := ParseBudgets(map[string]string{"usage.budget.headless.7d": "lots"}); !errors.Is(err, ErrBadBudget) {
		t.Errorf("err = %v, want ErrBadBudget", err)
	}
}

func TestCheckFlagsOnlyTheBudgetsThatAreOver(t *testing.T) {
	es := fixtureEntries(t)
	week := InLast(es, WeekLength, at("2026-11-21T20:00:00Z"))
	windows := map[string][]Entry{"7d": week, "5h": nil}
	// In the week: headless m4 = 25000 tokens of 61000; interactive the rest.
	vs := Check([]Budget{
		{Headless, "7d", BudgetShare, 50},     // 25000/61000 = 41% -> under
		{Headless, "7d", BudgetTokens, 20000}, // 25000 > 20000 -> over
		{Interactive, "7d", BudgetDollars, 0.1},
		{Interactive, "5h", BudgetTokens, 1}, // nothing in the window -> under
	}, windows)
	if len(vs) != 4 {
		t.Fatalf("verdicts = %d", len(vs))
	}
	if vs[0].Over || !vs[1].Over || vs[3].Over {
		t.Errorf("over = [%v %v %v], want [false true false]", vs[0].Over, vs[1].Over, vs[3].Over)
	}
	// Interactive spent 0.0991 USD plus one response of a model with no price:
	// under the limit as far as known, but only a lower bound.
	if vs[2].Over || !vs[2].Unknown {
		t.Errorf("dollar verdict = %+v, want under and unknown", vs[2])
	}
	if got := vs[1].Used; got != 25000 {
		t.Errorf("used = %v, want 25000 tokens", got)
	}
}

func TestCheckDollarBudgetWithUnpricedUsageIsUnknownNotUnder(t *testing.T) {
	e := []Entry{{Time: time.Now(), Consumer: Daemon, Model: "local-x", Tokens: Tokens{Input: 10}}}
	vs := Check([]Budget{{Daemon, "7d", BudgetDollars, 1}}, map[string][]Entry{"7d": e})
	if vs[0].Over || !vs[0].Unknown {
		t.Errorf("verdict = %+v, want not over and unknown (cost n/a)", vs[0])
	}
}
