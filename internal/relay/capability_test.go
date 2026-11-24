package relay

import "testing"

func TestTier(t *testing.T) {
	claude := Capabilities{Compaction: CompactionReplace, Inject: true, Session: SessionRestart}
	cases := []struct {
		name string
		caps Capabilities
		mode RunMode
		want Tier
	}{
		{"ClaudeNativeUI_T0", claude, ModeNativeUI, T0},
		{"SteerAndInject_T1", Capabilities{Compaction: CompactionSteer, Inject: true}, ModeNativeUI, T1},
		{"SteerWithoutInject_NotT1", Capabilities{Compaction: CompactionSteer, MCP: true, Rules: true}, ModeNativeUI, T3},
		{"SessionRestart_T2", Capabilities{Compaction: CompactionObserve, Session: SessionRestart}, ModeNativeUI, T2},
		{"HeadlessPutsAvalInThePath_T2", Capabilities{Compaction: CompactionObserve}, ModeHeadless, T2},
		{"ACPHosted_T2", Capabilities{}, ModeACPHosted, T2},
		{"MCPAndRules_T3", Capabilities{MCP: true, Rules: true}, ModeNativeUI, T3},
		{"MCPWithoutRules_T4", Capabilities{MCP: true}, ModeNativeUI, T4},
		{"Nothing_T4", Capabilities{}, ModeNativeUI, T4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TierOf(c.caps, c.mode); got != c.want {
				t.Fatalf("got T%d, want T%d", got, c.want)
			}
		})
	}
}

func TestDegrade(t *testing.T) {
	claude := Capabilities{Compaction: CompactionReplace, Inject: true, Session: SessionRestart, MCP: true, Rules: true}
	cases := []struct {
		name   string
		caps   Capabilities
		failed []Probe
		want   Tier
	}{
		{"ReplaceProbeFailed_T1", claude, []Probe{ProbeReplace}, T1},
		{"SteerProbeFailed_T2", claude, []Probe{ProbeReplace, ProbeSteer}, T2},
		{"SessionProbeFailed_T3", claude, []Probe{ProbeReplace, ProbeSteer, ProbeSession}, T3},
		{"MCPProbeFailed_T4", claude, []Probe{ProbeReplace, ProbeSteer, ProbeSession, ProbeMCP}, T4},
		{"InjectProbeFailed_SkipsT1", claude, []Probe{ProbeReplace, ProbeInject}, T2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := c.caps
			for _, p := range c.failed {
				caps = Degrade(caps, p)
			}
			if got := TierOf(caps, ModeNativeUI); got != c.want {
				t.Fatalf("got T%d, want T%d", got, c.want)
			}
		})
	}
}
