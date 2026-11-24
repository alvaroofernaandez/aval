package relay

import (
	"strconv"
	"strings"
)

// Outcome is what one relay did, as the notification reports it.
type Outcome struct {
	Tier      Tier
	Before    int  // context tokens when the relay started
	After     int  // context tokens at the first measure after it; 0 not yet measured
	Floor     int  // tokens a fresh session starts with; 0 unknown
	ColdCache bool // the prompt cache had expired
}

// NotifyText is the notification text for o. Tokens, never percentages
// (ADR 0005); T3 and T4 name the action the person takes.
func NotifyText(o Outcome) string {
	before := Tokens(o.Before)
	switch o.Tier {
	case T2:
		return "Continued in a new session at " + before
	case T3:
		return "Context relay ready at " + before + " · open a new chat and run /continue"
	case T4:
		return "Context at " + before + " needs a relay · start a new session"
	}
	var b strings.Builder
	lead := "Relayed"
	if o.Tier == T1 {
		lead = "Compacted with steering"
	}
	b.WriteString(lead)
	if o.After > 0 {
		b.WriteString(" " + before + " → " + Tokens(o.After))
	} else {
		b.WriteString(" at " + before)
	}
	if o.Floor > 0 {
		b.WriteString(" (floor " + Tokens(o.Floor) + ")")
	}
	if o.ColdCache {
		b.WriteString(" · cold cache")
	}
	return b.String()
}

// Tokens writes a token count short: 900, 212k, 1.3M.
func Tokens(n int) string {
	switch {
	case n >= 1_000_000:
		tenths := (n + 50_000) / 100_000
		return strconv.Itoa(tenths/10) + "." + strconv.Itoa(tenths%10) + "M"
	case n >= 1000:
		return strconv.Itoa((n+500)/1000) + "k"
	}
	return strconv.Itoa(n)
}
