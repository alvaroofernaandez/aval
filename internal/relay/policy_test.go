package relay

import (
	"testing"
	"time"
)

func warm(tokens, window int) Snapshot {
	return Snapshot{
		Reading:  Reading{Tokens: tokens, Window: window, FloorTokens: 58_000},
		CacheAge: time.Minute,
		CacheTTL: time.Hour,
	}
}

func TestDecide_HardTokensCapsLargeWindow(t *testing.T) {
	got := DefaultPolicy().Decide(warm(200_000, 1_000_000))
	if got.Action != ActionRelay {
		t.Fatalf("1M window at 200k tokens: got action %d, want relay (%d)", got.Action, ActionRelay)
	}
}

func cold(tokens, window int) Snapshot {
	s := warm(tokens, window)
	s.CacheAge = 2 * time.Hour
	return s
}

func TestDecide(t *testing.T) {
	unknownCache := warm(150_000, 1_000_000)
	unknownCache.CacheAge, unknownCache.CacheTTL = 3*time.Hour, 0

	bigFloor := warm(200_000, 1_000_000)
	bigFloor.Reading.FloorTokens = 190_000

	estimated := warm(180_000, 1_000_000)
	estimated.Reading.ErrPct = 0.15

	cases := []struct {
		name string
		snap Snapshot
		want Action
	}{
		{"BelowSoft_None", warm(100_000, 1_000_000), ActionNone},
		{"AtSoft_Precompute", warm(160_000, 1_000_000), ActionPrecompute},
		{"SmallWindowUsesPctCap", warm(121_000, 200_000), ActionRelay},
		{"NilWindow_TokensOnly", warm(200_000, 0), ActionRelay},
		{"ColdCacheAboveColdMin_Relay", cold(100_000, 1_000_000), ActionRelay},
		{"ColdCacheBelowColdMin_None", cold(99_000, 1_000_000), ActionNone},
		{"WarmCacheBelowHardTokens_None", warm(150_000, 1_000_000), ActionNone},
		{"NetBenefitBelowFloor_None", bigFloor, ActionNone},
		{"UnknownCache_TreatedAsWarm", unknownCache, ActionNone},
		{"UsesUpperBound", estimated, ActionRelay},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DefaultPolicy().Decide(c.snap); got.Action != c.want {
				t.Fatalf("got action %d, want %d", got.Action, c.want)
			}
		})
	}
}
