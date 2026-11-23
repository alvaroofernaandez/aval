package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/manifest"
	"github.com/alvaroofernaandez/aval/internal/core/repoloc"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
	"github.com/alvaroofernaandez/aval/internal/core/txsync"
	"github.com/alvaroofernaandez/aval/internal/core/uninstall"
	"github.com/alvaroofernaandez/aval/internal/core/update"
	"github.com/alvaroofernaandez/aval/internal/devmode"
	"github.com/alvaroofernaandez/aval/internal/release"
	"github.com/alvaroofernaandez/aval/internal/ui"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// hubEnv is every seam bare `aval` needs from the outside world, so tests
// never start a real tea.Program or probe the machine.
type hubEnv struct {
	isTTY    func(fd uintptr) bool
	getenv   func(string) string
	run      func(tea.Model) (tea.Model, error)
	lookPath func(string) (string, error)
	exe      func() (string, error)
	stderr   io.Writer
}

func realHubEnv() hubEnv {
	return hubEnv{
		isTTY:  term.IsTerminal,
		getenv: os.Getenv,
		run: func(m tea.Model) (tea.Model, error) {
			return tea.NewProgram(m, tea.WithAltScreen()).Run()
		},
		lookPath: exec.LookPath,
		exe:      os.Executable,
		stderr:   os.Stderr,
	}
}

// hubWanted: a terminal on stdin and stdout and a TERM that is not dumb.
func (e hubEnv) hubWanted() bool {
	return e.isTTY(os.Stdin.Fd()) && e.isTTY(os.Stdout.Fd()) && e.getenv("TERM") != "dumb"
}

// runBare is a bare `aval`: the hub on a terminal, the usage everywhere else
// (pipes, CI, TERM=dumb). Opening the hub writes nothing and makes no
// network call: the update line comes from the cache.
func runBare(out io.Writer, repoFlag string, flags uiFlags, e hubEnv) error {
	if !e.hubWanted() {
		printUsage(out)
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	us, err := settingsForCommand("", flags, e.getenv, home, e.stderr)
	if err != nil {
		return err
	}
	c := hubCaps(e, us)
	r := lipgloss.NewRenderer(os.Stdout)
	r.SetColorProfile(c.Profile)
	theme, err := ui.NewTheme(themeOrDefault(us.Theme), c, r)
	if err != nil {
		return err
	}
	svc, closeFn := newHubServices(e, home, repoFlag, us)
	_, err = e.run(ui.NewHub(theme, ui.PlatformLabel(runtime.GOOS, runtime.GOARCH), svc, c.Motion))
	if cerr := closeFn(); err == nil {
		err = cerr
	}
	return err
}

// hubCaps detects the terminal behind stdout.
func hubCaps(e hubEnv, us uiSettings) caps.Caps {
	fd := os.Stdout.Fd()
	width, height, _ := term.GetSize(fd)
	return caps.Detect(us.capsInputs(e.getenv, runtime.GOOS, e.isTTY(fd), width, height))
}

// newHubServices wires every ui.Services field to its real implementation.
// The returned close function ends any backup run an installer left open and
// cancels a clone still running.
//
// Without a repository the repo-bound functions stay nil, so their views show
// as unavailable instead of failing; backups, settings and install need none.
func newHubServices(e hubEnv, home, repoFlag string, us uiSettings) (ui.Services, func() error) {
	b := currentBuild()
	repo, repoErr := resolveRepo(repoFlag)
	if repoErr != nil {
		repo = ""
	}
	svc := ui.Services{
		Home: home, Repo: repo, RepoErr: repoErr,
		Build: ui.BuildInfo{Version: b.Version, Commit: b.Commit, Date: b.Date, Channel: release.Channel(b.Version)},
	}
	if e.getenv("AVAL_DEV") == "1" {
		svc.Sandbox = e.getenv("AVAL_DEV_NAME")
		if svc.Sandbox == "" {
			svc.Sandbox = "default"
		}
	}

	realHome := e.getenv("AVAL_DEV_REAL_HOME")
	if realHome == "" {
		realHome = home
	}
	devBase := devmode.BaseDir(e.getenv, realHome)
	goos := runtime.GOOS
	store := backup.NewStore(home)
	restoreOpt := backup.RestoreOptions{Repo: repo}
	cfgPath := config.Path(e.getenv, home, goos)

	// Backups, restore and pin work on ~/.aval-backup alone.
	svc.Backups = store.List
	svc.RunEntries = store.Load
	svc.RestorePreview = func(r backup.Run) ([]backup.Action, error) { return store.Preview(r, restoreOpt) }
	svc.Restore = func(r backup.Run) ([]backup.Action, error) { return store.Restore(r, restoreOpt) }
	svc.Pin = store.Pin
	svc.Sandboxes = func() ([]devmode.Info, error) { return devmode.List(devBase) }
	svc.RemoveSandbox = func(name string) error { return devmode.Remove(devBase, name, realHome) }

	fd := os.Stdout.Fd()
	tty := e.isTTY(fd)
	width, height, _ := term.GetSize(fd)
	svc.Settings = func() ui.SettingsEnv {
		return us.settingsEnv(cfgPath, home, e.getenv, tty, width, height)
	}
	svc.CachedNotice = func() update.Notice { return update.CachedWith(b.Version, us.UpdateCheck) }

	ctx, cancel := context.WithCancel(context.Background())
	var runner *install.Runner // the open backup run of the installer or re-apply on screen
	var chosen string
	closeRunner := func() error {
		if runner == nil {
			return nil
		}
		r := runner
		runner = nil
		return r.Finish()
	}
	opts := ui.Options{
		Version:  b.Version,
		Platform: ui.PlatformLabel(goos, runtime.GOARCH),
		Caps:     hubCaps(e, us),
		Theme:    us.Theme,
		Settings: svc.Settings(),
	}
	prepare := func(r string) (ui.Prepared, error) {
		if err := closeRunner(); err != nil {
			return ui.Prepared{}, err
		}
		m, p, pf, err := buildPlan(r, home)
		if err != nil {
			return ui.Prepared{}, err
		}
		if len(p.Groups) == 0 {
			return ui.Prepared{}, fmt.Errorf("nothing in the manifest applies to %s", pf.Display())
		}
		symlinks := symlinkMode(pf, home, false)
		runner = install.New(install.Options{Home: home, Symlinks: symlinks})
		chosen = r
		return ui.Prepared{Plan: p, Runner: runner, Unavailable: unavailableGroups(m, goos), Symlinks: symlinks}, nil
	}
	svc.Installer = func() (ui.Model, error) {
		if repo != "" {
			prep, err := prepare(repo)
			if err != nil {
				return ui.Model{}, err
			}
			o := opts
			o.Symlinks = prep.Symlinks
			m := ui.NewModel(prep.Plan, prep.Runner, o)
			m.Unavailable = prep.Unavailable
			return m, nil
		}
		env := ui.WelcomeEnv{
			Home:      home,
			CloneDest: filepath.Join(home, ".aval"),
			CloneURL:  cloneURL,
			Cloner:    gitCloner{ctx: ctx, url: cloneURL},
			Prepare:   prepare,
		}
		if wd, err := os.Getwd(); err == nil {
			env.Candidate = repoloc.Find(wd)
		}
		return ui.NewWelcome(opts, env), nil
	}
	svc.FinishInstall = func(ui.Model) ([]string, error) {
		if err := closeRunner(); err != nil {
			return nil, err
		}
		return completeInstall(chosen, home, false)
	}

	closeFn := func() error {
		cancel()
		return closeRunner()
	}
	if repo == "" {
		return svc, closeFn
	}

	pathOpt := func(security bool) doctor.Options {
		return doctor.Options{
			Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: goos,
			Security: security, ConfigPath: cfgPath,
		}
	}
	svc.Doctor = func(security bool) []doctor.Result { return doctor.Run(pathOpt(security)) }
	svc.FixCheck = func(id string) ([]string, error) {
		return fixCheck(pathOpt(false), store, id, home, goos)
	}

	uopt := uninstall.Options{Repo: repo, Home: home, GOOS: goos}
	svc.UninstallPreview = func() ([]uninstall.Item, error) { return uninstall.Preview(uopt) }
	svc.Uninstall = func() ([]uninstall.Item, error) { return uninstall.Run(uopt) }

	git := update.NewGit(repo)
	svc.UpdatePreview = func(ctx context.Context) (update.Preview, error) {
		var m *manifest.Manifest
		if loaded, err := manifest.Load(filepath.Join(repo, "install.manifest")); err == nil {
			m = loaded
		}
		return update.DryRun(ctx, git, update.ManagedPaths(m))
	}
	svc.Pull = func(ctx context.Context) (update.Status, error) { return update.Pull(ctx, git) }
	svc.Reapply = func() (ui.Model, error) {
		prep, err := prepare(repo)
		if err != nil {
			return ui.Model{}, err
		}
		o := opts
		o.Symlinks = prep.Symlinks
		m := ui.NewModel(prep.Plan, prep.Runner, o)
		m.Unavailable = prep.Unavailable
		return m, nil
	}
	// A re-apply only commits its backup run, as `aval update` does: no
	// settings seed, hooksPath or recorded repo (an install's concerns; the
	// repo is already known and linked). closeRunner is idempotent, so the
	// hub closing afterwards finds nothing open.
	svc.FinishReapply = closeRunner

	syncOpt := txsync.Options{Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: goos}
	svc.SyncPreview = func() (*txsync.Outcome, error) {
		o := syncOpt
		o.DryRun = true
		return txsync.Run(o)
	}
	svc.Sync = func() (*txsync.Outcome, error) { return txsync.Run(syncOpt) }

	// The dev view runs this binary, so it needs a checkout to seed from and
	// a Go toolchain on PATH for the sandbox's own builds.
	if _, err := e.lookPath("go"); err == nil {
		svc.DevCommand = func(sub, name string) *exec.Cmd {
			exe, err := e.exe()
			if err != nil {
				return &exec.Cmd{Err: err}
			}
			args := []string{"dev"}
			if sub != "" {
				args = append(args, sub)
			}
			return exec.Command(exe, append(args, "--name", name)...)
		}
	}
	return svc, closeFn
}

// fixCheck applies the hub's safe fix for one doctor result: dangling links
// are removed with a backup run (so restore undoes it), sync drift goes
// through the transactional sync.
func fixCheck(opt doctor.Options, store *backup.Store, id, home, goos string) ([]string, error) {
	var res *doctor.Result
	for _, r := range doctor.RunOnly(opt, id) {
		if r.ID == id {
			r := r
			res = &r
			break
		}
	}
	if res == nil || res.Fix == doctor.FixNone {
		return nil, fmt.Errorf("no safe fix for %q", id)
	}
	if res.Fix == doctor.FixBrokenLinks {
		rec := store.Begin()
		removed, err := doctor.RemoveBroken(opt, rec)
		if _, _, cerr := rec.Commit(); err == nil {
			err = cerr
		}
		lines := make([]string, len(removed))
		for i, p := range removed {
			lines[i] = "removed broken link " + short(home, p)
		}
		return lines, err
	}
	out, err := txsync.Run(txsync.Options{Repo: opt.Repo, Home: home, PrivateDir: opt.PrivateDir, GOOS: goos})
	var lines []string
	if out != nil {
		if out.RolledBack {
			err = errors.Join(err, fmt.Errorf("sync rolled back; backup %s is marked failed", out.RunID))
		}
		lines = syncLines(home, out.Report)
	}
	return lines, err
}

// syncLines names what a sync changed, for the fix result.
func syncLines(home string, rep *avalsync.Report) []string {
	if rep == nil {
		return nil
	}
	var lines []string
	if s := rep.Settings; len(s.Changed) > 0 || s.Created {
		lines = append(lines, "updated ~/.claude/settings.json: "+strings.Join(s.Changed, ", "))
	}
	for _, r := range rep.Rules {
		if !r.Skipped && r.Changed {
			lines = append(lines, "updated "+short(home, r.Path))
		}
	}
	return lines
}
