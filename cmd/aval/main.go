// Command aval installs this repository's configs on macOS,
// Linux and Windows.
//
//	aval            interactive TUI
//	aval --dry-run  show what would happen, change nothing
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

	"github.com/alvaroofernaandez/aval/internal/agent/statusline"
	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/manifest"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/core/platform"
	"github.com/alvaroofernaandez/aval/internal/core/update"
	"github.com/alvaroofernaandez/aval/internal/release"
	"github.com/alvaroofernaandez/aval/internal/ui"
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
		repoFlag  = flag.String("repo", "", "path to the aval repository (default: the binary's repository)")
		showVer   = flag.Bool("version", false, "print the version and exit")
		forceCopy = flag.Bool("copy", false, "copy files instead of symlinking them")
	)
	flag.Parse()

	if *showVer {
		fmt.Print(currentBuild().text(runtime.GOOS, runtime.GOARCH))
		return nil
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
	}

	repo, err := resolveRepo(*repoFlag)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	// doctor and sync are offline and local, so they dispatch before the
	// update notice: a health check must not wait on the network.
	switch flag.Arg(0) {
	case "doctor":
		return runDoctor(repo, home, flag.Args()[1:], os.Stdout)
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

	notifyIfOutdated()

	return runInstall(repo, home, *dryRun, *yes, *forceCopy)
}

// notifyIfOutdated tells the user when a newer release exists.
//
// npm never pushes an update to an installed package, so a globally installed
// copy stays on whatever version it was installed at until someone acts. This
// is that prompt. It is rate-limited to once a day, opt-out-able via
// AVAL_NO_UPDATE_CHECK, and silent on every failure — being offline must
// not stop anyone installing their dotfiles.
func notifyIfOutdated() {
	v := currentBuild().Version
	if release.Channel(v) == release.ChannelDev || !update.ShouldCheck(time.Now()) {
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

// resolveRepo finds the repository. The binary normally lives inside it, so
// walking up from the executable finds it without configuration — but a
// go-installed binary sits in GOPATH/bin, so the working directory and the
// flag are both honoured.
func resolveRepo(flagValue string) (string, error) {
	candidates := []string{}
	if flagValue != "" {
		candidates = append(candidates, flagValue)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), ".."))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}

	for _, c := range candidates {
		if root := walkUpToManifest(c); root != "" {
			return root, nil
		}
	}
	return "", fmt.Errorf("cannot find install.manifest; run from the repository or pass --repo")
}

func walkUpToManifest(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "install.manifest")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
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
// it greyed out instead of silently omitting it.
func unavailableGroups(m *manifest.Manifest) []ui.UnavailableGroup {
	var out []ui.UnavailableGroup
	for _, g := range m.Groups {
		if !g.AppliesTo(runtime.GOOS) {
			out = append(out, ui.UnavailableGroup{
				Label:  g.Label,
				Reason: "not available on Windows natively — use WSL",
			})
		}
	}
	return out
}

func runInstall(repo, home string, dryRun, yes, forceCopy bool) error {
	m, p, pf, err := buildPlan(repo, home)
	if err != nil {
		return err
	}
	if len(p.Groups) == 0 {
		return fmt.Errorf("nothing in the manifest applies to %s", pf.Display())
	}

	symlinks := symlinkMode(pf, home, forceCopy)
	runner := install.New(install.Options{Home: home, Symlinks: symlinks, DryRun: dryRun})

	if yes {
		err := runHeadless(p, pf, runner, symlinks, dryRun)
		if err == nil {
			err = finishInstall(repo, home, dryRun)
		}
		return finish(runner, err)
	}

	model := ui.NewModel(p, pf, runner, symlinks, dryRun)
	model.Unavailable = unavailableGroups(m)

	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if ferr := runner.Finish(); err == nil {
		err = ferr
	}
	if err != nil {
		return err
	}
	if fm, ok := final.(ui.Model); ok && len(fm.Errs()) > 0 {
		return fmt.Errorf("%d action(s) failed", len(fm.Errs()))
	}
	return finishInstall(repo, home, dryRun)
}

// finish closes the runner's backup run without hiding the original error.
func finish(r *install.Runner, err error) error {
	if ferr := r.Finish(); err == nil {
		return ferr
	}
	return err
}

// runHeadless is the provisioning path: no TUI, one line per action, and a
// non-zero exit if anything failed.
func runHeadless(p *plan.Plan, pf platform.Platform, runner *install.Runner, symlinks, dryRun bool) error {
	fmt.Printf("aval %s — %s\n", version, pf.Display())
	fmt.Printf("%s\n\n", pf.InstallStyle(symlinks))

	var failures int
	backups := 0
	for _, a := range p.SelectedActions() {
		res, err := runner.Apply(a)
		if err != nil {
			fmt.Printf("  %-44s %s\n", a.Label, "FAILED: "+err.Error())
			failures++
			continue
		}
		note := ""
		if res.Backup != "" {
			note = "  (backed up)"
			backups++
		}
		fmt.Printf("  %-44s %s%s\n", a.Label, res.Status, note)
	}

	if backups > 0 {
		fmt.Printf("\n%d existing file(s) moved to %s\n", backups, runner.BackupRoot())
	}
	if failures > 0 {
		return fmt.Errorf("%d action(s) failed", failures)
	}
	if dryRun {
		fmt.Println("\nDry run — nothing was written.")
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
	paths := []string{"install.manifest"}
	if m, err := manifest.Load(filepath.Join(repo, "install.manifest")); err == nil {
		seen := map[string]bool{"install.manifest": true}
		for _, grp := range m.Groups {
			srcs := []string{grp.FanoutSource}
			for _, e := range grp.Entries {
				srcs = append(srcs, e.Source)
			}
			for _, s := range srcs {
				if s != "" && !seen[s] {
					seen[s] = true
					paths = append(paths, s)
				}
			}
		}
	}

	p, err := update.DryRun(ctx, g, paths)
	if err != nil {
		return err
	}
	if p.Behind == 0 {
		fmt.Fprintf(out, "Already up to date (%s)\n\nDry run — nothing was changed.\n", p.Current)
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
	fmt.Fprintln(out, "\nDry run — nothing was changed.")
	return nil
}
