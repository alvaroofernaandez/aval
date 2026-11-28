package sync

import (
	"fmt"
	"reflect"
	"slices"
	"sort"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/claude"
)

// UnmanagedDrift lists the top-level keys of the repo settings that sync does
// not manage and whose live value differs or is missing, writing nothing.
// Sync leaves these keys to the person, so this is information for --check,
// not something sync would change. A key the private overlay sets is the
// overlay's to own; live-only keys are never listed.
func UnmanagedDrift(opt Options) ([]string, error) {
	repo, err := RepoSettings(opt)
	if err != nil {
		return nil, err
	}
	a := opt.adapter()
	live, _, err := a.Live(opt.claudeDir())
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	overlay, err := a.Overlay()
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	managed := append(append([]string{"permissions"}, claude.ReplaceKeys...), claude.MergeKeys...)
	var drift []string
	for k, v := range repo {
		if _, owned := overlay[k]; owned || slices.Contains(managed, k) {
			continue
		}
		if old, ok := live[k]; !ok || !reflect.DeepEqual(old, v) {
			drift = append(drift, k)
		}
	}
	sort.Strings(drift)
	return drift, nil
}
