package generation

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Retain is how many unpinned generations survive a Prune, like backup.
const Retain = 5

// pinsDir holds one empty file per pinned generation, beside the store and
// not inside it: a generation is immutable.
const pinsDir = "pins"

// Pin protects generation id from Prune, or lifts the protection.
func Pin(home, id string, on bool) error {
	if _, err := Read(home, id); err != nil {
		return err
	}
	marker := filepath.Join(Root(home), pinsDir, id)
	if !on {
		if err := os.Remove(marker); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("generation: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return fmt.Errorf("generation: %w", err)
	}
	if err := os.WriteFile(marker, []byte("pinned\n"), 0o644); err != nil {
		return fmt.Errorf("generation: %w", err)
	}
	return nil
}

// Pinned reports whether id is pinned.
func Pinned(home, id string) bool {
	_, err := os.Stat(filepath.Join(Root(home), pinsDir, id))
	return err == nil
}

// Prune removes the oldest unpinned generations beyond Retain and returns
// the ids it removed. The active generation, the one active before it and
// the ones a consumer is pinned to are never removed, and do not count.
func Prune(home string) ([]string, error) {
	gs, err := List(home)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	if id, err := Current(home); err == nil && id != "" {
		keep[id] = true
	}
	if id, err := Previous(home); err == nil {
		keep[id] = true
	}
	for _, id := range consumerPins(home) {
		keep[id] = true
	}
	var removed []string
	kept := 0
	for _, g := range gs { // newest first
		if keep[g.ID] || Pinned(home, g.ID) {
			continue
		}
		kept++
		if kept <= Retain {
			continue
		}
		if err := os.RemoveAll(filepath.Join(Root(home), g.ID)); err != nil {
			return removed, fmt.Errorf("generation: prune %s: %w", g.ID, err)
		}
		removed = append(removed, g.ID)
	}
	return removed, nil
}
