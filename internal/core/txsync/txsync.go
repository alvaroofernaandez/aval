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

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
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
	// Generation, when set, is the id of the generation to activate: its
	// Claude Code settings are applied instead of the repo's, and it becomes
	// the current generation once the doctor checks pass.
	Generation string
}

// Crash points of an activation, for the crash tests.
const (
	stageApplied  = "applied"
	stageSwitched = "switched"
)

// crash is called at each stage; the crash tests make it panic.
var crash = func(string) {}

// Recover restores the runs of transactions that died before finishing
// (no completion marker: a crash), marks them failed, and puts back the
// generation that was current before an interrupted activation. It returns
// the ids of the runs it restored.
func Recover(opt Options) ([]string, error) {
	store := backup.NewStore(opt.Home)
	store.Now = opt.Now
	stale, err := store.Pending()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, run := range stale {
		if _, err := store.Restore(run, backup.RestoreOptions{Repo: opt.Repo}); err != nil {
			return out, fmt.Errorf("sync: previous run %s was interrupted and could not be restored: %w", run.ID, err)
		}
		if err := store.MarkFailed(run, "interrupted: no completion marker (crash); restored by the next sync"); err != nil {
			return out, err
		}
		out = append(out, run.ID)
	}
	if len(out) == 0 {
		// A record without a pending run is an activation that finished
		// and crashed only before forgetting its record: it is live.
		return out, generation.EndSwitch(opt.Home)
	}
	return out, generation.UndoSwitch(opt.Home)
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
	syncOpt := avalsync.Options{Repo: opt.Repo, Home: opt.Home, PrivateDir: opt.PrivateDir, GOOS: opt.GOOS, Now: opt.Now}
	if opt.Generation != "" {
		if err := generation.Verify(opt.Home, opt.Generation); err != nil {
			return out, err
		}
		syncOpt.Generation = generation.Dir(opt.Home, opt.Generation, agentcfg.ClaudeCode)
	}

	if opt.DryRun {
		syncOpt.DryRun = true
		rep, err := avalsync.Run(syncOpt)
		out.Report = rep
		return out, err
	}

	store := backup.NewStore(opt.Home)
	store.Now = opt.Now
	ropt := backup.RestoreOptions{Repo: opt.Repo}

	recovered, err := Recover(opt)
	out.Recovered = recovered
	if err != nil {
		return out, err
	}

	rec := store.Begin()
	rec.SetOrigin("sync")
	if err := rec.SetPending(); err != nil {
		return out, fmt.Errorf("sync: backup: %w", err)
	}
	syncOpt.Recorder = rec
	if opt.Generation != "" {
		if err := generation.BeginSwitch(opt.Home); err != nil {
			return out, rollback(opt.Home, store, rec, ropt, out, "activation: "+err.Error())
		}
	}

	rep, err := avalsync.Run(syncOpt)
	out.Report = rep
	if err != nil {
		return out, rollback(opt.Home, store, rec, ropt, out, "sync failed: "+err.Error())
	}
	crash(stageApplied)

	if failed := verify(opt, syncOpt.Generation, out); len(failed) > 0 {
		return out, rollback(opt.Home, store, rec, ropt, out, "doctor checks failed after sync: "+strings.Join(failed, "; "))
	}

	if opt.Generation != "" {
		if err := generation.SetCurrent(opt.Home, opt.Generation); err != nil {
			return out, rollback(opt.Home, store, rec, ropt, out, "activation: "+err.Error())
		}
		crash(stageSwitched)
	}

	run, _, err := rec.Commit()
	if err != nil {
		return out, err
	}
	out.RunID = run.ID
	return out, finishSwitch(opt)
}

// finishSwitch records a finished activation in the history, then forgets
// its crash record.
func finishSwitch(opt Options) error {
	if opt.Generation != "" {
		when := time.Now
		if opt.Now != nil {
			when = opt.Now
		}
		if err := generation.AppendHistory(opt.Home, opt.Generation, when()); err != nil {
			return err
		}
	}
	return generation.EndSwitch(opt.Home)
}

// verify runs the doctor checks sync can change against what was applied
// (the generation's tree, or the repo), records the failures in out and
// returns them as "id: message".
func verify(opt Options, gen string, out *Outcome) []string {
	dopt := doctor.Options{
		Repo: opt.Repo, Home: opt.Home, PrivateDir: opt.PrivateDir, GOOS: opt.GOOS, LookPath: opt.LookPath,
		Generation: gen,
	}
	var failed []string
	for _, r := range doctor.RunOnly(dopt, Checks...) {
		if r.Status == doctor.Fail {
			out.Failed = append(out.Failed, r)
			failed = append(failed, r.ID+": "+r.Message)
		}
	}
	return failed
}

// rollback restores the run, keeps it marked failed, and builds the error.
func rollback(home string, store *backup.Store, rec *backup.Recorder, ropt backup.RestoreOptions, out *Outcome, reason string) error {
	// The pointer moves last, after every check: a rolled-back activation
	// never switched it, so its record is simply forgotten.
	_ = generation.EndSwitch(home)
	run := store.LoadRun(rec.Dir())
	if len(rec.Entries()) == 0 {
		_ = rec.Fail(reason)
		out.RolledBack = true
		out.RunID = run.ID
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
