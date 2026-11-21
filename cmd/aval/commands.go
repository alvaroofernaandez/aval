package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/budget"
	"github.com/alvaroofernaandez/aval/internal/doctor"
	"github.com/alvaroofernaandez/aval/internal/review"
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

// runBudget is `aval budget [--json] [--skip-hooks] [--runs N]`: always-on
// context per agent plus hook latency against its budget. Exit 1 when a
// synchronous PreToolUse/PostToolUse hook is over budget.
func runBudget(home string, args []string, out io.Writer) error {
	fs := newFlagSet("budget", out)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	skip := fs.Bool("skip-hooks", false, "measure context only; do not execute any hook")
	runs := fs.Int("runs", 5, "executions per hook")
	hookBudget := fs.Duration("hook-budget", 20*time.Millisecond, "p95 ceiling for synchronous Pre/PostToolUse hooks")
	timeout := fs.Duration("timeout", 2*time.Second, "per-execution timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rep, err := budget.Run(budget.Options{
		Home: home, Runs: *runs, Timeout: *timeout, HookBudget: *hookBudget, SkipHooks: *skip,
	})
	if err != nil {
		return err
	}
	if *asJSON {
		if err := budget.RenderJSON(out, rep); err != nil {
			return err
		}
	} else {
		budget.Render(out, rep)
	}
	if rep.Over {
		return errChecksFailed
	}
	return nil
}

// runReview is `aval review assess [--base <ref>] [--json]`: a git-only risk
// tier and the reviewer lenses worth running for it. It always exits 0 when
// it can assess; the tier is information, not a gate.
func runReview(dir string, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "assess" {
		return fmt.Errorf("usage: aval review assess [--base <ref>] [--json]")
	}
	fs := newFlagSet("review assess", out)
	base := fs.String("base", "", "ref to compare against (default: main, master, origin/main, origin/master, else HEAD)")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	res, err := review.Assess(dir, *base)
	if err != nil {
		return err
	}
	if *asJSON {
		return review.RenderJSON(out, res)
	}
	review.Render(out, res)
	return nil
}
