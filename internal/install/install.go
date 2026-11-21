// Package install executes a plan against the filesystem.
//
// It keeps the promise install.sh makes and that users rely on: nothing is ever
// deleted. Anything already sitting at a destination is moved into a timestamped
// quarantine directory before the new entry takes its place, and re-running is a
// no-op for entries already pointing at the repo.
package install

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alvaroofernaandez/aval/internal/backup"
	"github.com/alvaroofernaandez/aval/internal/plan"
)

// Status is what happened to one action.
type Status int

const (
	StatusLinked  Status = iota // a symlink was created
	StatusCopied                // content was copied (Windows without symlink rights)
	StatusAlready               // already pointing at the repo; nothing done
	StatusWould                 // dry run
)

func (s Status) String() string {
	switch s {
	case StatusLinked:
		return "linked"
	case StatusCopied:
		return "copied"
	case StatusAlready:
		return "already installed"
	case StatusWould:
		return "would install"
	}
	return "unknown"
}

// Result describes one completed action.
type Result struct {
	Action plan.Action
	Status Status
	// Backup is where a displaced file went, empty if nothing was displaced.
	Backup string
}

// Options configures a Runner.
type Options struct {
	Home string
	// Symlinks selects link-vs-copy. It comes from a real probe on Windows
	// rather than from GOOS, since the answer depends on Developer Mode.
	Symlinks bool
	DryRun   bool
	// BackupRoot overrides where displaced files go. Empty means a fresh run
	// under ~/.aval-backup/<YYYYmmdd-HHMMSS>.
	BackupRoot string
	// Recorder, when set, receives everything this runner displaces or
	// creates (sync shares one run across its steps). The caller commits it.
	Recorder *backup.Recorder
	// Now stamps the backup run; nil means time.Now.
	Now func() time.Time
}

// Runner applies actions.
type Runner struct {
	opt Options
	rec *backup.Recorder
	own bool // the runner created rec and so commits it in Finish
}

// New builds a Runner, fixing the backup run for the whole invocation so that
// everything displaced by it lands together.
func New(opt Options) *Runner {
	r := &Runner{opt: opt, rec: opt.Recorder}
	if r.rec == nil {
		r.own = true
		switch {
		case opt.BackupRoot != "":
			r.rec = backup.RecorderAt(opt.BackupRoot, opt.Home)
		default:
			st := backup.NewStore(opt.Home)
			st.Now = opt.Now
			r.rec = st.Begin()
		}
	}
	return r
}

// BackupRoot is where this run puts displaced files.
func (r *Runner) BackupRoot() string { return r.rec.Dir() }

// Finish closes the backup run: it is dropped when nothing was recorded or
// when it equals the previous run, and old runs are pruned. A runner given a
// shared Recorder leaves that to its owner.
func (r *Runner) Finish() error {
	if !r.own || r.opt.DryRun {
		return nil
	}
	_, _, err := r.rec.Commit()
	return err
}

// Apply performs one action.
func (r *Runner) Apply(a plan.Action) (Result, error) {
	res := Result{Action: a}

	if _, err := os.Stat(a.Source); err != nil {
		return res, fmt.Errorf("install: source missing: %s", a.Source)
	}

	// Already correct? Nothing to do — this is what makes re-running safe.
	if r.opt.Symlinks {
		if target, err := os.Readlink(a.Dest); err == nil && target == a.Source {
			res.Status = StatusAlready
			return res, nil
		}
	}

	if r.opt.DryRun {
		res.Status = StatusWould
		return res, nil
	}

	if err := r.rec.MkdirAll(filepath.Dir(a.Dest), 0o755); err != nil {
		return res, fmt.Errorf("install: %w", err)
	}

	// Displace handles broken links too (it uses Lstat, not Stat) and records
	// a destination that does not exist, so a restore can remove what we make.
	saved, err := r.rec.Displace(a.Dest)
	if err != nil {
		return res, fmt.Errorf("install: backup %s: %w", a.Label, err)
	}
	res.Backup = saved

	if r.opt.Symlinks {
		if err := os.Symlink(a.Source, a.Dest); err != nil {
			return res, fmt.Errorf("install: link %s: %w", a.Label, err)
		}
		res.Status = StatusLinked
	} else {
		if err := copyPath(a.Source, a.Dest); err != nil {
			return res, fmt.Errorf("install: copy %s: %w", a.Label, err)
		}
		res.Status = StatusCopied
	}
	if err := r.rec.Wrote(a.Dest); err != nil {
		return res, fmt.Errorf("install: record %s: %w", a.Label, err)
	}
	return res, nil
}

// copyPath copies a file or a directory tree, preserving permission bits.
// Most manifest entries are directories, and the tmux scripts among them are
// only useful if the executable bit survives.
func copyPath(src, dst string) error { return backup.CopyPath(src, dst) }
