package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	cellNA = "n/a"
	maxKey = 30
)

// Render writes the report as the plain table `aval usage` prints: ASCII, no
// colour, counts instead of decoration (.agents/DESIGN.md section 10).
func Render(w io.Writer, r Report) error {
	window := "all time"
	if r.Window != "" {
		window = "window " + r.Window
	}
	fmt.Fprintf(w, "usage by %s, %s\n\n", r.By, window)
	if err := renderRows(w, r); err != nil {
		return err
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "5h block   %s\n", blockLine(r))
	fmt.Fprintf(w, "7d window  %s\n", windowLine(r, r.Week7d))
	if err := renderBudgets(w, r); err != nil {
		return err
	}
	renderNotes(w, r)
	return nil
}

func renderRows(w io.Writer, r Report) error {
	api := r.Billing == BillingAPI
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	head := []string{strings.ToUpper(string(r.By)), "MESSAGES", "INPUT", "OUTPUT", "CACHE WRITE", "CACHE READ", "TOTAL"}
	if api {
		head = append(head, "COST")
	}
	fmt.Fprintln(tw, strings.Join(head, "\t"))
	line := func(key string, row Row) {
		cells := []string{key, fmt.Sprint(row.Messages), fmt.Sprint(row.Tokens.Input), fmt.Sprint(row.Tokens.Output),
			fmt.Sprint(row.Tokens.CacheWrite), fmt.Sprint(row.Tokens.CacheRead), fmt.Sprint(row.Tokens.Total())}
		if api {
			cells = append(cells, costCell(row))
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	for _, row := range r.Rows {
		line(clipFront(row.Key, maxKey), row)
	}
	line("total", r.Total)
	if err := tw.Flush(); err != nil {
		return err
	}
	if api && r.Total.Unpriced > 0 {
		fmt.Fprintf(w, "\n+ cost leaves out %s from models with no price: %s (prices as of %s)\n",
			plural(r.Total.Unpriced, "response"), strings.Join(r.Total.UnpricedModels, ", "), PricesEffective)
	} else if api {
		fmt.Fprintf(w, "\nPrices as of %s, estimated from list prices.\n", PricesEffective)
	}
	return nil
}

func renderBudgets(w io.Writer, r Report) error {
	if len(r.Checks) == 0 {
		return nil
	}
	fmt.Fprintln(w, "\nBudgets")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, v := range r.Checks {
		fmt.Fprintf(tw, "  %s %s\t%s\t%s\n", v.Consumer, v.Window, budgetText(v), verdictText(v))
	}
	return tw.Flush()
}

func renderNotes(w io.Writer, r Report) {
	switch r.Billing {
	case BillingSubscription:
		if r.Block5h.Share == nil && r.Week7d.Share == nil {
			fmt.Fprintln(w, "\nPlan share is n/a: no recent rate limits from the runtime. The status line keeps them while Claude Code runs; or pass --rate-limits <file>.")
		}
	}
	fmt.Fprintf(w, "\n%s read", plural(r.Files, "file"))
	if r.Corrupt > 0 {
		fmt.Fprintf(w, ", %s skipped as corrupt", plural(r.Corrupt, "line"))
	}
	fmt.Fprintln(w)
}

func blockLine(r Report) string {
	b := r.Block5h
	if !b.Active {
		return "none open" + extras(r, b)
	}
	return fmt.Sprintf("%d tokens, closes %s in %s%s", b.Tokens.Total(), b.End.In(r.Loc).Format("2006-01-02 15:04 MST"),
		hm(b.Remaining), extras(r, b))
}

func windowLine(r Report, w WindowInfo) string {
	return fmt.Sprintf("%d tokens%s", w.Tokens.Total(), extras(r, w))
}

// extras is what follows the tokens of a window: the cost in api mode, the
// plan share the runtime reported (or n/a) in subscription mode.
func extras(r Report, w WindowInfo) string {
	switch r.Billing {
	case BillingAPI:
		if !w.Active && w.Messages == 0 {
			return ""
		}
		return ", " + costCell(w.Row)
	case BillingSubscription:
		if w.Share == nil {
			return ", plan share " + cellNA
		}
		return fmt.Sprintf(", plan share %.0f%% (reported by the runtime)", *w.Share)
	}
	return ""
}

func costCell(r Row) string {
	if r.Messages > 0 && r.Unpriced == r.Messages {
		return cellNA
	}
	s := fmt.Sprintf("$%.4f", r.Cost.Dollars())
	if r.Unpriced > 0 {
		s += "+"
	}
	return s
}

func budgetText(v Verdict) string {
	switch v.Kind {
	case BudgetShare:
		return fmt.Sprintf("%.0f%% of the window's tokens", v.Limit)
	case BudgetDollars:
		return fmt.Sprintf("$%g", v.Limit)
	}
	return fmt.Sprintf("%.0f tokens", v.Limit)
}

func verdictText(v Verdict) string {
	switch {
	case v.Skipped:
		return "skipped: dollar budgets need usage.billing api"
	case v.Over:
		return fmt.Sprintf("OVER, used %s", usedText(v))
	case v.Unknown:
		return fmt.Sprintf("ok so far, used %s, some cost is n/a", usedText(v))
	}
	return fmt.Sprintf("ok, used %s", usedText(v))
}

func usedText(v Verdict) string {
	switch v.Kind {
	case BudgetShare:
		return fmt.Sprintf("%.0f%%", v.Used)
	case BudgetDollars:
		return fmt.Sprintf("$%.4f", v.Used)
	}
	return fmt.Sprintf("%.0f", v.Used)
}

func hm(d time.Duration) string {
	d = d.Round(time.Minute)
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// clipFront keeps the tail of a path that does not fit.
func clipFront(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-(n-3):]
}

type jsonRow struct {
	Key            string          `json:"key,omitempty"`
	Messages       int             `json:"messages"`
	Input          int64           `json:"input"`
	Output         int64           `json:"output"`
	CacheWrite     int64           `json:"cache_write"`
	CacheRead      int64           `json:"cache_read"`
	Total          int64           `json:"total"`
	CostUSD        json.RawMessage `json:"cost_usd,omitempty"`
	Unpriced       int             `json:"unpriced,omitempty"`
	UnpricedModels []string        `json:"unpriced_models,omitempty"`
}

type jsonWindow struct {
	jsonRow
	Active           *bool           `json:"active,omitempty"`
	Start            string          `json:"start,omitempty"`
	End              string          `json:"end,omitempty"`
	RemainingSeconds *int64          `json:"remaining_seconds,omitempty"`
	SharePercent     json.RawMessage `json:"share_percent,omitempty"`
}

type jsonCheck struct {
	Consumer Consumer   `json:"consumer"`
	Window   string     `json:"window"`
	Unit     BudgetKind `json:"unit"`
	Limit    float64    `json:"limit"`
	Used     float64    `json:"used"`
	Over     bool       `json:"over"`
	Unknown  bool       `json:"unknown,omitempty"`
	Skipped  bool       `json:"skipped,omitempty"`
}

type jsonReport struct {
	Billing         string      `json:"billing"`
	By              By          `json:"by"`
	Window          string      `json:"window"`
	PricesEffective string      `json:"prices_effective,omitempty"`
	Rows            []jsonRow   `json:"rows"`
	Total           jsonRow     `json:"total"`
	Block5h         jsonWindow  `json:"block_5h"`
	Week7d          jsonWindow  `json:"week_7d"`
	Checks          []jsonCheck `json:"checks"`
	Over            bool        `json:"over"`
	Files           int         `json:"files"`
	Corrupt         int         `json:"corrupt_lines"`
}

// RenderJSON writes the report as the stable document `aval usage --json`
// prints. Cost fields exist only under billing api, share_percent only under
// subscription; null is n/a.
func RenderJSON(w io.Writer, r Report) error {
	api, sub := r.Billing == BillingAPI, r.Billing == BillingSubscription
	row := func(key string, x Row) jsonRow {
		j := jsonRow{Key: key, Messages: x.Messages, Input: x.Tokens.Input, Output: x.Tokens.Output,
			CacheWrite: x.Tokens.CacheWrite, CacheRead: x.Tokens.CacheRead, Total: x.Tokens.Total(),
			Unpriced: x.Unpriced, UnpricedModels: x.UnpricedModels}
		if api {
			j.CostUSD = json.RawMessage("null")
			if !(x.Messages > 0 && x.Unpriced == x.Messages) {
				j.CostUSD = json.RawMessage(fmt.Sprint(x.Cost.Dollars()))
			}
		}
		return j
	}
	share := func(p *float64) json.RawMessage {
		if !sub {
			return nil
		}
		if p == nil {
			return json.RawMessage("null")
		}
		return json.RawMessage(fmt.Sprint(*p))
	}
	out := jsonReport{
		Billing: r.Billing, By: r.By, Window: r.Window, Rows: []jsonRow{}, Checks: []jsonCheck{},
		Total: row("", r.Total), Over: r.Over(), Files: r.Files, Corrupt: r.Corrupt,
	}
	if api {
		out.PricesEffective = PricesEffective
	}
	if out.Window == "" {
		out.Window = "all"
	}
	for _, x := range r.Rows {
		out.Rows = append(out.Rows, row(x.Key, x))
	}
	active, rem := r.Block5h.Active, int64(r.Block5h.Remaining.Seconds())
	out.Block5h = jsonWindow{jsonRow: row("", r.Block5h.Row), Active: &active, SharePercent: share(r.Block5h.Share)}
	if active {
		out.Block5h.Start, out.Block5h.End = r.Block5h.Start.UTC().Format(time.RFC3339), r.Block5h.End.UTC().Format(time.RFC3339)
		out.Block5h.RemainingSeconds = &rem
	}
	out.Week7d = jsonWindow{jsonRow: row("", r.Week7d.Row), SharePercent: share(r.Week7d.Share)}
	for _, v := range r.Checks {
		out.Checks = append(out.Checks, jsonCheck{Consumer: v.Consumer, Window: v.Window, Unit: v.Kind, Limit: v.Limit,
			Used: v.Used, Over: v.Over, Unknown: v.Unknown, Skipped: v.Skipped})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
