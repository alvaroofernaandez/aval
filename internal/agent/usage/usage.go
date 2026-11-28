// Package usage turns the local agent transcripts into token counts and cost,
// per project, session, day and consumer, with the rolling 5 h and 7 d windows
// (`aval usage`). It reads files and nothing else: no network, no prices
// downloaded, no write to a transcript.
package usage

import "time"

// Consumer says who drove the agent that wrote an entry.
type Consumer string

const (
	// Interactive is a person at a terminal or an editor.
	Interactive Consumer = "interactive"
	// Daemon is an agent driven over RPC, such as Pi.
	Daemon Consumer = "daemon"
	// Headless is `claude -p`, the Agent SDK and factories built on them.
	Headless Consumer = "headless"
)

// Tokens are the four counters every response reports.
type Tokens struct {
	Input, Output, CacheWrite, CacheRead int64
}

// Total is the sum of the four counters, as ccusage's totalTokens.
func (t Tokens) Total() int64 { return t.Input + t.Output + t.CacheWrite + t.CacheRead }

// Add returns the counter-wise sum.
func (t Tokens) Add(o Tokens) Tokens {
	return Tokens{t.Input + o.Input, t.Output + o.Output, t.CacheWrite + o.CacheWrite, t.CacheRead + o.CacheRead}
}

// Entry is one assistant response that reported usage.
type Entry struct {
	Time     time.Time
	Source   string // "claude" or "pi"
	Project  string
	Session  string
	Model    string
	Consumer Consumer
	Tokens   Tokens
	// Reported is the cost the runtime itself wrote on the entry (Pi does), in
	// Cost units; HasReported is false when it wrote none.
	Reported    Cost
	HasReported bool
}

// Cost is an amount of money in 1e-8 USD, so token counts times a price per
// million tokens stay exact integers.
type Cost int64

// Dollars is the amount in USD.
func (c Cost) Dollars() float64 { return float64(c) / 1e8 }
