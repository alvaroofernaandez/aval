package doctor

import (
	"context"

	"github.com/alvaroofernaandez/aval/internal/core/deps"
)

// checkDeps probes the external tool inventory. A missing blocking tool fails
// and the message names only those, so it reads the same on every machine; a
// missing degrading one warns. Skipped probes (an app bundle off macOS) count
// for neither.
func checkDeps(opt Options) Result {
	m, err := deps.Embedded()
	if err != nil {
		return result("deps", Fail, false, "the inventory does not parse: %v", err)
	}
	timeout := opt.DepsTimeout
	if timeout == 0 {
		timeout = deps.DefaultTimeout
	}
	reports := deps.Check(context.Background(), m.Entries, opt.DepsProber, timeout)
	var blocking, missing []string
	present, skipped := 0, 0
	for _, r := range reports {
		switch r.Status {
		case deps.Present:
			present++
		case deps.Skipped:
			skipped++
		default:
			missing = append(missing, r.Name)
			if r.IfMissing == deps.Blocks {
				blocking = append(blocking, r.Name)
			}
		}
	}
	switch {
	case len(blocking) > 0:
		return withDetails(result("deps", Fail, false, "blocking tools missing: %s (aval deps lists all)", list(blocking)), blocking)
	case len(missing) > 0:
		return withDetails(result("deps", Warn, false, "%d missing: %s (aval deps lists all)", len(missing), list(missing)), missing)
	case skipped > 0:
		return result("deps", OK, false, "%d tools present, %d not probed on this platform", present, skipped)
	}
	return result("deps", OK, false, "%d tools present", present)
}
