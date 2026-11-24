package relay

import "testing"

func chainOf(links ...Link) Chain {
	var c Chain
	for _, l := range links {
		c = c.Record(l)
	}
	return c
}

func TestBrake(t *testing.T) {
	b := Brakes{MaxChain: 2, MinTurnsBetween: 3, CostCapUSD: 1}
	one := chainOf(Link{SessionID: "s1", Fingerprint: "a", Turn: 10, CostUSD: 0.2})
	cases := []struct {
		name  string
		chain Chain
		next  Attempt
		want  BrakeReason
	}{
		{"SameFingerprintTwice", one, Attempt{Fingerprint: "a", Turn: 20}, BrakeNoProgress},
		{"ThrashWithinMinTurns", one, Attempt{Fingerprint: "b", Turn: 12}, BrakeThrash},
		{"MaxChain", chainOf(Link{Fingerprint: "a", Turn: 1}, Link{Fingerprint: "b", Turn: 10}), Attempt{Fingerprint: "c", Turn: 20}, BrakeMaxChain},
		{"CostCap", chainOf(Link{Fingerprint: "a", Turn: 1, CostUSD: 1.5}), Attempt{Fingerprint: "b", Turn: 20}, BrakeCostCap},
		{"Progress_Go", one, Attempt{Fingerprint: "b", Turn: 20}, BrakeNone},
		{"EmptyChain_Go", Chain{}, Attempt{Fingerprint: "a", Turn: 1}, BrakeNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := b.Check(c.chain, c.next); got != c.want {
				t.Fatalf("got brake %d, want %d", got, c.want)
			}
		})
	}
}

func TestBrake_ZeroLimitsAreOff(t *testing.T) {
	c := chainOf(Link{Fingerprint: "a", Turn: 1, CostUSD: 99}, Link{Fingerprint: "b", Turn: 2})
	if got := (Brakes{}).Check(c, Attempt{Fingerprint: "c", Turn: 3}); got != BrakeNone {
		t.Fatalf("got brake %d with every limit off, want none", got)
	}
}

func TestChain_EpochIncrements(t *testing.T) {
	c := chainOf(Link{SessionID: "s1"}, Link{SessionID: "s1"})
	if got := c.Epoch(); got != 2 {
		t.Fatalf("epoch after two relays: got %d, want 2", got)
	}
}

func TestChain_SessionIDMapAfterClear(t *testing.T) {
	c := chainOf(Link{SessionID: "s1"}, Link{SessionID: "s2"})
	for _, id := range []string{"s1", "s2"} {
		if !c.Owns(id) {
			t.Errorf("chain does not own session %q after a clear", id)
		}
	}
	if c.Owns("other") {
		t.Error("chain owns a session it never ran in")
	}
}
