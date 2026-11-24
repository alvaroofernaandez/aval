package relay

import "slices"

// Link is one relay in a chain.
type Link struct {
	SessionID   string  // the runtime session the relay ran in
	Fingerprint string  // progress fingerprint at the relay
	Turn        int     // turn number at the relay
	CostUSD     float64 // cost attributed to the relay
}

// Chain is the sequence of relays of one piece of work. A relay that clears
// the session starts a new runtime session id; the chain follows it.
type Chain struct {
	Links []Link
}

// Epoch is the number of relays so far; a fresh chain is epoch 0.
func (c Chain) Epoch() int { return len(c.Links) }

// Record returns the chain with l appended; c is not modified.
func (c Chain) Record(l Link) Chain {
	return Chain{Links: append(slices.Clone(c.Links), l)}
}

// Owns reports whether sessionID belongs to this chain.
func (c Chain) Owns(sessionID string) bool {
	return slices.ContainsFunc(c.Links, func(l Link) bool { return l.SessionID == sessionID })
}

// last returns the latest relay, if any.
func (c Chain) last() (Link, bool) {
	if len(c.Links) == 0 {
		return Link{}, false
	}
	return c.Links[len(c.Links)-1], true
}

// cost is the total cost of the chain.
func (c Chain) cost() float64 {
	var sum float64
	for _, l := range c.Links {
		sum += l.CostUSD
	}
	return sum
}
