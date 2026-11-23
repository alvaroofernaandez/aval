package ui

import (
	"context"
	"os/exec"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	"github.com/alvaroofernaandez/aval/internal/core/txsync"
	"github.com/alvaroofernaandez/aval/internal/core/uninstall"
	coreupdate "github.com/alvaroofernaandez/aval/internal/core/update"
	"github.com/alvaroofernaandez/aval/internal/devmode"
)

// Services is everything the hub and its views need from the outside world,
// defined once so view tasks never edit it. Behaviour is reached only through
// these functions, which cmd/aval wires to the real core packages and tests
// replace with fakes: no view touches the network, HOME or git directly.
//
// A nil function means the capability is not available here; the view that
// needs it shows as unavailable instead of calling it.
type Services struct {
	// Home is the real home directory, for ~ shortening.
	Home string
	// Repo is the resolved aval checkout; empty with RepoErr set when none
	// was found.
	Repo    string
	RepoErr error
	// Build is what `aval version` prints.
	Build BuildInfo
	// Sandbox is the `aval dev` sandbox name when running inside one, empty
	// otherwise. Inside a sandbox the dev view is read-only.
	Sandbox string

	// Doctor runs the read-only checks; security adds the --security ones.
	Doctor func(security bool) []doctor.Result
	// FixCheck applies the hub's safe fix for one check id (doctor.FixKind
	// says which) and returns what it changed.
	FixCheck func(id string) ([]string, error)

	// Backups lists the runs under ~/.aval-backup, newest first.
	Backups func() ([]backup.Run, error)
	// RunEntries lists the paths a run holds.
	RunEntries func(run backup.Run) ([]backup.Entry, error)
	// RestorePreview says what Restore would do and writes nothing.
	RestorePreview func(run backup.Run) ([]backup.Action, error)
	Restore        func(run backup.Run) ([]backup.Action, error)
	// Pin keeps a run out of pruning, or releases it.
	Pin func(id string, on bool) error

	// UninstallPreview is the dry run of Uninstall and writes nothing.
	UninstallPreview func() ([]uninstall.Item, error)
	Uninstall        func() ([]uninstall.Item, error)

	// UpdatePreview fetches and compares, without pulling.
	UpdatePreview func(ctx context.Context) (coreupdate.Preview, error)
	// Pull fast-forwards the checkout.
	Pull func(ctx context.Context) (coreupdate.Status, error)
	// Reapply builds the installer that re-links what the pull brought,
	// titled "Re-applying".
	Reapply func() (Model, error)

	// SyncPreview is the transactional sync as a dry run; Sync applies it,
	// verifies with doctor and rolls back on failure.
	SyncPreview func() (*txsync.Outcome, error)
	Sync        func() (*txsync.Outcome, error)

	// Sandboxes lists the dev sandboxes.
	Sandboxes func() ([]devmode.Info, error)
	// DevCommand is `aval dev [sub] --name <name>`, run through
	// tea.ExecProcess; sub is "" for a shell or "reset".
	DevCommand    func(sub, name string) *exec.Cmd
	RemoveSandbox func(name string) error

	// Installer builds the install flow: select when a repository is known,
	// welcome otherwise.
	Installer func() (Model, error)
	// FinishInstall runs the post-install steps after a completed run and
	// returns warnings instead of writing to stderr.
	FinishInstall func(m Model) ([]string, error)
	// FinishReapply closes the backup run of the update view's re-apply when
	// its done screen closes, completed or stopped. Unlike FinishInstall it
	// does no post-install steps: `aval update` re-applies the links and
	// commits the run, nothing more.
	FinishReapply func() error
	// Settings is where the settings screen reads and writes.
	Settings func() SettingsEnv
	// CachedNotice is the update line from the last check's cache; it never
	// fetches.
	CachedNotice func() coreupdate.Notice
}

// BuildInfo is the build block of `aval version`.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
	Channel string
}
