package relay

import (
	"testing"
	"time"
)

func TestEstimator_TranscriptDivisorIsProvisional(t *testing.T) {
	var e Estimator
	r := e.Transcript("opus", 60_000, 175_000, 1_000_000)
	if r.Tokens != 160_000 || r.Source != SourceTranscript || r.ErrPct != transcriptErrPct {
		t.Fatalf("got %+v, want 160000 tokens (floor + chars/1.75), transcript source, ErrPct %v", r, transcriptErrPct)
	}
}

func TestEstimator_UpperBoundTriggersEarly(t *testing.T) {
	var e Estimator
	r := e.Transcript("opus", 60_000, 210_000, 1_000_000) // 180k, upper bound 207k
	s := Snapshot{Reading: r, CacheAge: time.Minute, CacheTTL: time.Hour}
	if got := DefaultPolicy().Decide(s); got.Action != ActionRelay {
		t.Fatalf("estimate 180k with a 15%% bound: got action %d, want relay", got.Action)
	}
}

func TestEstimator_CalibratesOnCompaction(t *testing.T) {
	var e Estimator
	before := e.Transcript("opus", 60_000, 175_000, 1_000_000).Tokens
	e.Calibrate("opus", before, 192_000)
	after := e.Transcript("opus", 60_000, 175_000, 1_000_000).Tokens
	if after <= before || after > 192_000 {
		t.Fatalf("after one exact reading of 192000: estimate %d, want between %d and 192000", after, before)
	}
}

func TestEstimator_UnknownModelWidensBound(t *testing.T) {
	var e Estimator
	r := e.Transcript("", 60_000, 175_000, 0)
	if r.ErrPct != unknownErrPct || r.Window != smallestWindow {
		t.Fatalf("unknown model: got ErrPct %v window %d, want %v and %d", r.ErrPct, r.Window, unknownErrPct, smallestWindow)
	}
}
