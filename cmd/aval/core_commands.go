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
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/core/deps"
	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
	"github.com/alvaroofernaandez/aval/internal/core/txsync"
	"github.com/alvaroofernaandez/aval/internal/core/uninstall"
	"github.com/alvaroofernaandez/aval/internal/i18n"
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

	tr := i18n.New(ui.Lang)
	opt := doctor.Options{
		Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: runtime.GOOS,
		Security: *security, Allow: allow,
		ConfigPath: config.Path(os.Getenv, home, runtime.GOOS),
		DepsProber: deps.SystemProber{GOOS: runtime.GOOS, Getenv: os.Getenv, NoVersion: true},
	}

	if *fix {
		removed, err := doctor.Fix(opt)
		for _, p := range removed {
			fmt.Fprintln(out, tr.T("cli.doctor.removed", i18n.Args{"path": short(home, p)}))
		}
		if len(removed) > 0 || err == nil {
			fmt.Fprintln(out, tr.T("cli.doctor.synced", nil))
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
		doctor.RenderText(out, results, newPrinterFor(out, ui), doctorText(tr))
	}
	if doctor.HasFail(results) {
		return errChecksFailed
	}
	return nil
}

// runSync is `aval sync [--dry-run]`. dryRun is the global flag, so both
// `aval --dry-run sync` and `aval sync --dry-run` work.
func runSync(repo, home string, args []string, dryRun bool, out io.Writer, tr i18n.Translator) error {
	fs := newFlagSet("sync", out)
	dry := fs.Bool("dry-run", dryRun, "show what would change, write nothing")
	check := fs.Bool("check", false, "read-only: exit 1 if sync would change anything")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *check {
		return checkSync(repo, home, out, tr)
	}

	res, err := txsync.Run(txsync.Options{
		Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: runtime.GOOS, DryRun: *dry,
	})
	if res != nil {
		for _, id := range res.Recovered {
			fmt.Fprintln(out, tr.T("cli.sync.recovered", i18n.Args{"id": id}))
		}
		if res.Report != nil {
			printSync(out, home, res.Report, *dry, tr)
		}
		if res.RolledBack {
			fmt.Fprintf(out, "\n%s\n", tr.T("cli.sync.rolled_back", i18n.Args{"id": res.RunID}))
		}
	}
	return err
}

// runRestore is `aval restore [--list|latest|<id>] [--pin <id>]`.
func runRestore(repo, home string, args []string, dryRun bool, out io.Writer, tr i18n.Translator) error {
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
		msg := tr.T("cli.restore.pinned", i18n.Args{"id": run.ID})
		if !on {
			msg = tr.T("cli.restore.unpinned", i18n.Args{"id": run.ID})
		}
		fmt.Fprintln(out, msg)
		return nil
	case *list || fs.NArg() == 0:
		return printBackups(out, store, tr)
	}

	run, err := store.Resolve(fs.Arg(0))
	if err != nil {
		return err
	}
	actions, err := store.Restore(run, backup.RestoreOptions{Repo: repo, DryRun: *dry})
	for _, a := range actions {
		fmt.Fprintf(out, "%-22s %s\n", restoreActionWord(tr, a.Action), short(home, a.Path))
	}
	if err != nil {
		return err
	}
	if *dry {
		fmt.Fprintf(out, "\n%s\n", tr.T("cli.dry_run", nil))
	} else {
		fmt.Fprintf(out, "\n%s\n", tr.T("cli.restore.done", i18n.Args{"id": run.ID}))
	}
	return nil
}

// restoreActionWord translates the step words the backup package returns. A
// word the catalog does not know is printed as it came.
func restoreActionWord(tr i18n.Translator, word string) string {
	switch word {
	case "already gone":
		return tr.T("cli.restore.action.gone", nil)
	case "would remove if empty":
		return tr.T("cli.restore.action.remove_empty", nil)
	case "kept (not empty)":
		return tr.T("cli.restore.action.kept", nil)
	case "removed (empty dir)":
		return tr.T("cli.restore.action.removed_dir", nil)
	case "unchanged":
		return tr.T("cli.restore.action.unchanged", nil)
	case "would restore":
		return tr.T("cli.restore.action.would", nil)
	case "restored":
		return tr.T("cli.restore.action.restored", nil)
	case "would remove":
		return tr.T("cli.restore.action.would_remove", nil)
	case "removed":
		return tr.T("cli.restore.action.removed", nil)
	}
	return word
}

// runUninstall is `aval uninstall [--dry-run] [--yes]`.
func runUninstall(repo, home string, args []string, dryRun, yes bool, in io.Reader, out io.Writer, tr i18n.Translator) error {
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
	printUninstall(out, home, items, tr)
	if *dry {
		fmt.Fprintf(out, "\n%s\n", tr.T("cli.dry_run", nil))
		return nil
	}
	if len(items) == 0 {
		return nil
	}
	if !*assume {
		fmt.Fprint(out, "\n"+tr.T("cli.uninstall.prompt", nil))
		line, _ := bufio.NewReader(in).ReadString('\n')
		if !saysYes(tr, line) {
			fmt.Fprintln(out, tr.T("cli.uninstall.aborted", nil))
			return nil
		}
	}

	items, err = uninstall.Run(opt)
	fmt.Fprintln(out)
	printUninstall(out, home, items, tr)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%s\n", tr.T("cli.uninstall.undo", nil))
	return nil
}

// saysYes reports whether an answer to a prompt is a yes in tr's language.
func saysYes(tr i18n.Translator, answer string) bool {
	a := strings.ToLower(strings.TrimSpace(answer))
	return a != "" && slices.Contains(strings.Fields(tr.T("cli.uninstall.yes_answers", nil)), a)
}

func printUninstall(out io.Writer, home string, items []uninstall.Item, tr i18n.Translator) {
	if len(items) == 0 {
		fmt.Fprintln(out, tr.T("cli.uninstall.nothing", nil))
		return
	}
	for _, it := range items {
		fmt.Fprintf(out, "%-60s %s\n", short(home, it.Path), uninstallActionWord(tr, it.Action))
	}
}

// uninstallActionWord translates the fixed step words the uninstall package
// returns. A step that carries data (the backup it restores from) is printed
// as it came.
func uninstallActionWord(tr i18n.Translator, word string) string {
	switch word {
	case "would remove link":
		return tr.T("cli.uninstall.action.would_link", nil)
	case "would remove copy":
		return tr.T("cli.uninstall.action.would_copy", nil)
	case "removed link":
		return tr.T("cli.uninstall.action.link", nil)
	case "removed copy":
		return tr.T("cli.uninstall.action.copy", nil)
	case "would remove the aval:agent-rules block":
		return tr.T("cli.uninstall.action.would_block", nil)
	case "removed the aval:agent-rules block":
		return tr.T("cli.uninstall.action.block", nil)
	case "removed empty directory":
		return tr.T("cli.uninstall.action.dir", nil)
	}
	return word
}

func printBackups(out io.Writer, store *backup.Store, tr i18n.Translator) error {
	runs, err := store.List()
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Fprintln(out, tr.T("cli.restore.none", nil))
		return nil
	}
	// The columns grow to their heading, which is longer in some languages.
	entries, pinnedCol := tr.T("cli.restore.col.entries", nil), tr.T("cli.restore.col.pinned", nil)
	wEntries, wPinned := max(7, utf8.RuneCountInString(entries)), max(6, utf8.RuneCountInString(pinnedCol))
	fmt.Fprintf(out, "%-18s  %-16s  %-*s  %-*s  %s\n", tr.T("cli.restore.col.id", nil), tr.T("cli.restore.col.date", nil),
		wEntries, entries, wPinned, pinnedCol, tr.T("cli.restore.col.state", nil))
	for _, r := range runs {
		pinned, state := tr.T("cli.restore.no", nil), tr.T("cli.restore.state.ok", nil)
		if r.Pinned {
			pinned = tr.T("cli.restore.yes", nil)
		}
		switch {
		case r.Pending:
			state = tr.T("cli.restore.state.pending", nil)
		case r.Failed:
			state = tr.T("cli.restore.state.failed", i18n.Args{"reason": r.Reason})
		case r.Legacy:
			state = tr.T("cli.restore.state.legacy", nil)
		}
		fmt.Fprintf(out, "%-18s  %-16s  %-*d  %-*s  %s\n", r.ID, r.Time.Format("2006-01-02 15:04"), wEntries, r.Entries, wPinned, pinned, state)
	}
	return nil
}

// syncLabel is a row label of the sync report, padded to the report's column.
func syncLabel(tr i18n.Translator, label string) string {
	var word string
	switch label {
	case "settings":
		word = tr.T("cli.sync.label.settings", nil)
	case "rules":
		word = tr.T("cli.sync.label.rules", nil)
	case "overlay":
		word = tr.T("cli.sync.label.overlay", nil)
	case "warning":
		word = tr.T("cli.sync.label.warning", nil)
	}
	return fmt.Sprintf("%-10s", word)
}

// overlayStatus translates the link words the sync package returns.
func overlayStatus(tr i18n.Translator, status string) string {
	switch status {
	case "linked":
		return tr.T("cli.sync.overlay.linked", nil)
	case "already linked":
		return tr.T("cli.sync.overlay.already", nil)
	case "would link":
		return tr.T("cli.sync.overlay.would", nil)
	}
	return status
}

func printSync(out io.Writer, home string, rep *avalsync.Report, dry bool, tr i18n.Translator) {
	s := rep.Settings
	settings := syncLabel(tr, "settings")
	keys := i18n.Args{"keys": strings.Join(s.Changed, ", ")}
	switch {
	case len(s.Changed) == 0 && !s.Created:
		fmt.Fprintln(out, settings+tr.T("cli.sync.settings.uptodate", nil))
	case dry:
		fmt.Fprintln(out, settings+tr.T("cli.sync.settings.would", keys))
	default:
		fmt.Fprintln(out, settings+tr.T("cli.sync.settings.updated", keys))
		if s.Backup != "" {
			fmt.Fprintf(out, "%s%s\n", strings.Repeat(" ", 10), tr.T("cli.sync.settings.backup", i18n.Args{"path": short(home, s.Backup)}))
		}
	}
	for _, w := range s.Warnings {
		fmt.Fprintf(out, "%s%s\n", syncLabel(tr, "warning"), w)
	}
	rules := syncLabel(tr, "rules")
	for _, r := range rep.Rules {
		path := i18n.Args{"path": short(home, r.Path)}
		switch {
		case r.Skipped:
			fmt.Fprintln(out, rules+tr.T("cli.sync.rules.skipped", path))
		case !r.Changed:
			fmt.Fprintln(out, rules+tr.T("cli.sync.rules.uptodate", path))
		case dry:
			fmt.Fprintln(out, rules+tr.T("cli.sync.rules.would", path))
		default:
			fmt.Fprintln(out, rules+tr.T("cli.sync.rules.updated", path))
		}
	}
	for _, l := range rep.Links {
		fmt.Fprintf(out, "%s%s %s\n", syncLabel(tr, "overlay"), short(home, l.Dest), overlayStatus(tr, l.Status))
	}
	if dry {
		fmt.Fprintf(out, "\n%s\n", tr.T("cli.dry_run", nil))
	}
}

// checkSync is `aval sync --check`: a forced dry run that prints only what
// drifted and fails when anything did.
func checkSync(repo, home string, out io.Writer, tr i18n.Translator) error {
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
			keys = tr.T("cli.sync.drift.file_missing", i18n.Args{"keys": keys})
		}
		drift = append(drift, syncLabel(tr, "settings")+tr.T("cli.sync.drift.settings", i18n.Args{"keys": keys}))
	}
	for _, r := range rep.Rules {
		if !r.Skipped && r.Changed {
			drift = append(drift, syncLabel(tr, "rules")+tr.T("cli.sync.drift.rules", i18n.Args{"path": short(home, r.Path)}))
		}
	}
	for _, l := range rep.Links {
		if l.Status == "would link" {
			drift = append(drift, syncLabel(tr, "overlay")+tr.T("cli.sync.drift.overlay", i18n.Args{"path": short(home, l.Dest)}))
		}
	}
	if len(drift) == 0 {
		fmt.Fprintln(out, tr.T("cli.sync.no_drift", nil))
		return nil
	}
	for _, d := range drift {
		fmt.Fprintln(out, tr.T("cli.sync.label.drift", nil)+"  "+d)
	}
	fmt.Fprintf(out, "\n%s\n", tr.T("cli.sync.apply", nil))
	return errChecksFailed
}
