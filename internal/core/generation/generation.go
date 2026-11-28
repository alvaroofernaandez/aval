// Package generation builds immutable, content-addressed generations: a
// read-only snapshot of everything aval renders for every adapter, under
// ~/.aval/generations/<id>/<adapter>/, with its record in generation.json.
//
// The id is gen-<16 hex>, the start of the sha256 of the rendered tree
// (adapter, path, mode and bytes of every file), so the same inputs always
// give the same id and a rebuild is a no-op. A build is written to a
// temporary directory and renamed into place, so an interrupted build leaves
// no generation behind. Nothing secret is ever rendered (the adapters refuse
// API keys, and the Pi login is linked at activation, never stored here).
package generation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

// RecordFile is the record of a generation, at its root.
const RecordFile = "generation.json"

// ErrUnknown means no generation has that id.
var ErrUnknown = errors.New("generation: unknown generation")

// Options configures Build.
type Options struct {
	Home     string
	Config   agentcfg.Config
	Adapters []agentcfg.Adapter
	// Source records where the config came from (the aval repo commit).
	Source string
	Now    func() time.Time
}

// Generation is the record of one build, generation.json.
type Generation struct {
	ID          string        `json:"id"`
	ContentHash string        `json:"contentHash"`
	Built       time.Time     `json:"built"`
	Source      string        `json:"source,omitempty"`
	Adapters    []AdapterInfo `json:"adapters"`
}

// AdapterInfo is what one adapter contributed. Warnings are what it could
// not express, so a generation shows what each runtime lost.
type AdapterInfo struct {
	Name     string   `json:"name"`
	Files    int      `json:"files"`
	Warnings []string `json:"warnings,omitempty"`
}

// Change is one path that differs between two generations.
type Change struct {
	Path string `json:"path"` // <adapter>/<file>
	Kind string `json:"kind"` // added, removed, changed
}

// Root is the generation store of home.
func Root(home string) string { return filepath.Join(home, ".aval", "generations") }

type rendering struct {
	name string
	r    agentcfg.Rendered
}

// Build renders every adapter and stores the result. A build whose content
// already exists is a no-op that returns the stored record and created=false.
func Build(opt Options) (Generation, bool, error) {
	outs, err := renderAll(opt)
	if err != nil {
		return Generation{}, false, err
	}
	sum := contentHash(outs)
	g := Generation{ID: "gen-" + sum[:16], ContentHash: "sha256:" + sum, Built: now(opt).UTC(), Source: opt.Source}
	for _, o := range outs {
		info := AdapterInfo{Name: o.name, Files: len(o.r.Files)}
		for _, w := range o.r.Warnings {
			info.Warnings = append(info.Warnings, w.String())
		}
		g.Adapters = append(g.Adapters, info)
	}
	root := Root(opt.Home)
	if old, err := Read(opt.Home, g.ID); err == nil {
		return old, false, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Generation{}, false, fmt.Errorf("generation: %w", err)
	}
	if err := store(root, g, outs); err != nil {
		return Generation{}, false, err
	}
	return g, true, nil
}

func now(opt Options) time.Time {
	if opt.Now != nil {
		return opt.Now()
	}
	return time.Now()
}

// renderAll renders each adapter, sorted by name; any error fails the build.
func renderAll(opt Options) ([]rendering, error) {
	var outs []rendering
	for _, a := range opt.Adapters {
		r, err := a.Render(opt.Config)
		if err != nil {
			return nil, fmt.Errorf("generation: %s: %w", a.Name(), err)
		}
		for _, f := range r.Files {
			if err := agentcfg.SafePath(f.Path); err != nil {
				return nil, fmt.Errorf("generation: %s: %w", a.Name(), err)
			}
		}
		outs = append(outs, rendering{name: a.Name(), r: r})
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].name < outs[j].name })
	return outs, nil
}

func mode(f agentcfg.File) fs.FileMode {
	if f.Mode == 0 {
		return 0o644
	}
	return f.Mode
}

func contentHash(outs []rendering) string {
	h := sha256.New()
	for _, o := range outs {
		files := slicesSorted(o.r.Files)
		for _, f := range files {
			fmt.Fprintf(h, "%s\x00%s\x00%o\x00%d\x00", o.name, f.Path, mode(f), len(f.Data))
			h.Write(f.Data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func slicesSorted(files []agentcfg.File) []agentcfg.File {
	out := append([]agentcfg.File(nil), files...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// store writes the generation into a temporary directory, makes it read-only
// and renames it into place.
func store(root string, g Generation, outs []rendering) error {
	tmp, err := os.MkdirTemp(root, ".tmp-")
	if err != nil {
		return fmt.Errorf("generation: %w", err)
	}
	fail := func(err error) error {
		_ = removeTree(tmp)
		return fmt.Errorf("generation: %w", err)
	}
	for _, o := range outs {
		for _, f := range o.r.Files {
			p := filepath.Join(tmp, o.name, filepath.FromSlash(f.Path))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return fail(err)
			}
			if err := os.WriteFile(p, f.Data, mode(f)); err != nil {
				return fail(err)
			}
		}
	}
	rec, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, RecordFile), append(rec, '\n'), 0o644); err != nil {
		return fail(err)
	}
	if err := readOnly(tmp); err != nil {
		return fail(err)
	}
	if err := os.Rename(tmp, filepath.Join(root, g.ID)); err != nil {
		if _, rerr := os.Stat(filepath.Join(root, g.ID, RecordFile)); rerr == nil {
			_ = removeTree(tmp) // a concurrent build of the same content won
			return nil
		}
		return fail(err)
	}
	return nil
}

// readOnly drops every write bit under dir, files first, then directories.
func readOnly(dir string) error {
	var dirs []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chmod(p, info.Mode().Perm()&^0o222)
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i], 0o555); err != nil {
			return err
		}
	}
	return nil
}

// removeTree deletes a read-only tree: write bits back on, then RemoveAll.
func removeTree(dir string) error {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
	return os.RemoveAll(dir)
}

// Read returns the record of generation id.
func Read(home, id string) (Generation, error) {
	var g Generation
	if !validID(id) {
		return g, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	b, err := os.ReadFile(filepath.Join(Root(home), id, RecordFile))
	if errors.Is(err, fs.ErrNotExist) {
		return g, fmt.Errorf("%w: %s", ErrUnknown, id)
	}
	if err != nil {
		return g, fmt.Errorf("generation: %w", err)
	}
	if err := json.Unmarshal(b, &g); err != nil {
		return g, fmt.Errorf("generation: %s: %w", id, err)
	}
	return g, nil
}

func validID(id string) bool {
	hexPart, ok := strings.CutPrefix(id, "gen-")
	if !ok || len(hexPart) != 16 {
		return false
	}
	_, err := hex.DecodeString(hexPart)
	return err == nil
}

// List returns the stored generations, newest first. Unfinished builds
// (.tmp-*) and anything that is not a generation are skipped.
func List(home string) ([]Generation, error) {
	ents, err := os.ReadDir(Root(home))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("generation: %w", err)
	}
	var out []Generation
	for _, e := range ents {
		if g, err := Read(home, e.Name()); err == nil {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Built.Equal(out[j].Built) {
			return out[i].Built.After(out[j].Built)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Diff lists what changes from generation a to generation b, sorted by path.
func Diff(home, a, b string) ([]Change, error) {
	fa, err := files(home, a)
	if err != nil {
		return nil, err
	}
	fb, err := files(home, b)
	if err != nil {
		return nil, err
	}
	var out []Change
	for p, da := range fa {
		db, ok := fb[p]
		switch {
		case !ok:
			out = append(out, Change{Path: p, Kind: "removed"})
		case !bytes.Equal(da, db):
			out = append(out, Change{Path: p, Kind: "changed"})
		}
	}
	for p := range fb {
		if _, ok := fa[p]; !ok {
			out = append(out, Change{Path: p, Kind: "added"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// files reads every rendered file of generation id, by <adapter>/<path>.
func files(home, id string) (map[string][]byte, error) {
	if _, err := Read(home, id); err != nil {
		return nil, err
	}
	dir := filepath.Join(Root(home), id)
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == RecordFile {
			return nil
		}
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = b
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("generation: %w", err)
	}
	return out, nil
}
