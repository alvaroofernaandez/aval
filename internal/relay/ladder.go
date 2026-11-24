package relay

// Level is a rung of the fallback ladder of #65, 0 the best.
type Level int

const (
	Level0 Level = iota // the mod answers the compaction with the capsule
	Level1              // the mod steers the native summary
	Level2              // classic hooks add the capsule after compaction
	Level3              // a new session: clear, then resubmit
	Level4              // native compaction at the lowered window
)

// LadderReason says why the ladder left level 0. LadderNone is level 0.
type LadderReason int

const (
	LadderNone LadderReason = iota
	// LadderCoreFailed: aval relay compact was missing, slow or failed.
	LadderCoreFailed
	// LadderNoReplace: the runtime lets the adapter steer, not replace.
	LadderNoReplace
	// LadderModNotLoaded: the relay mod does not run in this session.
	LadderModNotLoaded
	// LadderNewSession: the policy asks for a truly new session.
	LadderNewSession
	// LadderAllFailed: nothing above is available.
	LadderAllFailed
)

// String is the reason's name on the wire.
func (r LadderReason) String() string {
	switch r {
	case LadderCoreFailed:
		return "core_failed"
	case LadderNoReplace:
		return "no_replace"
	case LadderModNotLoaded:
		return "mod_not_loaded"
	case LadderNewSession:
		return "new_session"
	case LadderAllFailed:
		return "all_failed"
	}
	return "none"
}

// Health is what is known about the relay path of one session.
type Health struct {
	ModLoaded    bool // the relay mod runs in this session
	CoreAnswered bool // aval relay compact answered in time, usable
	NewSession   bool // the policy asks for a truly new session
}

// Step is the ladder's answer: the level and why it is not level 0.
type Step struct {
	Level Level
	Why   LadderReason
}

// Ladder returns the best level h and c allow. A new session the policy asks
// for wins when the runtime can restart; otherwise the mod replaces, then
// steers, then the classic hooks inject, and the lowered window is the floor.
func Ladder(h Health, c Capabilities) Step {
	switch {
	case h.NewSession && c.Session == SessionRestart:
		return Step{Level: Level3, Why: LadderNewSession}
	case h.ModLoaded && c.Compaction == CompactionReplace && h.CoreAnswered:
		return Step{Level: Level0}
	case h.ModLoaded && c.Compaction == CompactionReplace:
		return Step{Level: Level1, Why: LadderCoreFailed}
	case h.ModLoaded && c.Compaction == CompactionSteer:
		return Step{Level: Level1, Why: LadderNoReplace}
	case c.Inject:
		return Step{Level: Level2, Why: LadderModNotLoaded}
	}
	return Step{Level: Level4, Why: LadderAllFailed}
}
