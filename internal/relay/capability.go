package relay

// Compaction is what the runtime lets an adapter do at compaction time.
type Compaction int

const (
	CompactionNone    Compaction = iota // no compaction event reaches the adapter
	CompactionObserve                   // the adapter sees it, cannot change it
	CompactionSteer                     // the adapter can pass instructions to the summariser
	CompactionReplace                   // the adapter answers with the new context itself
)

// SessionControl is whether the adapter can start a new session in place.
type SessionControl int

const (
	SessionNone    SessionControl = iota
	SessionRestart                // clear and resubmit from the adapter (S6)
)

// Capabilities is the closed capability vector of one runtime. It holds only
// what Tier reads; #8 adopts and extends it (D8 of odd/tasks/relay.md).
type Capabilities struct {
	Compaction Compaction
	Inject     bool // the adapter can add text to the next context
	Session    SessionControl
	MCP        bool // the runtime loads an aval MCP server
	Rules      bool // the runtime reads a generated rules file
}

// RunMode is how the runtime is started.
type RunMode int

const (
	ModeNativeUI  RunMode = iota // a person drives the runtime's own UI
	ModeHeadless                 // aval starts it without a UI
	ModeACPHosted                // aval hosts it over ACP
)

// Tier is the delivery tier, T0 the best.
type Tier int

const (
	T0 Tier = iota // compaction is replace: no model call, no restart
	T1             // steered native compaction plus an inject point
	T2             // programmatic new session
	T3             // MCP plus a rules file: one user action
	T4             // notification only
)

// Probe names a capability check that can fail at run time.
type Probe int

const (
	ProbeReplace Probe = iota
	ProbeSteer
	ProbeInject
	ProbeSession
	ProbeMCP
)

// TierOf returns the best tier c allows in mode. Headless and ACP-hosted
// runs put aval in the path, so it can always start a new session.
func TierOf(c Capabilities, mode RunMode) Tier {
	switch {
	case c.Compaction == CompactionReplace:
		return T0
	case c.Compaction == CompactionSteer && c.Inject:
		return T1
	case c.Session == SessionRestart || mode != ModeNativeUI:
		return T2
	case c.MCP && c.Rules:
		return T3
	}
	return T4
}

// Degrade returns c without the capability whose probe failed. A failed
// replace probe falls back to steering the native compaction.
func Degrade(c Capabilities, failed Probe) Capabilities {
	switch failed {
	case ProbeReplace:
		c.Compaction = min(c.Compaction, CompactionSteer)
	case ProbeSteer:
		c.Compaction = min(c.Compaction, CompactionObserve)
	case ProbeInject:
		c.Inject = false
	case ProbeSession:
		c.Session = SessionNone
	case ProbeMCP:
		c.MCP = false
	}
	return c
}
