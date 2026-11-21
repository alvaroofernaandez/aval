package main

import (
	"fmt"
	"io"
	"time"

	"github.com/alvaroofernaandez/aval/internal/agent/budget"
	"github.com/alvaroofernaandez/aval/internal/agent/review"
)

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
