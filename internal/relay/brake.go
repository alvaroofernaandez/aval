package relay

// BrakeReason says why a relay is refused. BrakeNone lets it go.
type BrakeReason int

const (
	BrakeNone BrakeReason = iota
	// BrakeNoProgress: the fingerprint has not changed since the last relay.
	BrakeNoProgress
	// BrakeThrash: too few turns since the last relay.
	BrakeThrash
	// BrakeMaxChain: the chain already holds MaxChain relays.
	BrakeMaxChain
	// BrakeCostCap: the chain has spent its cost cap.
	BrakeCostCap
)

// String is the reason's name on the wire.
func (r BrakeReason) String() string {
	switch r {
	case BrakeNoProgress:
		return "no_progress"
	case BrakeThrash:
		return "thrash"
	case BrakeMaxChain:
		return "max_chain"
	case BrakeCostCap:
		return "cost_cap"
	}
	return "none"
}

// Attempt is the relay being considered.
type Attempt struct {
	Fingerprint string
	Turn        int
}

// Brakes are the limits that stop a relay loop. A zero limit is off.
type Brakes struct {
	MaxChain        int
	MinTurnsBetween int
	CostCapUSD      float64
}

// DefaultBrakes returns provisional limits.
func DefaultBrakes() Brakes {
	return Brakes{MaxChain: 5, MinTurnsBetween: 3}
}

// Check returns the first brake that refuses a, or BrakeNone.
func (b Brakes) Check(c Chain, a Attempt) BrakeReason {
	if b.MaxChain > 0 && c.Epoch() >= b.MaxChain {
		return BrakeMaxChain
	}
	if b.CostCapUSD > 0 && c.cost() >= b.CostCapUSD {
		return BrakeCostCap
	}
	last, ok := c.last()
	if !ok {
		return BrakeNone
	}
	if a.Fingerprint == last.Fingerprint {
		return BrakeNoProgress
	}
	if b.MinTurnsBetween > 0 && a.Turn-last.Turn < b.MinTurnsBetween {
		return BrakeThrash
	}
	return BrakeNone
}
