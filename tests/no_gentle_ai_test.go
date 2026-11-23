package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gentle-ai is no longer part of this harness (#4): its hooks, its markers in
// the instruction files and its formula are gone, and they must not come back.
// History (CHANGELOG.md, odd/tasks, docs/decks) keeps the name on purpose.

// gentleNeedle is built from fragments so this file does not match itself.
var gentleNeedle = "gentle" + "-" + "ai"

func TestNoGentleAILeftovers(t *testing.T) {
	exact := map[string]bool{"Brewfile": true, "README.md": true, ".agents/DESIGN.md": true}
	roots := []string{"config/", "internal/", "cmd/", "skills/_shared/"}
	var hits []string
	for _, f := range trackedAndUntracked(t) {
		in := exact[f]
		for _, r := range roots {
			in = in || strings.HasPrefix(f, r)
		}
		if !in {
			continue
		}
		b, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(f)))
		if err != nil {
			continue // listed but deleted in the working tree
		}
		if strings.Contains(strings.ToLower(string(b)), gentleNeedle) {
			hits = append(hits, f)
		}
	}
	eq(t, "files that still mention the retired harness", "", strings.Join(hits, " "))
}
