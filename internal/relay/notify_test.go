package relay

import "testing"

func TestNotifyText_T0_TokensBeforeAfterFloor(t *testing.T) {
	got := NotifyText(Outcome{Tier: T0, Before: 212_400, After: 61_300, Floor: 58_000, ColdCache: true})
	if want := "Relayed 212k → 61k (floor 58k) · cold cache"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNotifyText_OneGoldenPerTier(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Outcome
		want string
	}{
		{"T0_Warm", Outcome{Tier: T0, Before: 212_400, After: 61_300, Floor: 58_000}, "Relayed 212k → 61k (floor 58k)"},
		{"T0_NotYetMeasured", Outcome{Tier: T0, Before: 212_400, Floor: 58_000}, "Relayed at 212k (floor 58k)"},
		{"T0_NoFloor", Outcome{Tier: T0, Before: 212_400, After: 61_300}, "Relayed 212k → 61k"},
		{"T1", Outcome{Tier: T1, Before: 212_400, After: 61_300, Floor: 58_000}, "Compacted with steering 212k → 61k (floor 58k)"},
		{"T2", Outcome{Tier: T2, Before: 212_400}, "Continued in a new session at 212k"},
		{"T3_NamesTheAction", Outcome{Tier: T3, Before: 212_400}, "Context relay ready at 212k · open a new chat and run /continue"},
		{"T4_NamesTheAction", Outcome{Tier: T4, Before: 212_400}, "Context at 212k needs a relay · start a new session"},
		{"SmallAndLarge", Outcome{Tier: T0, Before: 1_250_000, After: 900}, "Relayed 1.3M → 900"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NotifyText(tc.o); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTierString(t *testing.T) {
	for tier, want := range map[Tier]string{T0: "T0", T2: "T2", T4: "T4", Tier(9): "T?"} {
		if got := tier.String(); got != want {
			t.Errorf("Tier(%d) = %q, want %q", tier, got, want)
		}
	}
}

func TestErrPctOf(t *testing.T) {
	for src, want := range map[string]float64{"exact": 0.02, "transcript": 0.15, "bogus": 0.50} {
		if got := ErrPctOf(src); got != want {
			t.Errorf("ErrPctOf(%q) = %v, want %v", src, got, want)
		}
	}
}
