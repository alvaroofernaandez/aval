package budget

import "sort"

// classify sets the percentiles and the verdict of m from its samples (ms):
// only a budgeted (synchronous Pre/PostToolUse) hook can be over, by p95 or by
// a timeout. It is pure, so the verdict is tested without a wall clock.
func classify(m Hook, samples []float64, timedOut bool) Hook {
	sort.Float64s(samples)
	m.TimedOut = timedOut
	m.P50Ms = round(percentile(samples, 0.50))
	m.P95Ms = round(percentile(samples, 0.95))
	m.Over = m.Budgeted && (m.TimedOut || m.P95Ms > m.BudgetMs)
	return m
}

func round(ms float64) float64 { return float64(int(ms*100+0.5)) / 100 }

// percentile is nearest-rank over an ascending sample.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p*float64(len(sorted))+0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
