package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// installOpts are the flags install understands. They are accepted after the
// subcommand (`aval install --yes`) and, for scripts written before install
// was a subcommand, as global flags (`aval --yes`).
type installOpts struct{ dryRun, yes, copy bool }

type command struct{ name, args, summary string }

// commands is the single list of subcommands: the usage text is rendered from
// it and anything not in it is rejected before the repository is looked up.
var commands = []command{
	{"install", "[--dry-run] [--yes] [--copy]", "link the repo's configs into HOME (TUI; --yes for none)"},
	{"update", "[--dry-run]", "pull the repo, then re-apply"},
	{"doctor", "[--fix] [--json]", "check this machine against the repo"},
	{"sync", "[--dry-run]", "merge repo-managed settings into the live config"},
	{"rules", "sync [--check]", "mirror CLAUDE.md's shared rules into OpenCode's AGENTS.md"},
	{"budget", "[--json]", "always-on context per agent and hook latency"},
	{"review", "assess [--base] [--json]", "git-only risk tier and reviewer lenses"},
	{"design", "<gate|ack> ...", "design gates for the project in the current directory"},
	{"restore", "[--list | latest | <id> | --pin <id>]", "list or restore backup runs"},
	{"uninstall", "[--dry-run] [--yes]", "remove aval's links and restore what they displaced"},
	{"dev", "[run | diff | reset | ls | rm | project]", "sandboxed HOME for developing aval"},
	{"hook", "<lessons|design-gate|pre-push|...>", "agent hooks and the git pre-push gate"},
	{"statusline", "", "Claude Code status line"},
	{"tmux", "<helper>", "tmux, Ghostty and zsh helpers"},
	{"macos", "<helper>", "macOS defaults and LaunchAgents"},
	{"reap-orphan-tests", "[--min-age] [--dry-run]", "end orphaned test processes"},
	{"kiri", "up", "make Docker ready on Kiri"},
	{"docker", "cleanup [--dry-run]", "safe Docker prunes on the kiri context"},
	{"deck", "<build|check|example> ...", "turn a plan into a PDF slide deck, rendered locally"},
	{"pdf", "<init|assets> ...", "scaffold PDF report projects and their brand assets"},
	{"release", "<status|next|patch|minor|major|rc|beta|promote>", "cut a release"},
	{"version", "", "version, commit, build date and channel"},
	{"help", "", "show this help"},
}

func knownCommand(name string) bool {
	for _, c := range commands {
		if c.name == name {
			return true
		}
	}
	return false
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, "aval installs and maintains this machine's dotfiles and agent configs.\n\n")
	fmt.Fprint(w, "Usage:\n  aval <command> [flags]\n\nCommands:\n")
	width := 0
	for _, c := range commands {
		width = max(width, len(c.name))
	}
	for _, c := range commands {
		fmt.Fprintf(w, "  aval %-*s  %s\n", width, c.name, c.summary)
		if c.args != "" {
			fmt.Fprintf(w, "       %-*s    %s\n", width, "", c.args)
		}
	}
	fmt.Fprint(w, `
Global flags:
  --repo <path>  the aval repository to use

The repository is found from, in order: --repo, $AVAL_REPO, the location
'aval install' recorded in ~/.config/aval/config, then the current directory.
First time: clone the repo, cd into it and run 'aval install'.
`)
}

// parseInstallArgs reads install's flags from the arguments after the
// subcommand, on top of any given as global flags.
func parseInstallArgs(base installOpts, args []string) (installOpts, error) {
	o := base
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.dryRun, "dry-run", base.dryRun, "show what would happen, change nothing")
	fs.BoolVar(&o.yes, "yes", base.yes, "install everything applicable without the TUI")
	fs.BoolVar(&o.copy, "copy", base.copy, "copy files instead of symlinking them")

	const usage = "usage: aval install [--dry-run] [--yes] [--copy]"
	if err := fs.Parse(args); err != nil {
		return base, fmt.Errorf("%s: %w", usage, err)
	}
	if fs.NArg() > 0 {
		return base, fmt.Errorf("%s: unexpected argument %q", usage, strings.Join(fs.Args(), " "))
	}
	return o, nil
}
