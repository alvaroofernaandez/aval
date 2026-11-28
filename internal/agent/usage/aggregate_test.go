package usage

import (
	"testing"
	"time"
)

func fixtureEntries(t *testing.T) []Entry {
	t.Helper()
	c, err := Read(fixtureRoots())
	if err != nil {
		t.Fatal(err)
	}
	return c.Entries
}

// Totals below are the hand-computed sums of testdata/claude (see reconcile_test.go
// for the same corpus checked against ccusage).
func TestAggregateByDayMatchesTheHandComputedFixture(t *testing.T) {
	rows := Aggregate(fixtureEntries(t), ByDay, time.UTC)
	want := []Row{
		{Key: "2026-11-20", Tokens: Tokens{Input: 3500, Output: 1750, CacheWrite: 3000, CacheRead: 27000}, Messages: 3, Cost: 9_810_000},
		{Key: "2026-11-21", Tokens: Tokens{Input: 3500, Output: 1750, CacheWrite: 500, CacheRead: 20000}, Messages: 3, Cost: 4_042_500,
			Unpriced: 1, UnpricedModels: []string{"claude-mystery-9"}},
	}
	assertRows(t, rows, want)
}

func TestAggregateByProjectSessionAndConsumer(t *testing.T) {
	es := fixtureEntries(t)
	for _, c := range []struct {
		by   By
		keys []string
	}{
		{ByProject, []string{"/work/alpha", "/work/beta"}},
		{BySession, []string{"a1", "a2", "b1"}},
		{ByConsumer, []string{"headless", "interactive"}},
	} {
		rows := Aggregate(es, c.by, time.UTC)
		var got []string
		var tokens Tokens
		for _, r := range rows {
			got = append(got, r.Key)
			tokens = tokens.Add(r.Tokens)
		}
		if len(got) != len(c.keys) {
			t.Fatalf("%s: keys = %v, want the set %v", c.by, got, c.keys)
		}
		if tokens != (Tokens{Input: 7000, Output: 3500, CacheWrite: 3500, CacheRead: 47000}) {
			t.Errorf("%s: tokens = %+v, want the corpus total", c.by, tokens)
		}
		for _, k := range c.keys {
			found := false
			for _, g := range got {
				found = found || g == k
			}
			if !found {
				t.Errorf("%s: missing key %q in %v", c.by, k, got)
			}
		}
	}
}

func TestSumKeepsUnpricedVisible(t *testing.T) {
	s := Sum(Aggregate(fixtureEntries(t), ByDay, time.UTC))
	if s.Cost != 13_852_500 || s.Messages != 6 || s.Unpriced != 1 {
		t.Errorf("sum = cost %d, messages %d, unpriced %d; want 13852500, 6, 1", s.Cost, s.Messages, s.Unpriced)
	}
}

func assertRows(t *testing.T, got, want []Row) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %+v, want %d", got, len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Key != w.Key || g.Tokens != w.Tokens || g.Messages != w.Messages || g.Cost != w.Cost ||
			g.Unpriced != w.Unpriced || len(g.UnpricedModels) != len(w.UnpricedModels) {
			t.Errorf("row %d = %+v, want %+v", i, g, w)
		}
	}
}
