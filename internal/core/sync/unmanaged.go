package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
)

// UnmanagedDrift lists the top-level keys of the repo settings that sync does
// not manage and whose live value differs or is missing, writing nothing.
// Sync leaves these keys to the person, so this is information for --check,
// not something sync would change. A key the private overlay sets is the
// overlay's to own; live-only keys are never listed.
func UnmanagedDrift(opt Options) ([]string, error) {
	repo, err := RepoSettings(opt)
	if err != nil {
		return nil, fmt.Errorf("sync: repo settings: %w", err)
	}
	live := map[string]any{}
	if m, _, rerr := readJSONFile(filepath.Join(opt.Home, ".claude", "settings.json")); rerr == nil {
		live = m
	} else if !os.IsNotExist(unwrapPathErr(rerr)) {
		return nil, fmt.Errorf("sync: live settings: %w", rerr)
	}
	overlay, err := overlaySettings(opt)
	if err != nil {
		return nil, err
	}
	managed := append(append([]string{"permissions"}, replaceKeys...), mergeKeys...)
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
