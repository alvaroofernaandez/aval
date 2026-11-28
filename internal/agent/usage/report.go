package usage

import (
	"encoding/json"
	"io"
	"time"
)

// Billing modes (the usage.billing setting). Unset means subscription: that
// shows tokens and the plan share the runtime reports, never a dollar amount
// nobody pays.
const (
	BillingNone         = ""
	BillingAPI          = "api"
	BillingSubscription = "subscription"
)

// RateLimits are the plan-window percentages a runtime reported, as Claude Code
// hands them to the status line (rate_limits.five_hour and .seven_day). nil is
// "not reported": aval never estimates a plan limit.
type RateLimits struct {
	FiveHour, SevenDay *float64
}

// ParseRateLimits reads a status line JSON document. Anything it cannot read
// is "not reported".
func ParseRateLimits(r io.Reader) RateLimits {
	var in struct {
		RateLimits *struct {
			FiveHour *struct {
				Used *float64 `json:"used_percentage"`
			} `json:"five_hour"`
			SevenDay *struct {
				Used *float64 `json:"used_percentage"`
			} `json:"seven_day"`
		} `json:"rate_limits"`
	}
	var out RateLimits
	if json.NewDecoder(r).Decode(&in) != nil || in.RateLimits == nil {
		return out
	}
	if w := in.RateLimits.FiveHour; w != nil {
		out.FiveHour = w.Used
	}
	if w := in.RateLimits.SevenDay; w != nil {
		out.SevenDay = w.Used
	}
	return out
}

// Options are the inputs of a report; the clock and the time zone are injected.
type Options struct {
	By      By
	Window  string // "", "5h" or "7d": limit the rows to that window
	Now     time.Time
	Loc     *time.Location
	Billing string
	Budgets []Budget
	Rates   RateLimits
}

// WindowInfo is a rolling window as the summary shows it.
type WindowInfo struct {
	Row
	Active    bool // 5 h only: a block is open
	Start     time.Time
	End       time.Time
	Remaining time.Duration
	// Share is the percentage of the plan window the runtime reported; nil is n/a.
	Share *float64
}

// Report is everything `aval usage` prints.
type Report struct {
	Options
	Rows    []Row
	Total   Row
	Block5h WindowInfo
	Week7d  WindowInfo
	Checks  []Verdict
	Files   int
	Corrupt int
}

// Over reports whether any budget is exceeded.
func (r Report) Over() bool {
	for _, v := range r.Checks {
		if v.Over {
			return true
		}
	}
	return false
}

// Build assembles the report from a corpus.
func Build(c Corpus, o Options) Report {
	if o.Loc == nil {
		o.Loc = time.UTC
	}
	if o.Billing == BillingNone {
		o.Billing = BillingSubscription
	}
	r := Report{Options: o, Files: c.Files, Corrupt: c.Corrupt}
	week := InLast(c.Entries, WeekLength, o.Now)
	r.Week7d = WindowInfo{Row: Sum(Aggregate(week, ByDay, o.Loc)), Share: o.Rates.SevenDay}
	r.Block5h.Share = o.Rates.FiveHour
	var block []Entry
	if b, ok := Active(Blocks(c.Entries), o.Now); ok {
		block = InBlock(c.Entries, b)
		r.Block5h = WindowInfo{Row: b.Row, Active: true, Start: b.Start, End: b.End,
			Remaining: b.Remaining(o.Now), Share: o.Rates.FiveHour}
	}
	rows := c.Entries
	switch o.Window {
	case "5h":
		rows = block
	case "7d":
		rows = week
	}
	r.Rows = Aggregate(rows, o.By, o.Loc)
	r.Total = Sum(r.Rows)
	var live []Budget
	for _, b := range o.Budgets {
		if b.Kind == BudgetDollars && o.Billing != BillingAPI {
			r.Checks = append(r.Checks, Verdict{Budget: b, Skipped: true})
			continue
		}
		live = append(live, b)
	}
	r.Checks = append(r.Checks, Check(live, map[string][]Entry{"5h": block, "7d": week})...)
	return r
}
