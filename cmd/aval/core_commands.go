package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
	"github.com/alvaroofernaandez/aval/internal/core/txsync"
	"github.com/alvaroofernaandez/aval/internal/core/uninstall"
)

// errChecksFailed makes main exit 1 without printing "error:" — the doctor
// already printed what failed.
var errChecksFailed = errors.New("checks failed")

func newFlagSet(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	return fs
}

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

func short(home, p string) string {
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return p
}

// runDoctor is `aval doctor [--fix] [--json]`: offline, read-only unless --fix.
func runDoctor(repo, home string, args []string, out io.Writer, ui uiSettings) error {
	fs := newFlagSet("doctor", out)
	fix := fs.Bool("fix", false, "remove broken links and run sync")
	asJSON := fs.Bool("json", false, "print the results as a JSON array")
	security := fs.Bool("security", false, "also run the security checks")
	var allow stringList
	fs.Var(&allow, "allow", "suppress a security check id (`id[:subject]`, repeatable); also read from the overlay's doctor-allow")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opt := doctor.Options{
		Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: runtime.GOOS,
		Security: *security, Allow: allow,
		ConfigPath: config.Path(os.Getenv, home, runtime.GOOS),
	}

	if *fix {
		removed, err := doctor.Fix(opt)
		for _, p := range removed {
			fmt.Fprintf(out, "removed broken link %s\n", short(home, p))
		}
		if len(removed) > 0 || err == nil {
			fmt.Fprintln(out, "synced settings and agent rules")
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(out)
	}

	results := doctor.Run(opt)
	if *asJSON {
		if err := doctor.RenderJSON(out, results); err != nil {
			return err
		}
	} else {
		doctor.RenderStyled(out, results, newPrinterFor(out, ui))
	}
	if doctor.HasFail(results) {
		return errChecksFailed
	}
	return nil
}

// runSync is `aval sync [--dry-run]`. dryRun is the global flag, so both
// `aval --dry-run sync` and `aval sync --dry-run` work.
func runSync(repo, home string, args []string, dryRun bool, out io.Writer) error {
	fs := newFlagSet("sync", out)
	dry := fs.Bool("dry-run", dryRun, "show what would change, write nothing")
	check := fs.Bool("check", false, "read-only: exit 1 if sync would change anything")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *check {
		return checkSync(repo, home, out)
	}

	res, err := txsync.Run(txsync.Options{
		Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: runtime.GOOS, DryRun: *dry,
	})
	if res != nil {
		for _, id := range res.Recovered {
			fmt.Fprintf(out, "recovered backup %s: a previous sync was interrupted and has been restored\n", id)
		}
		if res.Report != nil {
			printSync(out, home, res.Report, *dry)
		}
		if res.RolledBack {
			fmt.Fprintf(out, "\nsync rolled back; backup %s is marked failed (aval restore --list)\n", res.RunID)
		}
	}
	return err
}

// runRestore is `aval restore [--list|latest|<id>] [--pin <id>]`.
func runRestore(repo, home string, args []string, dryRun bool, out io.Writer) error {
	fs := newFlagSet("restore", out)
	list := fs.Bool("list", false, "list backup runs")
	pin := fs.String("pin", "", "protect a backup run from retention")
	unpin := fs.String("unpin", "", "lift the protection from a backup run")
	dry := fs.Bool("dry-run", dryRun, "show what would be restored, write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store := backup.NewStore(home)

	switch {
	case *pin != "" || *unpin != "":
		id, on := *pin, true
		if *unpin != "" {
			id, on = *unpin, false
		}
		run, err := store.Resolve(id)
		if err != nil {
			return err
		}
		if err := store.Pin(run.ID, on); err != nil {
			return err
		}
		verb := "pinned"
		if !on {
			verb = "unpinned"
		}
		fmt.Fprintf(out, "%s %s\n", verb, run.ID)
		return nil
	case *list || fs.NArg() == 0:
		return printBackups(out, store)
	}

	run, err := store.Resolve(fs.Arg(0))
	if err != nil {
		return err
	}
	actions, err := store.Restore(run, backup.RestoreOptions{Repo: repo, DryRun: *dry})
	for _, a := range actions {
		fmt.Fprintf(out, "%-22s %s\n", a.Action, short(home, a.Path))
	}
	if err != nil {
		return err
	}
	if *dry {
		fmt.Fprintln(out, "\nDry run: nothing was written.")
	} else {
		fmt.Fprintf(out, "\nrestored backup %s\n", run.ID)
	}
	return nil
}

// runUninstall is `aval uninstall [--dry-run] [--yes]`.
func runUninstall(repo, home string, args []string, dryRun, yes bool, in io.Reader, out io.Writer) error {
	fs := newFlagSet("uninstall", out)
	dry := fs.Bool("dry-run", dryRun, "show what would be removed, write nothing")
	assume := fs.Bool("yes", yes, "do not ask for confirmation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opt := uninstall.Options{Repo: repo, Home: home, GOOS: runtime.GOOS}

	// Always preview first: the prompt is only meaningful if it shows the plan.
	preview := opt
	preview.DryRun = true
	items, err := uninstall.Run(preview)
	if err != nil {
		return err
	}
	printUninstall(out, home, items)
	if *dry {
		fmt.Fprintln(out, "\nDry run: nothing was written.")
		return nil
	}
	if len(items) == 0 {
		return nil
	}
	if !*assume {
		fmt.Fprint(out, "\nProceed? [y/N] ")
		line, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Fprintln(out, "aborted, nothing was changed")
			return nil
		}
	}

	items, err = uninstall.Run(opt)
	fmt.Fprintln(out)
	printUninstall(out, home, items)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\nundo with: aval restore latest")
	return nil
}

func printUninstall(out io.Writer, home string, items []uninstall.Item) {
	if len(items) == 0 {
		fmt.Fprintln(out, "nothing to uninstall: no manifest link points into this repo")
		return
	}
	for _, it := range items {
		fmt.Fprintf(out, "%-60s %s\n", short(home, it.Path), it.Action)
	}
}

func printBackups(out io.Writer, store *backup.Store) error {
	runs, err := store.List()
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Fprintln(out, "no backups yet")
		return nil
	}
	fmt.Fprintf(out, "%-18s  %-16s  %-7s  %-6s  %s\n", "ID", "DATE", "ENTRIES", "PINNED", "STATE")
	for _, r := range runs {
		pinned, state := "no", "ok"
		if r.Pinned {
			pinned = "yes"
		}
		switch {
		case r.Pending:
			state = "pending (interrupted?)"
		case r.Failed:
			state = "failed: " + r.Reason
		case r.Legacy:
			state = "legacy (no manifest, restore by hand)"
		}
		fmt.Fprintf(out, "%-18s  %-16s  %-7d  %-6s  %s\n", r.ID, r.Time.Format("2006-01-02 15:04"), r.Entries, pinned, state)
	}
	return nil
}

func printSync(out io.Writer, home string, rep *avalsync.Report, dry bool) {
	s := rep.Settings
	switch {
	case len(s.Changed) == 0 && !s.Created:
		fmt.Fprintln(out, "settings  ~/.claude/settings.json is up to date")
	case dry:
		fmt.Fprintf(out, "settings  would change %s\n", strings.Join(s.Changed, ", "))
	default:
		fmt.Fprintf(out, "settings  updated %s\n", strings.Join(s.Changed, ", "))
		if s.Backup != "" {
			fmt.Fprintf(out, "          previous file saved to %s\n", short(home, s.Backup))
		}
	}
	for _, w := range s.Warnings {
		fmt.Fprintf(out, "warning   %s\n", w)
	}
	for _, r := range rep.Rules {
		switch {
		case r.Skipped:
			fmt.Fprintf(out, "rules     %s skipped (agent not installed)\n", short(home, r.Path))
		case !r.Changed:
			fmt.Fprintf(out, "rules     %s is up to date\n", short(home, r.Path))
		case dry:
			fmt.Fprintf(out, "rules     %s would be updated\n", short(home, r.Path))
		default:
			fmt.Fprintf(out, "rules     %s updated\n", short(home, r.Path))
		}
	}
	for _, l := range rep.Links {
		fmt.Fprintf(out, "overlay   %s %s\n", short(home, l.Dest), l.Status)
	}
	if dry {
		fmt.Fprintln(out, "\nDry run: nothing was written.")
	}
}

// checkSync is `aval sync --check`: a forced dry run that prints only what
// drifted and fails when anything did.
func checkSync(repo, home string, out io.Writer) error {
	rep, err := avalsync.Run(avalsync.Options{
		Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), DryRun: true,
	})
	if err != nil {
		return err
	}
	var drift []string
	if s := rep.Settings; s.Created || len(s.Changed) > 0 {
		keys := strings.Join(s.Changed, ", ")
		if s.Created {
			keys = "file missing; " + keys
		}
		drift = append(drift, "settings  ~/.claude/settings.json would change: "+keys)
	}
	for _, r := range rep.Rules {
		if !r.Skipped && r.Changed {
			drift = append(drift, "rules     "+short(home, r.Path)+" is out of date")
		}
	}
	for _, l := range rep.Links {
		if l.Status == "would link" {
			drift = append(drift, "overlay   "+short(home, l.Dest)+" is not linked")
		}
	}
	if len(drift) == 0 {
		fmt.Fprintln(out, "sync: no drift")
		return nil
	}
	for _, d := range drift {
		fmt.Fprintln(out, "drift  "+d)
	}
	fmt.Fprintln(out, "\nRun `aval sync` to apply.")
	return errChecksFailed
}
