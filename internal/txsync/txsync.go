// Package txsync runs sync as a transaction: back up, apply, verify with the
// doctor checks sync can affect, and undo everything if any of them fails.
//
// There is exactly one attempt. A failed verification restores the backup run
// and reports why; it never retries and never tries a "smaller" sync, because
// a second guess at what to write is how a half-working config survives.
package txsync

import (
	"fmt"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/backup"
	"github.com/alvaroofernaandez/aval/internal/doctor"
	avalsync "github.com/alvaroofernaandez/aval/internal/sync"
)

// Checks are the doctor checks that sync can change.
var Checks = []string{"settings-drift", "hooks-resolve", "hooks-retired", "hooks-drift", "overlay"}

// Options configures a run.
type Options struct {
	Repo, Home, PrivateDir, GOOS string
	Now                          func() time.Time
	// LookPath resolves bare hook commands in the doctor; nil means exec.LookPath.
	LookPath func(string) (string, error)
	DryRun   bool
}

// Outcome is what happened.
type Outcome struct {
	Report *avalsync.Report
	// RunID is the backup run of this sync ("" when nothing was recorded).
	RunID string
	// Recovered lists stale pending runs (a crashed sync) restored first.
	Recovered  []string
	Failed     []doctor.Result
	RolledBack bool
}

// Run applies sync transactionally.
func Run(opt Options) (*Outcome, error) {
	out := &Outcome{}
	syncOpt := avalsync.Options{Repo: opt.Repo, Home: opt.Home, PrivateDir: opt.PrivateDir, Now: opt.Now}

	if opt.DryRun {
		syncOpt.DryRun = true
		rep, err := avalsync.Run(syncOpt)
		out.Report = rep
		return out, err
	}

	store := backup.NewStore(opt.Home)
	store.Now = opt.Now
	ropt := backup.RestoreOptions{Repo: opt.Repo}

	// A pending marker that outlived its process means the last sync crashed
	// mid-apply: put things back before building on top of them.
	stale, err := store.Pending()
	if err != nil {
		return out, err
	}
	for _, run := range stale {
		if _, err := store.Restore(run, ropt); err != nil {
			return out, fmt.Errorf("sync: previous run %s was interrupted and could not be restored: %w", run.ID, err)
		}
		if err := store.MarkFailed(run, "interrupted: no completion marker (crash); restored by the next sync"); err != nil {
			return out, err
		}
		out.Recovered = append(out.Recovered, run.ID)
	}

	rec := store.Begin()
	if err := rec.SetPending(); err != nil {
		return out, fmt.Errorf("sync: backup: %w", err)
	}
	syncOpt.Recorder = rec

	rep, err := avalsync.Run(syncOpt)
	out.Report = rep
	if err != nil {
		return out, rollback(store, rec, ropt, out, "sync failed: "+err.Error())
	}

	dopt := doctor.Options{
		Repo: opt.Repo, Home: opt.Home, PrivateDir: opt.PrivateDir, GOOS: opt.GOOS, LookPath: opt.LookPath,
	}
	var failed []string
	for _, r := range doctor.RunOnly(dopt, Checks...) {
		if r.Status == doctor.Fail {
			out.Failed = append(out.Failed, r)
			failed = append(failed, r.ID+": "+r.Message)
		}
	}
	if len(failed) > 0 {
		return out, rollback(store, rec, ropt, out, "doctor checks failed after sync: "+strings.Join(failed, "; "))
	}

	run, _, err := rec.Commit()
	if err != nil {
		return out, err
	}
	out.RunID = run.ID
	return out, nil
}

// rollback restores the run, keeps it marked failed, and builds the error.
func rollback(store *backup.Store, rec *backup.Recorder, ropt backup.RestoreOptions, out *Outcome, reason string) error {
	run := store.LoadRun(rec.Dir())
	if len(rec.Entries()) == 0 {
		_ = rec.Fail(reason)
		out.RolledBack = true
		return fmt.Errorf("sync rolled back: %s (nothing had been written)", reason)
	}
	if _, err := store.Restore(run, ropt); err != nil {
		_ = store.MarkFailed(run, reason+"; RESTORE FAILED: "+err.Error())
		return fmt.Errorf("sync failed and the automatic restore failed too (%v); restore by hand with: aval restore %s. Reason: %s", err, run.ID, reason)
	}
	if err := rec.Fail(reason); err != nil {
		return err
	}
	out.RolledBack = true
	out.RunID = run.ID
	return fmt.Errorf("sync rolled back: %s (previous state restored from backup %s)", reason, run.ID)
}
