package update

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/manifest"
)

// Preview is what an update would change, computed without changing it.
type Preview struct {
	Status
	// Stat is `git diff --stat` of the managed paths between HEAD and upstream.
	Stat string
	// Added and Removed are manifest entries ("source -> dest") that the
	// upstream manifest has and the local one lacks, and the reverse.
	Added, Removed []string
}

// DryRun fetches, then reports what Pull would bring in. It only reads: the
// one write is the fetch itself, which moves remote-tracking refs and nothing
// the user can see in their working tree. A dirty tree is refused with the
// same message Pull uses, so a dry run predicts the real run's refusals.
func DryRun(ctx context.Context, g Git, paths []string) (Preview, error) {
	var p Preview
	s, err := Check(ctx, g)
	p.Status = s
	if err != nil {
		return p, err
	}
	if s.DirtyTree {
		return p, fmt.Errorf("update: the repository has uncommitted changes; commit or stash them first")
	}
	if s.Behind == 0 {
		return p, nil
	}

	args := []string{"diff", "--stat", "HEAD..@{upstream}"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	if p.Stat, err = g.Run(ctx, args...); err != nil {
		return p, err
	}

	before, err := entriesAt(ctx, g, "HEAD")
	if err != nil {
		return p, err
	}
	after, err := entriesAt(ctx, g, "@{upstream}")
	if err != nil {
		return p, err
	}
	p.Added = difference(after, before)
	p.Removed = difference(before, after)
	return p, nil
}

// entriesAt parses the manifest as it exists at a revision.
func entriesAt(ctx context.Context, g Git, rev string) (map[string]bool, error) {
	text, err := g.Run(ctx, "show", rev+":install.manifest")
	if err != nil {
		return nil, err
	}
	m, err := manifest.Parse(strings.NewReader(text + "\n"))
	if err != nil {
		return nil, fmt.Errorf("update: manifest at %s: %w", rev, err)
	}
	out := map[string]bool{}
	for _, grp := range m.Groups {
		for _, e := range grp.Entries {
			out[e.Source+" -> "+e.Dest] = true
		}
		for _, d := range grp.FanoutDests {
			out[grp.FanoutSource+" -> "+d] = true
		}
	}
	return out, nil
}

// difference lists keys of a that b lacks, sorted.
func difference(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
