// Command aval installs this repository's configs on macOS,
// Linux and Windows.
//
//	aval            print the usage; never installs
//	aval install    interactive TUI (--yes: none, --dry-run: change nothing, --copy)
//	aval update     pull the repo, then re-apply
//	aval doctor     check this machine against the repo (--fix, --json)
//	aval update --dry-run  preview what update would change
//	aval budget     always-on context per agent and hook latency (--json)
//	aval review assess  git-only risk tier and reviewer lenses (--base, --json)
//	aval sync       merge repo-managed settings into the live config (--dry-run)
//	aval rules sync mirror CLAUDE.md's shared rules and lessons into OpenCode's AGENTS.md (--check)
//	aval macos      macOS defaults and the screenshot-to-clipboard LaunchAgent
//	aval reap-orphan-tests  end orphaned test processes (--min-age, --dry-run)
//	aval hook ...   agent hooks and the git pre-push gate
//	aval config   show or change settings: list, get, set, path
//	aval restore    list or restore backup runs (--list, latest, <id>, --pin <id>)
//	aval uninstall  remove aval's links and restore what they displaced (--dry-run, --yes)
//	aval version    version, commit, build date and channel (--version: same)
//	aval release    cut a release: status, next, patch|minor|major|rc|beta|promote
//
// The non-interactive paths matter as much as the TUI: --yes makes it usable
// from a provisioning script, and update is what a cron job or a shell hook
// calls to keep a machine current.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/alvaroofernaandez/aval/internal/agent/statusline"
	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/manifest"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/core/platform"
	"github.com/alvaroofernaandez/aval/internal/core/repoloc"
	"github.com/alvaroofernaandez/aval/internal/core/update"
	"github.com/alvaroofernaandez/aval/internal/release"
	"github.com/alvaroofernaandez/aval/internal/ui"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
	"github.com/alvaroofernaandez/aval/internal/ui/cli"
)

// Stamped at build time by GoReleaser (see .goreleaser.yaml):
//
//	go build -ldflags "-X main.version=0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%FT%TZ)"
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, errChecksFailed) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

func run() error {
	var (
		dryRun    = flag.Bool("dry-run", false, "show what would happen, change nothing")
		yes       = flag.Bool("yes", false, "install everything applicable without the TUI")
		repoFlag  = flag.String("repo", "", "path to the aval repository (default: $AVAL_REPO, then the one aval install recorded)")
		showVer   = flag.Bool("version", false, "print the version and exit")
		forceCopy = flag.Bool("copy", false, "copy files instead of symlinking them")
		themeFlag = flag.String("theme", "", "colour theme for this run (default: $AVAL_THEME, then the config file, then aval)")
		plainFlag = flag.Bool("plain", false, "monochrome, ASCII glyphs and no motion")
	)
	flag.Usage = func() { printUsage(flag.CommandLine.Output()) }
	_ = flag.CommandLine.Parse(hoistGlobals(os.Args[1:])) // ExitOnError: never returns an error

	if *showVer {
		fmt.Print(currentBuild().text(runtime.GOOS, runtime.GOARCH))
		return nil
	}

	// A bare `aval` prints the usage (or opens the hub on a terminal, which
	// writes nothing): installing writes to HOME, so it is never the default. Unknown commands fail here, before any repository lookup.
	legacyInstall := flag.NArg() == 0 && installFlagsGiven()
	if flag.Arg(0) == "help" {
		printUsage(os.Stdout)
		return nil
	}
	if flag.NArg() == 0 && !legacyInstall {
		// On a terminal this opens the hub; everywhere else, the usage.
		return runBare(os.Stdout, *repoFlag, uiFlags{Theme: *themeFlag, Plain: *plainFlag}, realHubEnv())
	}
	if !legacyInstall && !knownCommand(flag.Arg(0)) {
		return fmt.Errorf("unknown command %q; run 'aval help'", flag.Arg(0))
	}

	// version and release need no aval repository: release works on the
	// repository in the current directory.
	if flag.Arg(0) == "version" {
		fmt.Print(currentBuild().text(runtime.GOOS, runtime.GOARCH))
		return nil
	}
	if flag.Arg(0) == "release" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		return runRelease(wd, flag.Args()[1:], os.Stdin, os.Stdout)
	}

	// The status line refreshes constantly: no repo lookup, no network.
	if flag.Arg(0) == "statusline" {
		fmt.Fprintln(os.Stdout, statusline.Render(os.Stdin))
		return nil
	}

	// Agent hooks run on every tool call: no repo lookup, no network.
	if flag.Arg(0) == "hook" {
		return runHook(flag.Args()[1:], os.Stdin, os.Stdout)
	}

	// review works on the repository in the current directory, not on aval's.
	if flag.Arg(0) == "review" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		return runReview(wd, flag.Args()[1:], os.Stdout)
	}

	// design gates and ack work on the project in the current directory.
	if flag.Arg(0) == "design" {
		wd, _ := os.Getwd()
		return runDesign(flag.Args()[1:], wd, os.Stdout)
	}

	// config reads and writes only aval's own settings file: no repo lookup.
	if flag.Arg(0) == "config" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		return runConfig(flag.Args()[1:], os.Stdout, os.Stderr, os.Getenv, home)
	}

	// Unix-only commands say so instead of failing somewhere inside.
	if err := unsupportedHere(runtime.GOOS, flag.Arg(0)); err != nil {
		return err
	}

	// tmux/Ghostty/zsh helpers run on every refresh or cd: no repo lookup.
	if flag.Arg(0) == "tmux" {
		os.Exit(runTmux(flag.Args()[1:]))
	}

	// macOS helpers and the orphan-test reaper are called by launchd with no
	// repository around: no repo lookup.
	if flag.Arg(0) == "macos" {
		return runMacos(flag.Args()[1:], os.Stdout)
	}
	if flag.Arg(0) == "reap-orphan-tests" {
		return runReap(flag.Args()[1:], os.Stdout)
	}

	// Skill tooling: standalone, no repository and no update check.
	switch flag.Arg(0) {
	case "kiri":
		return runKiri(flag.Args()[1:])
	case "docker":
		return runDocker(flag.Args()[1:])
	case "pdf":
		return runPDF(flag.Args()[1:], os.Stdout)
	case "deck":
		return runDeck(flag.Args()[1:], os.Stdout, realDeckDeps())
	case "dev":
		return runDev(*repoFlag, flag.Args()[1:])
	}

	// Only commands that draw read the UI settings; everything above this line
	// is a hot path or standalone and never touches the config file.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	cmdName := flag.Arg(0)
	if legacyInstall {
		cmdName = "install"
	}
	uiCfg, err := settingsForCommand(cmdName, uiFlags{Theme: *themeFlag, Plain: *plainFlag}, os.Getenv, home, os.Stderr)
	if err != nil {
		return err
	}

	opts := installOpts{dryRun: *dryRun, yes: *yes, copy: *forceCopy}
	if flag.Arg(0) == "install" {
		var err error
		if opts, err = parseInstallArgs(opts, flag.Args()[1:]); err != nil {
			return err
		}
	}

	// install resolves the repository itself: with none found it opens the
	// welcome screen instead of failing here.
	if legacyInstall || flag.Arg(0) == "install" {
		deps := realInstallDeps()
		deps.ui = uiCfg
		deps.notify = func() {
			if legacyInstall {
				fmt.Fprintln(os.Stderr, "notice: running aval with only flags is deprecated; use 'aval install [--dry-run] [--yes] [--copy]'")
			}
			notifyIfOutdated(uiCfg)
		}
		return runInstallWith(func() (string, error) { return resolveRepo(*repoFlag) },
			home, opts.dryRun, opts.yes, opts.copy, deps)
	}

	repo, err := resolveRepo(*repoFlag)
	if err != nil {
		return err
	}

	// doctor and sync are offline and local, so they dispatch before the
	// update notice: a health check must not wait on the network.
	switch flag.Arg(0) {
	case "doctor":
		return runDoctor(repo, home, flag.Args()[1:], os.Stdout, uiCfg)
	case "budget":
		return runBudget(home, flag.Args()[1:], os.Stdout)
	case "sync":
		return runSync(repo, home, flag.Args()[1:], *dryRun, os.Stdout)
	case "rules":
		return runRules(repo, flag.Args()[1:], os.Stdout)
	case "restore":
		return runRestore(repo, home, flag.Args()[1:], *dryRun, os.Stdout)
	case "uninstall":
		return runUninstall(repo, home, flag.Args()[1:], *dryRun, *yes, os.Stdin, os.Stdout)
	}

	if flag.Arg(0) == "update" {
		return runUpdate(repo, home, *dryRun, *forceCopy, os.Stdout)
	}

	// Every command in the list dispatches above; install left earlier.
	return fmt.Errorf("unknown command %q; run 'aval help'", flag.Arg(0))
}

// installFlagsGiven reports whether an install flag was set without a
// subcommand, the form scripts used before install was a subcommand.
func installFlagsGiven() bool {
	given := false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "dry-run", "yes", "copy":
			given = true
		}
	})
	return given
}

// notifyIfOutdated tells the user when a newer release exists.
//
// npm never pushes an update to an installed package, so a globally installed
// copy stays on whatever version it was installed at until someone acts. This
// is that prompt. It is rate-limited to once a day, opt-out-able via
// AVAL_NO_UPDATE_CHECK or update_check = off, and silent on every failure — being offline must
// not stop anyone installing their dotfiles.
func notifyIfOutdated(s uiSettings) {
	v := currentBuild().Version
	if release.Channel(v) == release.ChannelDev || !update.ShouldCheckWith(time.Now(), s.UpdateCheck) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Printed to stderr so it never contaminates the output of --dry-run when
	// that is piped somewhere.
	if notice := update.CheckLatest(ctx, v).String(); notice != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n\n", notice)
	}
}

// resolveRepo finds the repository: --repo, $AVAL_REPO, the location that
// install recorded, then the working directory and the binary's own checkout.
// See internal/core/repoloc for the order and why explicit sources fail loudly.
func resolveRepo(flagValue string) (string, error) {
	s := repoloc.Sources{Flag: flagValue, Env: os.Getenv(repoloc.EnvVar)}
	if home, err := os.UserHomeDir(); err == nil {
		s.ConfigFile = repoloc.ConfigPath(os.Getenv, home, runtime.GOOS)
	}
	if wd, err := os.Getwd(); err == nil {
		s.Wd = wd
	}
	if exe, err := os.Executable(); err == nil {
		s.Exe = filepath.Dir(exe)
	}
	return repoloc.Resolve(s)
}

// buildPlan is shared by install and update so the two can never disagree
// about what belongs on this machine.
func buildPlan(repo, home string) (*manifest.Manifest, *plan.Plan, platform.Platform, error) {
	pf := platform.Detect()

	m, err := manifest.Load(filepath.Join(repo, "install.manifest"))
	if err != nil {
		return nil, nil, pf, err
	}
	p, err := plan.Build(m, plan.Options{Repo: repo, Home: home, GOOS: runtime.GOOS})
	if err != nil {
		return nil, nil, pf, err
	}
	return m, p, pf, nil
}

// symlinkMode decides link-vs-copy by probing rather than by consulting GOOS:
// on Windows the answer depends on Developer Mode and elevation, neither of
// which is knowable in advance.
func symlinkMode(pf platform.Platform, home string, forceCopy bool) bool {
	if forceCopy {
		return false
	}
	return pf.SupportsSymlinks(home)
}

// unavailableGroups lists what this machine cannot take, so the TUI can show
// it greyed out instead of silently omitting it. The reason names the machine
// it runs on: only native Windows lacks a group today, and WSL is the way in.
func unavailableGroups(m *manifest.Manifest, goos string) []ui.UnavailableGroup {
	reason := "needs WSL on Windows"
	if goos != "windows" {
		reason = "not supported on " + ui.PlatformLabel(goos, runtime.GOARCH)
	}
	var out []ui.UnavailableGroup
	for _, g := range m.Groups {
		if !g.AppliesTo(goos) {
			out = append(out, ui.UnavailableGroup{Label: g.Label, Reason: reason})
		}
	}
	return out
}

func runInstall(repo, home string, dryRun, yes, forceCopy bool, s uiSettings) error {
	deps := realInstallDeps()
	deps.ui = s
	return runInstallWith(fixedRepo(repo), home, dryRun, yes, forceCopy, deps)
}

// installDeps are the seams runInstall needs from the outside world.
type installDeps struct {
	isTTY  func(fd uintptr) bool
	run    func(tea.Model) (tea.Model, error)
	finish func(repo, home string, dryRun bool) error
	// ui is the resolved look-and-feel; the zero value means defaults.
	ui uiSettings
	// cloner fetches the repository from the welcome screen.
	cloner ui.Cloner
	// notify runs once the repository question is settled: the deprecation
	// notice and the update check. Nil does nothing.
	notify func()
}

func realInstallDeps() installDeps {
	return installDeps{
		isTTY: term.IsTerminal,
		run: func(m tea.Model) (tea.Model, error) {
			return tea.NewProgram(m, tea.WithAltScreen()).Run()
		},
		finish: finishInstall,
	}
}

// installGuard refuses the interactive installer when there is nothing to
// draw on: without --yes it needs a terminal on both stdin and stdout.
func installGuard(yes bool, isTTY func(uintptr) bool) error {
	if yes || (isTTY(os.Stdin.Fd()) && isTTY(os.Stdout.Fd())) {
		return nil
	}
	return errors.New("aval install needs a terminal. For scripts use: aval install --yes")
}

// driveInstall runs the program and finishes the install only for a completed
// run. closeRunner ends the backup run; finish seeds settings, syncs and
// records the repo.
func driveInstall(model ui.Model, run func(tea.Model) (tea.Model, error), closeRunner, finish func() error) error {
	final, err := run(model)
	if ferr := closeRunner(); err == nil {
		err = ferr
	}
	if err != nil {
		return err
	}
	if fm, ok := final.(ui.Model); ok && len(fm.Errs()) > 0 {
		return fmt.Errorf("%d action(s) failed", len(fm.Errs()))
	}
	// Quitting on the select screen (or anywhere before the done screen) must
	// leave settings, sync, the repo record and git hooks untouched. A dry run
	// still reaches finish: finishInstall writes nothing when dryRun is set.
	if fm, ok := final.(ui.Model); ok && !fm.Completed() {
		return nil
	}
	return finish()
}

// runInstallWith resolves the repository, then installs it. With no
// repository found, a terminal and no --yes, it opens the welcome screen
// instead of failing; every other resolution error is returned unchanged.
// The welcome screen, select, install and done all run in one program.
func runInstallWith(resolve func() (string, error), home string, dryRun, yes, forceCopy bool, deps installDeps) error {
	repo, err := resolve()
	welcome := err != nil && opensWelcome(err, yes, deps.isTTY)
	if err != nil && !welcome {
		return err
	}
	if deps.notify != nil {
		deps.notify()
	}
	if err := installGuard(yes, deps.isTTY); err != nil {
		return err
	}

	if yes {
		_, p, pf, err := buildPlan(repo, home)
		if err != nil {
			return err
		}
		if len(p.Groups) == 0 {
			return fmt.Errorf("nothing in the manifest applies to %s", pf.Display())
		}
		symlinks := symlinkMode(pf, home, forceCopy)
		runner := install.New(install.Options{Home: home, Symlinks: symlinks, DryRun: dryRun})
		err = runHeadless(p, pf, runner, symlinks, dryRun, deps.ui)
		if err == nil {
			err = finishInstall(repo, home, dryRun)
		}
		return finish(runner, err)
	}

	// prepare is shared by both entries, so the repo finish records and the
	// runner closeRunner commits are the ones the program actually used.
	var (
		chosen string
		runner *install.Runner
	)
	prepare := func(r string) (ui.Prepared, error) {
		m, p, pf, err := buildPlan(r, home)
		if err != nil {
			return ui.Prepared{}, err
		}
		if len(p.Groups) == 0 {
			return ui.Prepared{}, fmt.Errorf("nothing in the manifest applies to %s", pf.Display())
		}
		symlinks := symlinkMode(pf, home, forceCopy)
		runner = install.New(install.Options{Home: home, Symlinks: symlinks, DryRun: dryRun})
		chosen = r
		return ui.Prepared{Plan: p, Runner: runner, Unavailable: unavailableGroups(m, runtime.GOOS), Symlinks: symlinks}, nil
	}

	fd := os.Stdout.Fd()
	width, height, _ := term.GetSize(fd)
	tty := term.IsTerminal(fd)
	opts := ui.Options{
		Version:  version,
		Platform: ui.PlatformLabel(runtime.GOOS, runtime.GOARCH),
		Caps:     caps.Detect(deps.ui.capsInputs(os.Getenv, runtime.GOOS, tty, width, height)),
		Theme:    deps.ui.Theme,
		DryRun:   dryRun,
		Settings: deps.ui.settingsEnv(config.Path(os.Getenv, home, runtime.GOOS), home, os.Getenv, tty, width, height),
	}

	var model ui.Model
	if welcome {
		// Cancelling the context stops a clone still running when the
		// program ends (q or ctrl+c while cloning).
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cloner := deps.cloner
		if cloner == nil {
			cloner = gitCloner{ctx: ctx, url: cloneURL}
		}
		env := ui.WelcomeEnv{
			Home:      home,
			CloneDest: filepath.Join(home, ".aval"),
			CloneURL:  cloneURL,
			Cloner:    cloner,
			Prepare:   prepare,
		}
		if wd, err := os.Getwd(); err == nil {
			env.Candidate = repoloc.Find(wd)
		}
		model = ui.NewWelcome(opts, env)
	} else {
		prep, err := prepare(repo)
		if err != nil {
			return err
		}
		opts.Symlinks = prep.Symlinks
		model = ui.NewModel(prep.Plan, prep.Runner, opts)
		model.Unavailable = prep.Unavailable
	}

	return driveInstall(model, deps.run,
		func() error {
			if runner == nil {
				return nil
			}
			return runner.Finish()
		},
		func() error { return deps.finish(chosen, home, dryRun) })
}

// opensWelcome reports whether a resolution error should open the welcome
// screen: nothing named a repository and none was found, there is a terminal
// on both ends, and no --yes. A wrong --repo, $AVAL_REPO or config value
// still fails loudly, and scripts keep the old error.
func opensWelcome(err error, yes bool, isTTY func(uintptr) bool) bool {
	return errors.Is(err, repoloc.ErrNotFound) && !yes &&
		isTTY(os.Stdin.Fd()) && isTTY(os.Stdout.Fd())
}

// finish closes the runner's backup run without hiding the original error.
func finish(r *install.Runner, err error) error {
	if ferr := r.Finish(); err == nil {
		return ferr
	}
	return err
}

// runHeadless is the provisioning path: no TUI, one line per action, and a
// non-zero exit if anything failed. Status cells are styled only when stdout is
// a terminal; otherwise the output is plain and byte-stable.
func runHeadless(p *plan.Plan, pf platform.Platform, runner *install.Runner, symlinks, dryRun bool, s uiSettings) error {
	return runHeadlessTo(os.Stdout, newPrinterFor(os.Stdout, s), p, pf, runner, symlinks, dryRun)
}

func runHeadlessTo(w io.Writer, pr *cli.Printer, p *plan.Plan, pf platform.Platform, runner *install.Runner, symlinks, dryRun bool) error {
	fmt.Fprintf(w, "aval %s on %s\n", version, pf.Display())
	fmt.Fprintf(w, "%s\n\n", pf.InstallStyle(symlinks))

	var failures int
	backups := 0
	for _, a := range p.SelectedActions() {
		res, err := runner.Apply(a)
		if err != nil {
			fmt.Fprintf(w, "  %-44s %s\n", a.Label, pr.Fail("FAILED: "+err.Error()))
			failures++
			continue
		}
		status := pr.OK(res.Status.String())
		if res.Status == install.StatusWould {
			status = pr.Muted(res.Status.String())
		}
		note := ""
		if res.Backup != "" {
			note = "  " + pr.Muted("(backed up)")
			backups++
		}
		fmt.Fprintf(w, "  %-44s %s%s\n", a.Label, status, note)
	}

	if backups > 0 {
		fmt.Fprintf(w, "\n%d existing file(s) moved to %s\n", backups, runner.BackupRoot())
	}
	if failures > 0 {
		return fmt.Errorf("%d action(s) failed", failures)
	}
	if dryRun {
		fmt.Fprintln(w, "\nDry run: nothing was written.")
	}
	return nil
}

// runUpdate pulls the repository and re-applies.
//
// Pulling is most of the update on its own, because the configs are symlinks:
// they change the instant the working tree does. What pulling misses is new
// manifest entries and newly added skills, which have no link yet — hence the
// re-apply, which is a no-op for everything already in place.
func runUpdate(repo, home string, dryRun, forceCopy bool, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), update.DefaultTimeout)
	defer cancel()

	git := update.NewGit(repo)
	if dryRun {
		return previewUpdate(ctx, git, repo, out)
	}
	status, err := update.Pull(ctx, git)
	if err != nil {
		return err
	}

	switch {
	case status.Updated:
		fmt.Fprintf(out, "Updated %s → %s (%d commit(s))\n", status.Current, status.Latest, status.Behind)
	default:
		fmt.Fprintf(out, "Already up to date (%s)\n", status.Current)
	}

	// Re-apply regardless of whether anything was pulled: a previous run may
	// have been interrupted, or a skill added by hand may still be unlinked.
	_, p, pf, err := buildPlan(repo, home)
	if err != nil {
		return err
	}
	symlinks := symlinkMode(pf, home, forceCopy)
	runner := install.New(install.Options{Home: home, Symlinks: symlinks, DryRun: dryRun})

	var changed, failed int
	for _, a := range p.SelectedActions() {
		res, err := runner.Apply(a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-44s FAILED: %v\n", a.Label, err)
			failed++
			continue
		}
		if res.Status != install.StatusAlready {
			fmt.Fprintf(out, "  %-44s %s\n", a.Label, res.Status)
			changed++
		}
	}

	if ferr := runner.Finish(); ferr != nil {
		return ferr
	}
	if changed == 0 && failed == 0 {
		fmt.Fprintln(out, "Everything already linked.")
	}
	if failed > 0 {
		return fmt.Errorf("%d action(s) failed", failed)
	}
	return nil
}

// previewUpdate is `aval update --dry-run`: what the pull would bring in, and
// which manifest entries it would add or remove, with nothing changed. Like
// the real update it refuses a dirty tree.
func previewUpdate(ctx context.Context, g update.Git, repo string, out io.Writer) error {
	var m *manifest.Manifest
	if loaded, err := manifest.Load(filepath.Join(repo, "install.manifest")); err == nil {
		m = loaded
	}
	paths := update.ManagedPaths(m)

	p, err := update.DryRun(ctx, g, paths)
	if err != nil {
		return err
	}
	if p.Behind == 0 {
		fmt.Fprintf(out, "Already up to date (%s)\n\nDry run: nothing was changed.\n", p.Current)
		return nil
	}
	fmt.Fprintf(out, "Would update %s → %s (%d commit(s))\n", p.Current, p.Latest, p.Behind)
	if p.Stat != "" {
		fmt.Fprintf(out, "\nManaged paths that change:\n%s\n", p.Stat)
	} else {
		fmt.Fprintln(out, "\nNo managed path changes.")
	}
	for _, a := range p.Added {
		fmt.Fprintf(out, "  + manifest entry %s\n", a)
	}
	for _, r := range p.Removed {
		fmt.Fprintf(out, "  - manifest entry %s\n", r)
	}
	fmt.Fprintln(out, "\nDry run: nothing was changed.")
	return nil
}

// fixedRepo is a resolver for a repository that is already known.
func fixedRepo(repo string) func() (string, error) {
	return func() (string, error) { return repo, nil }
}
