package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/doctor"
	avalsync "github.com/alvaroofernaandez/aval/internal/sync"
)

// errChecksFailed makes main exit 1 without printing "error:" — the doctor
// already printed what failed.
var errChecksFailed = errors.New("checks failed")

func newFlagSet(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	return fs
}

func short(home, p string) string {
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return p
}

// runDoctor is `aval doctor [--fix] [--json]`: offline, read-only unless --fix.
func runDoctor(repo, home string, args []string, out io.Writer) error {
	fs := newFlagSet("doctor", out)
	fix := fs.Bool("fix", false, "remove broken links and run sync")
	asJSON := fs.Bool("json", false, "print the results as a JSON array")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opt := doctor.Options{Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: runtime.GOOS}

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
		doctor.Render(out, results)
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
	if err := fs.Parse(args); err != nil {
		return err
	}

	rep, err := avalsync.Run(avalsync.Options{
		Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), DryRun: *dry,
	})
	if rep != nil {
		printSync(out, home, rep, *dry)
	}
	return err
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
		fmt.Fprintln(out, "\nDry run — nothing was written.")
	}
}
