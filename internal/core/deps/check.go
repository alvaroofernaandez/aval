package deps

import (
	"context"
	"errors"
	"sync"
	"time"
)

// DefaultTimeout is the budget of one probe.
const DefaultTimeout = 200 * time.Millisecond

// Check probes every entry at once, each under its own timeout, and returns
// one report per entry in the order given. A probe that fails, or answers
// only after its timeout, reports the tool missing.
func Check(ctx context.Context, es []Entry, p Prober, timeout time.Duration) []Report {
	rs := make([]Report, len(es))
	var wg sync.WaitGroup
	for i, e := range es {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			found, err := p.Probe(pctx, e.Probe)
			rs[i] = report(e, found, err)
		}()
	}
	wg.Wait()
	return rs
}

func report(e Entry, f Found, err error) Report {
	switch {
	case err == nil:
		return Report{Entry: e, Status: Present, Path: f.Path, Version: f.Version}
	case errors.Is(err, ErrSkipped):
		return Report{Entry: e, Status: Skipped}
	}
	return Report{Entry: e, Status: Missing}
}

// BlockingMissing is the reports of missing tools that block aval.
func BlockingMissing(rs []Report) []Report {
	var out []Report
	for _, r := range rs {
		if r.Status == Missing && r.IfMissing == Blocks {
			out = append(out, r)
		}
	}
	return out
}
