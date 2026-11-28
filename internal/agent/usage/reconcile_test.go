package usage

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Reconciliation with ccusage. testdata/ccusage-20.1.2-daily.json is the output of
//
//	TZ=UTC CLAUDE_CONFIG_DIR=<repo>/internal/agent/usage/testdata/claude \
//	  npx -y ccusage@20.1.2 daily --json --offline
//
// run on the committed synthetic corpus (testdata/claude, no real data). It
// priced the sonnet-4 and opus-4-1 responses and flagged claude-mystery-9 as
// missing pricing, which is the n/a rule of this package. The test compares
// every number ccusage printed, per day and in total, with what Aggregate
// computes from the same files; a ccusage release changes the file only when
// someone re-runs the command and commits the output.
func TestTotalsMatchCcusageOnTheSharedCorpus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "ccusage-20.1.2-daily.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cc struct {
		Daily []struct {
			Period                                                                       string
			InputTokens, OutputTokens, CacheCreationTokens, CacheReadTokens, TotalTokens int64
			TotalCost                                                                    float64
		}
		Totals struct {
			InputTokens, OutputTokens, CacheCreationTokens, CacheReadTokens, TotalTokens int64
			TotalCost                                                                    float64
		}
	}
	if err := json.Unmarshal(raw, &cc); err != nil {
		t.Fatal(err)
	}
	rows := Aggregate(fixtureEntries(t), ByDay, time.UTC)
	if len(rows) != len(cc.Daily) {
		t.Fatalf("days = %d, ccusage has %d", len(rows), len(cc.Daily))
	}
	near := func(got Cost, want float64) bool { return math.Abs(got.Dollars()-want) < 1e-9 }
	for i, d := range cc.Daily {
		r := rows[i]
		want := Tokens{Input: d.InputTokens, Output: d.OutputTokens, CacheWrite: d.CacheCreationTokens, CacheRead: d.CacheReadTokens}
		if r.Key != d.Period || r.Tokens != want || r.Tokens.Total() != d.TotalTokens || !near(r.Cost, d.TotalCost) {
			t.Errorf("%s: aval %+v cost %v, ccusage %+v cost %v", d.Period, r.Tokens, r.Cost.Dollars(), want, d.TotalCost)
		}
	}
	s := Sum(rows)
	wantTotal := Tokens{Input: cc.Totals.InputTokens, Output: cc.Totals.OutputTokens, CacheWrite: cc.Totals.CacheCreationTokens, CacheRead: cc.Totals.CacheReadTokens}
	if s.Tokens != wantTotal || s.Tokens.Total() != cc.Totals.TotalTokens || !near(s.Cost, cc.Totals.TotalCost) {
		t.Errorf("total: aval %+v cost %v, ccusage %+v cost %v", s.Tokens, s.Cost.Dollars(), wantTotal, cc.Totals.TotalCost)
	}
}
