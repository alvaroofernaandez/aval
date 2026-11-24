package relay

import "testing"

// claudeCode is the capability vector the spike #63 results give Claude Code.
var claudeCode = Capabilities{Compaction: CompactionReplace, Inject: true, Session: SessionRestart}

var healthy = Health{ModLoaded: true, CoreAnswered: true}

func TestLadder_Healthy_Level0(t *testing.T) {
	if got := Ladder(healthy, claudeCode); got != (Step{Level: Level0}) {
		t.Fatalf("healthy: got %+v, want level 0 with no reason", got)
	}
}

func TestLadder_BinarySlow_Level1(t *testing.T) {
	h := healthy
	h.CoreAnswered = false
	if got := Ladder(h, claudeCode); got != (Step{Level: Level1, Why: LadderCoreFailed}) {
		t.Fatalf("core failed: got %+v, want level 1 core_failed", got)
	}
}

func TestLadder_NoReplace_Level1(t *testing.T) {
	c := Degrade(claudeCode, ProbeReplace)
	if got := Ladder(healthy, c); got != (Step{Level: Level1, Why: LadderNoReplace}) {
		t.Fatalf("no replace: got %+v, want level 1 no_replace", got)
	}
}

func TestLadder_ModNotLoaded_Level2(t *testing.T) {
	h := Health{ModLoaded: false}
	if got := Ladder(h, claudeCode); got != (Step{Level: Level2, Why: LadderModNotLoaded}) {
		t.Fatalf("mod not loaded: got %+v, want level 2 mod_not_loaded", got)
	}
}

func TestLadder_PolicyWantsNewSession_Level3(t *testing.T) {
	h := healthy
	h.NewSession = true
	if got := Ladder(h, claudeCode); got != (Step{Level: Level3, Why: LadderNewSession}) {
		t.Fatalf("new session: got %+v, want level 3 new_session", got)
	}
}

func TestLadder_NewSessionWithoutRestart_NotLevel3(t *testing.T) {
	h := healthy
	h.NewSession = true
	if got := Ladder(h, Degrade(claudeCode, ProbeSession)); got.Level == Level3 {
		t.Fatalf("no restart capability: got level 3")
	}
}

func TestLadder_AllFailed_Level4(t *testing.T) {
	c := Degrade(Degrade(claudeCode, ProbeSteer), ProbeInject)
	if got := Ladder(Health{}, c); got != (Step{Level: Level4, Why: LadderAllFailed}) {
		t.Fatalf("all failed: got %+v, want level 4 all_failed", got)
	}
}

// TestLadder_NeverSilent: every level but 0 says why, with a named reason.
func TestLadder_NeverSilent(t *testing.T) {
	caps := []Capabilities{
		claudeCode,
		Degrade(claudeCode, ProbeReplace),
		Degrade(Degrade(claudeCode, ProbeSteer), ProbeInject),
		Degrade(claudeCode, ProbeSession),
		{},
	}
	for _, c := range caps {
		for _, h := range []Health{
			{}, {ModLoaded: true}, healthy, {CoreAnswered: true},
			{NewSession: true}, {ModLoaded: true, NewSession: true}, {ModLoaded: true, CoreAnswered: true, NewSession: true},
		} {
			got := Ladder(h, c)
			if got.Level != Level0 && (got.Why == LadderNone || got.Why.String() == "none") {
				t.Errorf("caps %+v health %+v: level %d with no reason", c, h, got.Level)
			}
		}
	}
}
