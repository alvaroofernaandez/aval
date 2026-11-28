package usage

import (
	"strings"
	"testing"
	"time"
)

func TestParseRateLimitsReadsTheStatusLineDocument(t *testing.T) {
	rl := ParseRateLimits(strings.NewReader(`{"rate_limits":{"five_hour":{"used_percentage":42.4,"resets_at":0},"seven_day":{"used_percentage":13}}}`))
	if rl.FiveHour == nil || *rl.FiveHour != 42.4 || rl.SevenDay == nil || *rl.SevenDay != 13 {
		t.Errorf("rates = %+v", rl)
	}
	for _, in := range []string{``, `not json`, `{}`, `{"rate_limits":{}}`} {
		if rl := ParseRateLimits(strings.NewReader(in)); rl.FiveHour != nil || rl.SevenDay != nil {
			t.Errorf("%q: rates = %+v, want none", in, rl)
		}
	}
}

func buildFixture(t *testing.T, o Options) Report {
	t.Helper()
	c, err := Read(fixtureRoots())
	if err != nil {
		t.Fatal(err)
	}
	if o.Now.IsZero() {
		o.Now = at("2026-11-21T13:00:00Z")
	}
	if o.Loc == nil {
		o.Loc = time.UTC
	}
	if o.By == "" {
		o.By = ByDay
	}
	return Build(c, o)
}

func TestBuildSummarisesBothWindowsAtTheInjectedTime(t *testing.T) {
	r := buildFixture(t, Options{Billing: BillingAPI})
	if !r.Block5h.Active || r.Block5h.Remaining != 2*time.Hour || r.Block5h.Tokens.Total() != 25750 {
		t.Errorf("5h = %+v, want an active block with 2h left and 25750 tokens", r.Block5h)
	}
	if r.Week7d.Tokens.Total() != 61000 || r.Week7d.Messages != 6 {
		t.Errorf("7d = %+v, want the whole corpus", r.Week7d)
	}
	if r.Total.Messages != 6 || r.Files != 3 || r.Corrupt != 1 {
		t.Errorf("total/files/corrupt = %d/%d/%d", r.Total.Messages, r.Files, r.Corrupt)
	}
}

func TestBuildWindowLimitsTheRows(t *testing.T) {
	r := buildFixture(t, Options{Window: "5h", By: ByConsumer})
	if r.Total.Messages != 3 {
		t.Errorf("rows under --window 5h hold %d responses, want the 3 of the open block", r.Total.Messages)
	}
	r = buildFixture(t, Options{Window: "7d", By: ByConsumer, Now: at("2026-11-27T12:00:00Z")})
	if r.Total.Messages != 4 {
		t.Errorf("rows under --window 7d at 11-27 hold %d responses, want 4", r.Total.Messages)
	}
}

func TestSubscriptionShowsRuntimeSharesOrNothingAndNeverDollars(t *testing.T) {
	five := 42.4
	r := buildFixture(t, Options{Billing: BillingSubscription, Rates: RateLimits{FiveHour: &five}})
	if r.Block5h.Share == nil || *r.Block5h.Share != 42.4 {
		t.Errorf("5h share = %v, want the reported 42.4", r.Block5h.Share)
	}
	if r.Week7d.Share != nil {
		t.Errorf("7d share = %v, want n/a: the runtime did not report it", *r.Week7d.Share)
	}
}

func TestBuildChecksBudgetsAndReportsOverrun(t *testing.T) {
	r := buildFixture(t, Options{Budgets: []Budget{{Headless, "7d", BudgetTokens, 20000}}})
	if len(r.Checks) != 1 || !r.Over() {
		t.Errorf("checks = %+v over = %v, want one overrun", r.Checks, r.Over())
	}
	r = buildFixture(t, Options{Budgets: []Budget{{Headless, "7d", BudgetTokens, 30000}}})
	if r.Over() {
		t.Error("under budget reported as over")
	}
}
