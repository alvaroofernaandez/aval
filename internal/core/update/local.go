package update

import (
	"context"
	"strconv"
)

// LocalStatus reads, from refs that are already fetched, how many commits
// HEAD is behind its upstream and whether the working tree has uncommitted
// changes. It never touches the network and never writes: status runs with
// --no-optional-locks so it does not refresh the index.
//
// known is false when there is no upstream or git fails.
func LocalStatus(ctx context.Context, g Git) (behind int, known bool, dirty bool) {
	out, err := g.Run(ctx, "--no-optional-locks", "status", "--porcelain")
	if err != nil {
		return 0, false, false
	}
	dirty = out != ""

	count, err := g.Run(ctx, "rev-list", "--count", "HEAD..@{upstream}")
	if err != nil {
		return 0, false, dirty
	}
	n, err := strconv.Atoi(count)
	if err != nil {
		return 0, false, dirty
	}
	return n, true, dirty
}
