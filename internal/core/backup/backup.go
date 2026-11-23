// Package backup records everything aval moves or overwrites into a run
// directory under ~/.aval-backup, so any change can be undone.
//
// A run is ~/.aval-backup/<YYYYmmdd-HHMMSS>[-n]/ holding manifest.json (one
// Entry per path touched) plus the saved copies. Runs written before this
// package existed have no manifest: they still list, but cannot be restored
// automatically and are never pruned.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	KindFile    = "file"
	KindDir     = "dir"
	KindSymlink = "symlink"

	// Retain is how many unpinned runs survive a prune.
	Retain = 5

	manifestName  = "manifest.json"
	pinnedMarker  = "PINNED"
	pendingMarker = "PENDING"
	originMarker  = "ORIGIN"
	failedMarker  = "FAILED"
	displacedDir  = ".displaced"
	idLayout      = "20060102-150405"
	legacyLayout  = "20060102150405"
)

// Entry is one path a run touched.
type Entry struct {
	Path       string `json:"path"`
	Existed    bool   `json:"existed"`
	Kind       string `json:"kind,omitempty"`
	LinkTarget string `json:"link_target,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Mode       uint32 `json:"mode,omitempty"`
	// Saved is the copy's path relative to the run directory.
	Saved string `json:"saved,omitempty"`
	// Wrote* describe what aval put at Path, so a restore can tell its own
	// output from foreign content.
	WroteKind   string `json:"wrote_kind,omitempty"`
	WroteLink   string `json:"wrote_link,omitempty"`
	WroteSHA256 string `json:"wrote_sha256,omitempty"`
	// CreatedDir marks a directory aval had to create on the way.
	CreatedDir bool `json:"created_dir,omitempty"`
}

// Run is one backup run on disk.
type Run struct {
	ID      string
	Dir     string
	Time    time.Time
	Entries int
	Pinned  bool
	Failed  bool
	Pending bool
	Legacy  bool
	Reason  string
	// Origin names the command that made the run (install, sync, uninstall,
	// ...). Empty for runs made before origins were recorded.
	Origin string
}

// ActionKind is the typed outcome of a restore step, next to the free text in
// Action.Action.
type ActionKind string

const (
	ActRestore   ActionKind = "restore"   // the saved state is put back
	ActRemove    ActionKind = "remove"    // aval's own output is removed
	ActUnchanged ActionKind = "unchanged" // already as saved
	ActKept      ActionKind = "kept"      // left alone: not aval's, or not empty
	ActGone      ActionKind = "gone"      // nothing there to undo
)

// Action is what a restore did to one path.
type Action struct {
	Path   string
	Action string
	Kind   ActionKind
}

// RestoreOptions tunes a restore.
type RestoreOptions struct {
	// Repo is the aval repository: symlinks pointing into it are aval's.
	Repo   string
	DryRun bool
}

// Store is ~/.aval-backup.
type Store struct {
	Root string
	Home string
	Now  func() time.Time
}

// NewStore returns the store under home.
func NewStore(home string) *Store {
	return &Store{Root: filepath.Join(home, ".aval-backup"), Home: home}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// --- listing ----------------------------------------------------------------

func parseID(id string, fallback time.Time) time.Time {
	if len(id) >= len(idLayout) {
		if t, err := time.ParseInLocation(idLayout, id[:len(idLayout)], time.Local); err == nil {
			return t
		}
	}
	if len(id) >= len(legacyLayout) {
		if t, err := time.ParseInLocation(legacyLayout, id[:len(legacyLayout)], time.Local); err == nil {
			return t
		}
	}
	return fallback
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func loadRun(dir string) Run {
	r := Run{ID: filepath.Base(dir), Dir: dir}
	var mod time.Time
	if fi, err := os.Stat(dir); err == nil {
		mod = fi.ModTime()
	}
	r.Time = parseID(r.ID, mod)
	if entries, err := readManifest(dir); err == nil {
		r.Entries = len(entries)
	} else {
		r.Legacy = true
	}
	r.Pinned = exists(filepath.Join(dir, pinnedMarker))
	if b, err := os.ReadFile(filepath.Join(dir, originMarker)); err == nil {
		r.Origin = strings.TrimSpace(string(b))
	}
	r.Pending = exists(filepath.Join(dir, pendingMarker))
	if b, err := os.ReadFile(filepath.Join(dir, failedMarker)); err == nil {
		r.Failed = true
		r.Reason = strings.TrimSpace(string(b))
	}
	return r
}

// LoadRun describes the run stored in dir.
func (s *Store) LoadRun(dir string) Run { return loadRun(dir) }

// MarkFailed keeps run, notes why it failed and clears its pending marker.
func (s *Store) MarkFailed(run Run, reason string) error {
	_ = os.Remove(filepath.Join(run.Dir, pendingMarker))
	return os.WriteFile(filepath.Join(run.Dir, failedMarker), []byte(reason+"\n"), 0o644)
}

// List returns every run, newest first. A missing store is an empty list.
func (s *Store) List() ([]Run, error) {
	des, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("backup: %w", err)
	}
	var runs []Run
	for _, d := range des {
		if d.IsDir() {
			runs = append(runs, loadRun(filepath.Join(s.Root, d.Name())))
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		if !runs[i].Time.Equal(runs[j].Time) {
			return runs[i].Time.After(runs[j].Time)
		}
		return runs[i].ID > runs[j].ID
	})
	return runs, nil
}

// Resolve finds a run by "latest", exact ID or unambiguous ID prefix.
func (s *Store) Resolve(ref string) (Run, error) {
	runs, err := s.List()
	if err != nil {
		return Run{}, err
	}
	if ref == "latest" || ref == "" {
		for _, r := range runs {
			if !r.Legacy {
				return r, nil
			}
		}
		return Run{}, errors.New("backup: no restorable backup run found")
	}
	var match []Run
	for _, r := range runs {
		if r.ID == ref {
			return r, nil
		}
		if strings.HasPrefix(r.ID, ref) {
			match = append(match, r)
		}
	}
	switch len(match) {
	case 0:
		return Run{}, fmt.Errorf("backup: no run %q (see: aval restore --list)", ref)
	case 1:
		return match[0], nil
	}
	return Run{}, fmt.Errorf("backup: %q matches %d runs, give the full id", ref, len(match))
}

func readManifest(dir string) ([]Entry, error) {
	b, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("backup: %s: %w", filepath.Join(dir, manifestName), err)
	}
	return entries, nil
}

// Load returns a run's entries.
func (s *Store) Load(run Run) ([]Entry, error) {
	entries, err := readManifest(run.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("backup: run %s has no manifest (made by an older aval); restore it by hand from %s", run.ID, run.Dir)
		}
		return nil, err
	}
	return entries, nil
}

// Pin protects a run from retention (or lifts the protection).
func (s *Store) Pin(id string, on bool) error {
	run, err := s.Resolve(id)
	if err != nil {
		return err
	}
	marker := filepath.Join(run.Dir, pinnedMarker)
	if on {
		return os.WriteFile(marker, []byte("pinned\n"), 0o644)
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Pending lists runs whose apply never finished.
func (s *Store) Pending() ([]Run, error) {
	runs, err := s.List()
	var out []Run
	for _, r := range runs {
		if r.Pending {
			out = append(out, r)
		}
	}
	return out, err
}

// Prune keeps the newest Retain unpinned runs. Legacy dirs, pinned runs and
// pending runs are never removed.
func (s *Store) Prune() error {
	runs, err := s.List()
	if err != nil {
		return err
	}
	kept := 0
	for _, r := range runs {
		if r.Legacy || r.Pinned || r.Pending {
			continue
		}
		kept++
		if kept > Retain {
			if err := os.RemoveAll(r.Dir); err != nil {
				return fmt.Errorf("backup: prune %s: %w", r.ID, err)
			}
		}
	}
	return nil
}

// --- recording --------------------------------------------------------------

// Recorder collects the entries of one run.
type Recorder struct {
	store   *Store
	dir     string
	fixed   bool
	opened  bool
	entries []Entry
	idx     map[string]int
	origin  string
}

// Begin starts a run. Nothing touches the disk until the first path is
// recorded, so a run that changes nothing leaves no trace.
func (s *Store) Begin() *Recorder {
	return &Recorder{store: s, idx: map[string]int{}}
}

// RecorderAt records into an exact directory instead of a fresh run ID.
func RecorderAt(dir, home string) *Recorder {
	return &Recorder{
		store: &Store{Root: filepath.Dir(dir), Home: home},
		dir:   dir, fixed: true, idx: map[string]int{},
	}
}

func (s *Store) nextDir() string {
	base := s.now().Format(idLayout)
	d := filepath.Join(s.Root, base)
	for n := 2; exists(d); n++ {
		d = filepath.Join(s.Root, fmt.Sprintf("%s-%d", base, n))
	}
	return d
}

// Dir is the run directory (not necessarily created yet).
func (r *Recorder) Dir() string {
	if r.dir == "" {
		r.dir = r.store.nextDir()
	}
	return r.dir
}

// Open creates the run directory.
func (r *Recorder) Open() error {
	if r.opened {
		return nil
	}
	dir := r.Dir()
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if r.fixed {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
	} else {
		for {
			err := os.Mkdir(r.dir, 0o755)
			if err == nil {
				break
			}
			if !os.IsExist(err) {
				return fmt.Errorf("backup: %w", err)
			}
			r.dir = r.store.nextDir()
		}
	}
	r.opened = true
	if err := r.writeOrigin(); err != nil {
		return err
	}
	return r.flush()
}

// SetOrigin names the command making this run. It is written beside the
// manifest once the run directory exists, so a run that records nothing still
// leaves no trace.
func (r *Recorder) SetOrigin(origin string) {
	r.origin = origin
	if r.opened {
		_ = r.writeOrigin()
	}
}

func (r *Recorder) writeOrigin() error {
	if r.origin == "" {
		return nil
	}
	return os.WriteFile(filepath.Join(r.dir, originMarker), []byte(r.origin+"\n"), 0o644)
}

func (r *Recorder) flush() error {
	if !r.opened {
		return nil
	}
	b, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return err
	}
	if r.entries == nil {
		b = []byte("[]")
	}
	return WriteFileAtomic(filepath.Join(r.dir, manifestName), append(b, '\n'), 0o644)
}

// Entries returns what was recorded so far.
func (r *Recorder) Entries() []Entry { return append([]Entry(nil), r.entries...) }

func (r *Recorder) savedName(path string) string {
	if rel, err := filepath.Rel(r.store.Home, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	clean := strings.NewReplacer(":", "_", `\`, "/").Replace(path)
	return filepath.Join("_abs", strings.TrimLeft(clean, "/"))
}

// Displace records path and moves it into the run directory, leaving the
// destination free. A path that does not exist is recorded as such.
func (r *Recorder) Displace(path string) (string, error) {
	return r.capture(path, r.savedName(path), true)
}

// Snapshot records path and copies it into the run directory, leaving the
// original in place (for files about to be overwritten).
func (r *Recorder) Snapshot(path string) (string, error) {
	return r.capture(path, r.savedName(path), false)
}

// SnapshotAs is Snapshot with an explicit name inside the run directory.
func (r *Recorder) SnapshotAs(path, name string) (string, error) {
	return r.capture(path, name, false)
}

func (r *Recorder) add(e Entry) int {
	r.entries = append(r.entries, e)
	r.idx[e.Path] = len(r.entries) - 1
	return len(r.entries) - 1
}

func (r *Recorder) capture(path, name string, move bool) (string, error) {
	if err := r.Open(); err != nil {
		return "", err
	}
	fi, err := os.Lstat(path)
	if _, seen := r.idx[path]; seen {
		// First capture wins: it holds the pre-aval state. A second object at
		// the same path is aval's own output, but move it aside rather than
		// destroy anything.
		if err == nil && move {
			return r.moveAside(path, name)
		}
		return "", nil
	}
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("backup: %w", err)
		}
		r.add(Entry{Path: path})
		return "", r.flush()
	}

	e := Entry{Path: path, Existed: true, Mode: uint32(fi.Mode().Perm())}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		e.Kind = KindSymlink
		e.LinkTarget, _ = os.Readlink(path)
	case fi.IsDir():
		e.Kind = KindDir
		e.SHA256, err = hashTree(path)
	default:
		e.Kind = KindFile
		e.SHA256, err = hashFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("backup: hash %s: %w", path, err)
	}
	e.Saved = name
	saved := filepath.Join(r.dir, name)
	if err := os.MkdirAll(filepath.Dir(saved), 0o755); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	if move {
		err = moveOrCopy(path, saved)
	} else {
		err = CopyPath(path, saved)
	}
	if err != nil {
		return "", fmt.Errorf("backup: save %s: %w", path, err)
	}
	r.add(e)
	return saved, r.flush()
}

func (r *Recorder) moveAside(path, name string) (string, error) {
	dst := filepath.Join(r.dir, displacedDir, name)
	dst = uniquePath(dst)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return dst, moveOrCopy(path, dst)
}

func uniquePath(p string) string {
	if !exists(p) {
		return p
	}
	for n := 1; ; n++ {
		q := fmt.Sprintf("%s.%d", p, n)
		if !exists(q) {
			return q
		}
	}
}

// moveOrCopy renames, falling back to copy-then-remove across filesystems so
// the original always survives a failure.
func moveOrCopy(src, dst string) error {
	if err := renameReplace(src, dst); err == nil {
		return nil
	}
	if err := CopyPath(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// MkdirAll creates path and records every directory it had to create, so an
// uninstall can remove them again if they end up empty.
func (r *Recorder) MkdirAll(path string, perm os.FileMode) error {
	var missing []string
	for p := path; !exists(p); p = filepath.Dir(p) {
		missing = append(missing, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	if len(missing) > 0 {
		if err := r.Open(); err != nil {
			return err
		}
		for i := len(missing) - 1; i >= 0; i-- {
			if _, seen := r.idx[missing[i]]; !seen {
				r.add(Entry{Path: missing[i], Kind: KindDir, CreatedDir: true})
			}
		}
		if err := r.flush(); err != nil {
			return err
		}
	}
	return os.MkdirAll(path, perm)
}

// Wrote notes what aval left at path. Call it after Displace/Snapshot and the
// write, so a restore can recognise aval's own output.
func (r *Recorder) Wrote(path string) error {
	i, ok := r.idx[path]
	if !ok {
		return nil
	}
	e := &r.entries[i]
	fi, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	e.WroteKind, e.WroteLink, e.WroteSHA256 = "", "", ""
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		e.WroteKind = KindSymlink
		e.WroteLink, _ = os.Readlink(path)
	case fi.IsDir():
		e.WroteKind = KindDir
		e.WroteSHA256, err = hashTree(path)
	default:
		e.WroteKind = KindFile
		e.WroteSHA256, err = hashFile(path)
	}
	if err != nil {
		return err
	}
	return r.flush()
}

// SetPending marks the run as in progress (crash detection).
func (r *Recorder) SetPending() error {
	if err := r.Open(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.dir, pendingMarker), []byte("pending\n"), 0o644)
}

// Fail keeps the run, marks it failed with the reason and clears pending.
// A run that recorded nothing is dropped.
func (r *Recorder) Fail(reason string) error {
	if !r.opened {
		return nil
	}
	if len(r.entries) == 0 {
		return os.RemoveAll(r.dir)
	}
	_ = os.Remove(filepath.Join(r.dir, pendingMarker))
	return os.WriteFile(filepath.Join(r.dir, failedMarker), []byte(reason+"\n"), 0o644)
}

func entriesDigest(entries []Entry) string {
	norm := append([]Entry(nil), entries...)
	for i := range norm {
		norm[i].Saved = ""
	}
	sort.Slice(norm, func(i, j int) bool { return norm[i].Path < norm[j].Path })
	b, _ := json.Marshal(norm)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Commit finishes a successful run. An empty run is dropped; a run identical
// to the previous one is dropped and the previous one returned (skipped=true);
// otherwise retention is applied.
func (r *Recorder) Commit() (Run, bool, error) {
	if !r.opened {
		return Run{}, false, nil
	}
	_ = os.Remove(filepath.Join(r.dir, pendingMarker))
	if len(r.entries) == 0 {
		return Run{}, false, os.RemoveAll(r.dir)
	}
	runs, err := r.store.List()
	if err != nil {
		return Run{}, false, err
	}
	digest := entriesDigest(r.entries)
	for _, prev := range runs {
		if prev.Dir == r.dir || prev.Legacy {
			continue
		}
		if pe, err := readManifest(prev.Dir); err == nil && entriesDigest(pe) == digest {
			return prev, true, os.RemoveAll(r.dir)
		}
		break // only the newest other run counts as "previous"
	}
	if err := r.store.Prune(); err != nil {
		return Run{}, false, err
	}
	return loadRun(r.dir), false, nil
}

// --- hashing and copying ------------------------------------------------------

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashTree(root string) (string, error) {
	h := sha256.New()
	var walk func(rel string) error
	walk = func(rel string) error {
		p := filepath.Join(root, rel)
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			t, _ := os.Readlink(p)
			fmt.Fprintf(h, "L %s %s\n", filepath.ToSlash(rel), t)
		case fi.IsDir():
			fmt.Fprintf(h, "D %s %o\n", filepath.ToSlash(rel), fi.Mode().Perm())
			des, err := os.ReadDir(p)
			if err != nil {
				return err
			}
			for _, d := range des {
				if err := walk(filepath.Join(rel, d.Name())); err != nil {
					return err
				}
			}
		default:
			sum, err := hashFile(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "F %s %o %s\n", filepath.ToSlash(rel), fi.Mode().Perm(), sum)
		}
		return nil
	}
	if err := walk("."); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CopyPath copies a file, symlink or directory tree, preserving permission
// bits (the tmux scripts are only useful if the executable bit survives).
func CopyPath(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		_ = os.Remove(dst)
		return os.Symlink(target, dst)
	}
	if !fi.IsDir() {
		return copyFile(src, dst, fi.Mode().Perm())
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	des, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range des {
		if err := CopyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	// Chmod last so a read-only directory can still be filled, and so the
	// umask does not shave the mode.
	return os.Chmod(dst, fi.Mode().Perm())
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}

// WriteFileAtomic writes data to a temp file beside path and renames it over.
// A symlinked path is written through, and an existing file keeps its mode.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		perm = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".aval-tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return cleanup(err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		return cleanup(err)
	}
	if err := renameReplace(name, path); err != nil {
		return cleanup(err)
	}
	return nil
}

// --- restoring ----------------------------------------------------------------

// Restore puts every path of a run back as it was.
func (s *Store) Restore(run Run, opt RestoreOptions) ([]Action, error) {
	entries, err := s.Load(run)
	if err != nil {
		return nil, err
	}
	var out []Action
	var created []Entry
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.CreatedDir {
			created = append(created, e)
			continue
		}
		act, err := s.restoreEntry(run, e, opt)
		out = append(out, act)
		if err != nil {
			return out, fmt.Errorf("backup: restore %s: %w", e.Path, err)
		}
	}
	sort.Slice(created, func(i, j int) bool { return len(created[i].Path) > len(created[j].Path) })
	for _, e := range created {
		act := Action{Path: e.Path}
		switch {
		case !exists(e.Path):
			act.Action, act.Kind = "already gone", ActGone
		case opt.DryRun:
			act.Action, act.Kind = "would remove if empty", ActRemove
		default:
			if err := os.Remove(e.Path); err != nil {
				act.Action, act.Kind = "kept (not empty)", ActKept
			} else {
				act.Action, act.Kind = "removed (empty dir)", ActRemove
			}
		}
		out = append(out, act)
	}
	return out, nil
}

// Preview is the named dry run of Restore: it changes nothing on disk and
// returns the typed actions a restore would take.
func (s *Store) Preview(run Run, opt RestoreOptions) ([]Action, error) {
	opt.DryRun = true
	return s.Restore(run, opt)
}

// RestorePath restores one path of a run.
func (s *Store) RestorePath(run Run, path string, opt RestoreOptions) (Action, error) {
	entries, err := s.Load(run)
	if err != nil {
		return Action{}, err
	}
	for _, e := range entries {
		if e.Path == path {
			return s.restoreEntry(run, e, opt)
		}
	}
	return Action{Path: path}, fmt.Errorf("backup: run %s has no entry for %s", run.ID, path)
}

// FindPrevious looks, newest run first, for what stood at path before aval
// first took it over. Runs whose saved state is itself a link into repo
// (aval's own work) are skipped, and so is the run named by exclude.
func (s *Store) FindPrevious(path, repo, exclude string) (Run, Entry, bool, error) {
	runs, err := s.List()
	if err != nil {
		return Run{}, Entry{}, false, err
	}
	for _, r := range runs {
		if r.Legacy || r.Dir == exclude {
			continue
		}
		entries, err := readManifest(r.Dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Path != path || e.CreatedDir {
				continue
			}
			if e.Existed && e.Kind == KindSymlink && IntoRepo(path, e.LinkTarget, repo) {
				continue
			}
			return r, e, true, nil
		}
	}
	return Run{}, Entry{}, false, nil
}

// IntoRepo reports whether the symlink at link, pointing at target, resolves
// under repo.
func IntoRepo(link, target, repo string) bool {
	if repo == "" || target == "" {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	target = filepath.Clean(target)
	roots := []string{filepath.Clean(repo)}
	if real, err := filepath.EvalSymlinks(repo); err == nil {
		roots = append(roots, real)
	}
	targets := []string{target}
	if real, err := filepath.EvalSymlinks(target); err == nil {
		targets = append(targets, real)
	}
	for _, root := range roots {
		for _, t := range targets {
			if t == root || strings.HasPrefix(t, root+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

func (s *Store) restoreEntry(run Run, e Entry, opt RestoreOptions) (Action, error) {
	act := Action{Path: e.Path}
	cur, curErr := os.Lstat(e.Path)
	present := curErr == nil

	if !e.Existed {
		return s.undoCreation(e, cur, present, opt, act)
	}

	if present && matchesSaved(e, cur) {
		act.Action, act.Kind = "unchanged", ActUnchanged
		return act, nil
	}
	if opt.DryRun {
		act.Action, act.Kind = "would restore", ActRestore
		return act, nil
	}
	if err := os.MkdirAll(filepath.Dir(e.Path), 0o755); err != nil {
		return act, err
	}
	if present {
		if err := s.clearForRestore(run, e, cur); err != nil {
			return act, err
		}
	}
	saved := filepath.Join(run.Dir, e.Saved)
	if e.Kind != KindSymlink && !exists(saved) {
		return act, fmt.Errorf("saved copy %s is missing", saved)
	}
	var err error
	switch e.Kind {
	case KindSymlink:
		err = placeSymlink(e.Path, e.LinkTarget)
	case KindDir:
		err = placeDir(saved, e.Path, os.FileMode(e.Mode))
	default:
		err = placeFile(saved, e.Path, os.FileMode(e.Mode))
	}
	if err != nil {
		return act, err
	}
	act.Action, act.Kind = "restored", ActRestore
	return act, nil
}

func matchesSaved(e Entry, cur os.FileInfo) bool {
	switch e.Kind {
	case KindSymlink:
		if cur.Mode()&os.ModeSymlink == 0 {
			return false
		}
		t, _ := os.Readlink(e.Path)
		return t == e.LinkTarget
	case KindDir:
		if !cur.IsDir() || cur.Mode()&os.ModeSymlink != 0 {
			return false
		}
		h, err := hashTree(e.Path)
		return err == nil && h == e.SHA256 && cur.Mode().Perm() == os.FileMode(e.Mode)
	default:
		if !cur.Mode().IsRegular() {
			return false
		}
		h, err := hashFile(e.Path)
		return err == nil && h == e.SHA256 && cur.Mode().Perm() == os.FileMode(e.Mode)
	}
}

// clearForRestore gets the way clear for the saved state. Anything that is not
// provably aval's own output is kept under the run's .displaced directory.
func (s *Store) clearForRestore(run Run, e Entry, cur os.FileInfo) error {
	keep := func() error {
		rel := e.Saved
		if rel == "" {
			rel = filepath.Base(e.Path)
		}
		dst := uniquePath(filepath.Join(run.Dir, displacedDir, rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return moveOrCopy(e.Path, dst)
	}
	switch {
	case cur.Mode()&os.ModeSymlink != 0:
		return os.Remove(e.Path)
	case cur.IsDir():
		if e.WroteKind == KindDir {
			if h, err := hashTree(e.Path); err == nil && h == e.WroteSHA256 {
				return os.RemoveAll(e.Path)
			}
		}
		return keep()
	default:
		if e.WroteKind == KindFile {
			if h, err := hashFile(e.Path); err == nil && h == e.WroteSHA256 {
				return os.Remove(e.Path)
			}
		}
		return keep()
	}
}

func (s *Store) undoCreation(e Entry, cur os.FileInfo, present bool, opt RestoreOptions, act Action) (Action, error) {
	if !present {
		act.Action, act.Kind = "already gone", ActGone
		return act, nil
	}
	remove, why := false, "kept (not created by aval)"
	switch {
	case cur.Mode()&os.ModeSymlink != 0:
		t, _ := os.Readlink(e.Path)
		if (e.WroteKind == KindSymlink && t == e.WroteLink) || IntoRepo(e.Path, t, opt.Repo) {
			remove = true
		}
	case cur.IsDir():
		if e.WroteKind == KindDir {
			if h, err := hashTree(e.Path); err == nil && h == e.WroteSHA256 {
				remove = true
			}
		}
	default:
		if e.WroteKind == KindFile {
			if h, err := hashFile(e.Path); err == nil && h == e.WroteSHA256 {
				remove = true
			}
		}
	}
	if !remove {
		act.Action, act.Kind = why, ActKept
		return act, nil
	}
	if opt.DryRun {
		act.Action, act.Kind = "would remove", ActRemove
		return act, nil
	}
	if err := os.RemoveAll(e.Path); err != nil {
		return act, err
	}
	act.Action, act.Kind = "removed", ActRemove
	return act, nil
}

func tempName(dir string) string {
	return filepath.Join(dir, fmt.Sprintf(".aval-restore-%d-%d", os.Getpid(), time.Now().UnixNano()))
}

func placeSymlink(path, target string) error {
	tmp := tempName(filepath.Dir(path))
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := renameReplace(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func placeFile(saved, path string, mode os.FileMode) error {
	tmp := tempName(filepath.Dir(path))
	if err := copyFile(saved, tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := renameReplace(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func placeDir(saved, path string, mode os.FileMode) error {
	tmp := tempName(filepath.Dir(path))
	if err := CopyPath(saved, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := renameReplace(tmp, path); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	return nil
}
