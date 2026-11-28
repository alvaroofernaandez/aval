package generation

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

// ErrTampered means a generation's files no longer hash to its contentHash.
var ErrTampered = errors.New("generation: changed since it was built")

// Verify re-hashes generation id from its files and its record's modes, and
// compares the result with its contentHash.
func Verify(home, id string) error {
	g, err := Read(home, id)
	if err != nil {
		return err
	}
	var outs []rendering
	for _, a := range g.Adapters {
		r, err := storedRendering(Dir(home, id, a.Name), a.Modes)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrTampered, id, err)
		}
		outs = append(outs, rendering{name: a.Name, r: r})
	}
	if "sha256:"+contentHash(outs) != g.ContentHash {
		return fmt.Errorf("%w: %s no longer matches %s", ErrTampered, id, g.ContentHash)
	}
	return nil
}

// storedRendering reads an adapter's tree back as the rendering it stores.
func storedRendering(dir string, modes map[string]string) (agentcfg.Rendered, error) {
	var r agentcfg.Rendered
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return r, nil // an adapter that rendered nothing
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		f := agentcfg.File{Path: filepath.ToSlash(rel), Data: b}
		if m, ok := modes[f.Path]; ok {
			n, err := strconv.ParseUint(m, 8, 32)
			if err != nil {
				return err
			}
			f.Mode = fs.FileMode(n)
		}
		r.Files = append(r.Files, f)
		return nil
	})
	return r, err
}
