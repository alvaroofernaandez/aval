package update

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/manifest"
)

// latestFile names the cache file holding the version the last check saw,
// next to the last-update-check stamp.
const latestFile = "latest-version"

func latestPath() (string, bool) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(cache, "aval", latestFile), true
}

// checkForAndStore is checkFor that remembers the version it heard about, so
// Cached can answer later without the network. A failed check stores nothing,
// and so keeps the previous answer; a failed write is ignored like the stamp's.
func checkForAndStore(ctx context.Context, base, current string) Notice {
	n := checkFor(ctx, base, current)
	if n.Latest == "" {
		return n
	}
	if p, ok := latestPath(); ok {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			_ = os.WriteFile(p, []byte(n.Latest), 0o644)
		}
	}
	return n
}

// Cached is the notice the last CheckLatest produced, read from the cache. It
// never touches the network and never writes, so it is safe to call when
// opening a screen. With no cache it returns an empty Notice; Latest is only
// set when it is strictly newer than current, the running build.
func Cached(current string) Notice { return CachedWith(current, true) }

// CachedWith is Cached with the update_check setting applied: enabled=false
// (update_check = off) and AVAL_NO_UPDATE_CHECK both give an empty Notice,
// the same opt-outs ShouldCheckWith honours.
func CachedWith(current string, enabled bool) Notice {
	n := Notice{Current: current}
	if !enabled || os.Getenv("AVAL_NO_UPDATE_CHECK") != "" {
		return n
	}
	p, ok := latestPath()
	if !ok {
		return n
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return n
	}
	seen := Notice{Current: current, Latest: strings.TrimSpace(string(b))}
	if seen.String() == "" {
		return n
	}
	seen.Command = updateCommand()
	return seen
}

// ManagedPaths lists the repository paths an update can change: the manifest
// itself, then every entry source and fan-out source once, in manifest order.
// A nil manifest (unreadable or absent) leaves just the manifest file.
func ManagedPaths(m *manifest.Manifest) []string {
	const self = "install.manifest"
	paths := []string{self}
	if m == nil {
		return paths
	}
	seen := map[string]bool{self: true}
	for _, grp := range m.Groups {
		srcs := []string{grp.FanoutSource}
		for _, e := range grp.Entries {
			srcs = append(srcs, e.Source)
		}
		for _, s := range srcs {
			if s != "" && !seen[s] {
				seen[s] = true
				paths = append(paths, s)
			}
		}
	}
	return paths
}
