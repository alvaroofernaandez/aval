package main

import (
	"fmt"
	"io"

	"github.com/alvaroofernaandez/aval/internal/rules"
)

// runRules is `aval rules sync [--check]`: mirror the blocking agent rules and
// the scoped lessons from CLAUDE.md into OpenCode's AGENTS.md.
func runRules(repo string, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "sync" {
		return fmt.Errorf("usage: aval rules sync [--check]")
	}
	fs := newFlagSet("rules sync", out)
	check := fs.Bool("check", false, "read-only: exit 1 if AGENTS.md has drifted from CLAUDE.md")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	res, err := rules.Sync(repo, *check)
	if err != nil {
		return err
	}
	if *check {
		if res.Changed {
			fmt.Fprintln(out, "rules sync: drift: config/opencode/AGENTS.md differs from config/claude/CLAUDE.md (run: aval rules sync)")
			return errChecksFailed
		}
		fmt.Fprintln(out, "rules sync: AGENTS.md is in sync")
		return nil
	}
	fmt.Fprintf(out, "rules sync: %d lines mirrored into AGENTS.md\n", res.Lines)
	return nil
}
