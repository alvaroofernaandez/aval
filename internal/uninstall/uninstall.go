// Package uninstall undoes an install: it removes the symlinks aval created
// and puts back what they displaced, and touches nothing else.
//
// "Aval created" is decided by evidence, not by the manifest: a destination is
// only removed when it is a symlink that resolves into this repository. A file
// the user put there, or a link they pointed elsewhere, is left alone.
package uninstall

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/backup"
	"github.com/alvaroofernaandez/aval/internal/manifest"
	"github.com/alvaroofernaandez/aval/internal/plan"
	avalsync "github.com/alvaroofernaandez/aval/internal/sync"
)

// Options configures a run.
type Options struct {
	Repo, Home, GOOS string
	DryRun           bool
	Now              func() time.Time
}

// Item is what happened to one path.
type Item struct {
	Path   string
	Action string
}

// Run uninstalls. Everything it changes is recorded in a backup run first, so
// `aval restore latest` undoes the uninstall itself.
func Run(opt Options) ([]Item, error) {
	if opt.GOOS == "" {
		opt.GOOS = runtime.GOOS
	}
	m, err := manifest.Load(filepath.Join(opt.Repo, "install.manifest"))
	if err != nil {
		return nil, err
	}
	p, err := plan.Build(m, plan.Options{Repo: opt.Repo, Home: opt.Home, GOOS: opt.GOOS})
	if err != nil {
		return nil, err
	}

	store := backup.NewStore(opt.Home)
	store.Now = opt.Now
	rec := store.Begin()
	ropt := backup.RestoreOptions{Repo: opt.Repo}

	var items []Item
	for _, g := range p.Groups {
		for _, a := range g.Actions {
			it, err := removeLink(store, rec, opt, ropt, a.Dest)
			if err != nil {
				return items, err
			}
			if it != nil {
				items = append(items, *it)
			}
		}
	}

	rules, err := removeRules(rec, opt)
	items = append(items, rules...)
	if err != nil {
		return items, err
	}

	if !opt.DryRun {
		dirs, err := removeCreatedDirs(store, opt.Home)
		items = append(items, dirs...)
		if err != nil {
			return items, err
		}
		if _, _, err := rec.Commit(); err != nil {
			return items, err
		}
	}
	return items, nil
}

// removeLink handles one manifest destination.
func removeLink(store *backup.Store, rec *backup.Recorder, opt Options, ropt backup.RestoreOptions, dest string) (*Item, error) {
	fi, err := os.Lstat(dest)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return nil, nil // missing, or a real file/dir: not ours to remove
	}
	target, err := os.Readlink(dest)
	if err != nil || !backup.IntoRepo(dest, target, opt.Repo) {
		return nil, nil // someone else's link
	}

	prev, entry, found, err := store.FindPrevious(dest, opt.Repo, rec.Dir())
	if err != nil {
		return nil, err
	}
	restore := found && entry.Existed

	if opt.DryRun {
		action := "would remove link"
		if restore {
			action = "would remove link and restore the previous " + entry.Kind + " (backup " + prev.ID + ")"
		}
		return &Item{Path: dest, Action: action}, nil
	}

	if _, err := rec.Snapshot(dest); err != nil {
		return nil, fmt.Errorf("uninstall: backup %s: %w", dest, err)
	}
	if err := os.Remove(dest); err != nil {
		return nil, fmt.Errorf("uninstall: remove %s: %w", dest, err)
	}
	action := "removed link"
	if restore {
		if _, err := store.RestorePath(prev, dest, ropt); err != nil {
			return nil, fmt.Errorf("uninstall: restore %s: %w", dest, err)
		}
		if err := rec.Wrote(dest); err != nil {
			return nil, err
		}
		action = "removed link, restored the previous " + entry.Kind
	}
	return &Item{Path: dest, Action: action}, nil
}

// removeRules cuts the aval:agent-rules block out of the Codex and Gemini
// instruction files, leaving every other byte alone.
func removeRules(rec *backup.Recorder, opt Options) ([]Item, error) {
	var items []Item
	for _, path := range []string{
		filepath.Join(opt.Home, ".codex", "AGENTS.md"),
		filepath.Join(opt.Home, ".gemini", "GEMINI.md"),
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		next, changed := avalsync.RemoveRules(string(b))
		if !changed {
			continue
		}
		if opt.DryRun {
			items = append(items, Item{Path: path, Action: "would remove the aval:agent-rules block"})
			continue
		}
		if _, err := rec.Snapshot(path); err != nil {
			return items, fmt.Errorf("uninstall: backup %s: %w", path, err)
		}
		if strings.TrimSpace(next) == "" {
			// Nothing but aval's block was in it; the snapshot keeps the old file.
			err = os.Remove(path)
		} else {
			err = backup.WriteFileAtomic(path, []byte(next), 0o644)
		}
		if err != nil {
			return items, fmt.Errorf("uninstall: %s: %w", path, err)
		}
		if err := rec.Wrote(path); err != nil {
			return items, err
		}
		items = append(items, Item{Path: path, Action: "removed the aval:agent-rules block"})
	}
	return items, nil
}

// removeCreatedDirs removes directories an earlier install had to create, but
// only the ones that ended up empty.
func removeCreatedDirs(store *backup.Store, home string) ([]Item, error) {
	runs, err := store.List()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var dirs []string
	for _, r := range runs {
		if r.Legacy {
			continue
		}
		entries, err := store.Load(r)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.CreatedDir && !seen[e.Path] && strings.HasPrefix(e.Path, home+string(filepath.Separator)) {
				seen[e.Path] = true
				dirs = append(dirs, e.Path)
			}
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	var items []Item
	for _, d := range dirs {
		if fi, err := os.Lstat(d); err != nil || !fi.IsDir() {
			continue
		}
		if err := os.Remove(d); err == nil {
			items = append(items, Item{Path: d, Action: "removed empty directory"})
		}
	}
	return items, nil
}
