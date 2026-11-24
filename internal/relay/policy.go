// Package relay is the I/O-free core of the context relay: it decides when
// to relay, what the new context holds and what the runtime shims speak.
// It reads no file, starts no process and touches no network (ADR 0005).
package relay

import "time"

// Action is what Decide asks the adapter to do.
type Action int

const (
	// ActionNone leaves the context alone.
	ActionNone Action = iota
	// ActionPrecompute prepares the note ahead of the relay.
	ActionPrecompute
	// ActionRelay replaces the context now.
	ActionRelay
)

// Reading is one measure of the context.
type Reading struct {
	Tokens      int     // tokens in the context
	Window      int     // model window in tokens; 0 when unknown
	ErrPct      float64 // relative error bound of Tokens (0.15 is 15%)
	FloorTokens int     // tokens a fresh session starts with
}

// Snapshot is what an adapter knows when it asks for a decision.
type Snapshot struct {
	Reading  Reading
	CacheAge time.Duration // time since the prompt cache was last written
	CacheTTL time.Duration // cache lifetime; 0 when the runtime does not say
}

// Decision is the answer of Decide.
type Decision struct {
	Action Action
}

// Policy holds the thresholds. The defaults are provisional starting values
// from odd/tasks/relay-cost-model.md, to be calibrated (#74).
type Policy struct {
	SoftTokens     int     // precompute from here
	HardTokens     int     // relay from here with a warm cache
	SoftPct        float64 // caps SoftTokens on small windows
	HardPct        float64 // caps HardTokens on small windows
	ColdMinTokens  int     // relay a cold cache from here
	MinFreedTokens int     // a relay must free at least this much
}

// DefaultPolicy returns the provisional defaults.
func DefaultPolicy() Policy {
	return Policy{
		SoftTokens:     160_000,
		HardTokens:     200_000,
		SoftPct:        0.50,
		HardPct:        0.60,
		ColdMinTokens:  100_000,
		MinFreedTokens: 20_000,
	}
}

// Decide returns what to do with the context in s. It compares the upper
// bound of the reading, so a poor signal relays early rather than late.
func (p Policy) Decide(s Snapshot) Decision {
	r := s.Reading
	upper := int(float64(r.Tokens) * (1 + r.ErrPct))
	if upper-r.FloorTokens < p.MinFreedTokens {
		return Decision{Action: ActionNone}
	}
	if s.cold() && upper >= p.ColdMinTokens {
		return Decision{Action: ActionRelay}
	}
	if upper >= capped(p.HardTokens, p.HardPct, r.Window) {
		return Decision{Action: ActionRelay}
	}
	if upper >= capped(p.SoftTokens, p.SoftPct, r.Window) {
		return Decision{Action: ActionPrecompute}
	}
	return Decision{Action: ActionNone}
}

// cold reports a cache the runtime says has expired. An unknown TTL is
// treated as warm: the relay never assumes a lifetime.
func (s Snapshot) cold() bool {
	return s.CacheTTL > 0 && s.CacheAge > s.CacheTTL
}

// capped is min(tokens, pct × window); an unknown window keeps tokens.
func capped(tokens int, pct float64, window int) int {
	if window <= 0 || pct <= 0 {
		return tokens
	}
	return min(tokens, int(pct*float64(window)))
}
