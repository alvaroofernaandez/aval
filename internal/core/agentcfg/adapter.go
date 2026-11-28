package agentcfg

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
)

// Support says how much of a capability a runtime can express.
type Support int

// Levels of support.
const (
	Unsupported Support = iota
	Partial
	Full
)

func (s Support) String() string {
	switch s {
	case Full:
		return "full"
	case Partial:
		return "partial"
	}
	return "unsupported"
}

// Capabilities declares the support of a runtime per capability. A capability
// that is missing from the map is Unsupported.
type Capabilities map[Capability]Support

// Adapter turns the model into the configuration of one agent runtime.
//
// Render is pure: it reads only cfg (and the skill directories cfg points at),
// never the live HOME, and returns the same bytes for the same cfg. Every
// element of cfg ends up in Rendered.Projected or in Rendered.Warnings.
//
// Diff compares a rendering with what is on disk under target, writing
// nothing. Apply makes target match the rendering and returns what changed;
// it never writes outside target, and a second Apply of the same rendering
// changes nothing. ApplyFiles and DiffFiles are the plain implementations an
// adapter whose files it owns outright can delegate to.
type Adapter interface {
	// Name is the runtime's stable id: "claude-code", "pi".
	Name() string
	// Capabilities is what the runtime supports; anything less than Full
	// produces warnings for the elements it loses.
	Capabilities() Capabilities
	Render(cfg Config) (Rendered, error)
	Diff(target string, r Rendered) ([]Change, error)
	Apply(target string, r Rendered, opt ApplyOptions) ([]Change, error)
}

// File is one rendered file, relative to the adapter's target directory.
type File struct {
	Path string // slash-separated, relative, clean
	Data []byte
	Mode fs.FileMode // 0 means 0o644
}

// Rendered is the output of Render.
type Rendered struct {
	Files []File
	// Projected lists the element ids the files express.
	Projected []string
	// Warnings report the elements the runtime cannot express, or expresses
	// only in part.
	Warnings []Warning
	// Linked lists the elements an `aval install` link delivers instead of
	// the files: the runtime reads them, the adapter does not write them.
	Linked []Link
}

// Link is an element delivered by an install link from the repo to the home.
type Link struct {
	ID         string
	Capability Capability
	Source     string // repo-relative, slash-separated
	Dest       string // home-relative, slash-separated
}

// Warning reports an element that was not projected, or only in part.
type Warning struct {
	ID         string
	Capability Capability
	Reason     string
}

func (w Warning) String() string { return w.ID + ": " + w.Reason }

// ChangeKind is what Diff or Apply found for one file.
type ChangeKind string

// Kinds of change.
const (
	Create ChangeKind = "create"
	Update ChangeKind = "update"
)

// Change is one file that differs from its rendering.
type Change struct {
	Path string // relative to the target, as in File.Path
	Kind ChangeKind
}

// ApplyOptions tunes Apply.
type ApplyOptions struct {
	DryRun bool
	// Recorder backs up what Apply overwrites; nil writes without a backup
	// (a fresh target, or a test).
	Recorder *backup.Recorder
}

// ErrOutsideTarget means a rendered path is absolute or climbs out of the
// target directory.
var ErrOutsideTarget = errors.New("agentcfg: path outside the target")

// SafePath checks that p is a clean, relative, slash-separated path that stays
// inside the target.
func SafePath(p string) error {
	if p == "" || path.IsAbs(p) || filepath.IsAbs(p) || strings.Contains(p, `\`) ||
		path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("%w: %q", ErrOutsideTarget, p)
	}
	return nil
}

// DiffFiles lists the files of r that are missing or different under target.
func DiffFiles(target string, r Rendered) ([]Change, error) {
	var out []Change
	for _, f := range r.Files {
		if err := SafePath(f.Path); err != nil {
			return nil, err
		}
		cur, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(f.Path)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			out = append(out, Change{Path: f.Path, Kind: Create})
		case err != nil:
			return nil, err
		case !bytes.Equal(cur, f.Data):
			out = append(out, Change{Path: f.Path, Kind: Update})
		}
	}
	return out, nil
}

// ApplyFiles writes every changed file of r under target atomically, backing
// up what it overwrites when opt.Recorder is set.
func ApplyFiles(target string, r Rendered, opt ApplyOptions) ([]Change, error) {
	changes, err := DiffFiles(target, r)
	if err != nil || opt.DryRun {
		return changes, err
	}
	data := map[string]File{}
	for _, f := range r.Files {
		data[f.Path] = f
	}
	for _, c := range changes {
		f := data[c.Path]
		dst := filepath.Join(target, filepath.FromSlash(f.Path))
		if err := writeOne(dst, f, opt.Recorder); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

func writeOne(dst string, f File, rec *backup.Recorder) error {
	mode := f.Mode
	if mode == 0 {
		mode = 0o644
	}
	if rec != nil {
		if _, err := rec.Snapshot(dst); err != nil {
			return fmt.Errorf("agentcfg: backup: %w", err)
		}
		if err := rec.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
	} else if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := backup.WriteFileAtomic(dst, f.Data, mode); err != nil {
		return fmt.Errorf("agentcfg: %w", err)
	}
	if rec != nil {
		return rec.Wrote(dst)
	}
	return nil
}

// Lose reports every element of capability c in cfg as not projected, with
// reason; adapters call it for what their runtime cannot express.
func Lose(cfg Config, c Capability, reason string) []Warning {
	var out []Warning
	for _, e := range cfg.Elements() {
		if e.Capability == c {
			out = append(out, Warning{ID: e.ID, Capability: c, Reason: reason})
		}
	}
	return out
}
