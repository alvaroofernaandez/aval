// Package contract is the reusable test suite every agentcfg.Adapter passes.
// An adapter's test calls Run with a config that exercises it:
//
//	func TestContract(t *testing.T) { contract.Run(t, myadapter.New(), cfg) }
//
// The suite checks the promises of the Adapter interface:
//
//   - deterministic: two renders of the same config are identical, byte for byte;
//   - projected or reported: every element of the config is in Projected or
//     in Warnings, and an element of a capability the adapter declares
//     Unsupported is reported, never projected;
//   - confined: every rendered path stays inside the target, and Apply writes
//     nothing outside it;
//   - idempotent: after Apply, Diff is empty and a second Apply changes nothing.
package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

// T is the part of *testing.T the suite uses, so the suite can test itself.
type T interface {
	Helper()
	Errorf(format string, args ...any)
	TempDir() string
}

// Run checks adapter a against cfg.
func Run(t T, a agentcfg.Adapter, cfg agentcfg.Config) {
	t.Helper()
	first, err := a.Render(cfg)
	if err != nil {
		t.Errorf("%s: Render: %v", a.Name(), err)
		return
	}
	second, err := a.Render(cfg)
	if err != nil {
		t.Errorf("%s: second Render: %v", a.Name(), err)
		return
	}
	deterministic(t, a.Name(), first, second)
	projectedOrReported(t, a, cfg, first)
	if !confinedPaths(t, a.Name(), first) {
		return
	}
	applies(t, a, first)
}

func deterministic(t T, name string, a, b agentcfg.Rendered) {
	t.Helper()
	if len(a.Files) != len(b.Files) {
		t.Errorf("%s: not deterministic: %d files, then %d", name, len(a.Files), len(b.Files))
		return
	}
	for i := range a.Files {
		if a.Files[i].Path != b.Files[i].Path || !bytes.Equal(a.Files[i].Data, b.Files[i].Data) || a.Files[i].Mode != b.Files[i].Mode {
			t.Errorf("%s: not deterministic: file %d differs between two renders (%s)", name, i, a.Files[i].Path)
		}
	}
	if strings.Join(a.Projected, "\n") != strings.Join(b.Projected, "\n") {
		t.Errorf("%s: not deterministic: Projected differs between two renders", name)
	}
	if len(a.Warnings) != len(b.Warnings) {
		t.Errorf("%s: not deterministic: %d warnings, then %d", name, len(a.Warnings), len(b.Warnings))
	}
}

func projectedOrReported(t T, a agentcfg.Adapter, cfg agentcfg.Config, r agentcfg.Rendered) {
	t.Helper()
	projected := map[string]bool{}
	for _, id := range r.Projected {
		projected[id] = true
	}
	reported := map[string]bool{}
	for _, w := range r.Warnings {
		reported[w.ID] = true
	}
	caps := a.Capabilities()
	for _, e := range cfg.Elements() {
		switch {
		case caps[e.Capability] == agentcfg.Unsupported && !reported[e.ID]:
			t.Errorf("%s: %s is %s, unsupported, but no warning reports it", a.Name(), e.ID, e.Capability)
		case caps[e.Capability] == agentcfg.Unsupported && projected[e.ID]:
			t.Errorf("%s: %s is %s, unsupported, yet it is listed as projected", a.Name(), e.ID, e.Capability)
		case !projected[e.ID] && !reported[e.ID]:
			t.Errorf("%s: %s was dropped in silence: neither projected nor reported", a.Name(), e.ID)
		}
	}
}

func confinedPaths(t T, name string, r agentcfg.Rendered) bool {
	t.Helper()
	ok := true
	seen := map[string]bool{}
	for _, f := range r.Files {
		if err := agentcfg.SafePath(f.Path); err != nil {
			t.Errorf("%s: %v", name, err)
			ok = false
		}
		if seen[f.Path] {
			t.Errorf("%s: %s is rendered twice", name, f.Path)
			ok = false
		}
		seen[f.Path] = true
	}
	return ok
}

func applies(t T, a agentcfg.Adapter, r agentcfg.Rendered) {
	t.Helper()
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	sentinel := filepath.Join(parent, "outside", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o755); err != nil {
		t.Errorf("setup: %v", err)
		return
	}
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Errorf("setup: %v", err)
		return
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Errorf("setup: %v", err)
		return
	}
	outside := treeHash(parent, target)

	before, err := a.Diff(target, r)
	if err != nil {
		t.Errorf("%s: Diff on an empty target: %v", a.Name(), err)
		return
	}
	if len(before) != len(r.Files) {
		t.Errorf("%s: Diff on an empty target lists %d changes, want one per file (%d)", a.Name(), len(before), len(r.Files))
	}
	if _, err := a.Apply(target, r, agentcfg.ApplyOptions{}); err != nil {
		t.Errorf("%s: Apply: %v", a.Name(), err)
		return
	}
	if treeHash(parent, target) != outside {
		t.Errorf("%s: Apply wrote outside its target", a.Name())
	}
	if d, err := a.Diff(target, r); err != nil || len(d) != 0 {
		t.Errorf("%s: Diff after Apply = %v, %v; want no changes", a.Name(), d, err)
	}
	if again, err := a.Apply(target, r, agentcfg.ApplyOptions{}); err != nil || len(again) != 0 {
		t.Errorf("%s: a second Apply changed %v (%v); want nothing", a.Name(), again, err)
	}
}

// treeHash hashes every path and file under root, except the skip subtree.
func treeHash(root, skip string) string {
	var lines []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == skip {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, p)
		b, _ := os.ReadFile(p)
		sum := sha256.Sum256(b)
		lines = append(lines, rel+" "+hex.EncodeToString(sum[:]))
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
