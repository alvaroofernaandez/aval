package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/config"
)

// installOpts are the flags install understands. They are accepted after the
// subcommand (`aval install --yes`) and, for scripts written before install
// was a subcommand, as global flags (`aval --yes`).
type installOpts struct{ dryRun, yes, copy bool }

type command struct{ name, args, summary, group string }

// usageGroups is the order of the headings in the usage text.
var usageGroups = []string{"Setup", "Health", "Undo", "Agent tooling", "Plumbing"}

// commands is the single list of subcommands: the usage text is rendered from
// it and anything not in it is rejected before the repository is looked up.
var commands = []command{
	{"install", "[--dry-run] [--yes] [--copy]", "link the repo's configs into HOME (TUI; --yes for none)", "Setup"},
	{"update", "[--dry-run]", "pull the repo, then re-apply", "Setup"},
	{"doctor", "[--fix] [--json]", "check this machine against the repo", "Health"},
	{"sync", "[--dry-run]", "merge repo-managed settings into the live config", "Setup"},
	{"rules", "sync [--check]", "mirror CLAUDE.md's shared rules into OpenCode's AGENTS.md", "Agent tooling"},
	{"budget", "[--json]", "always-on context per agent and hook latency", "Health"},
	{"review", "assess [--base] [--json]", "git-only risk tier and reviewer lenses", "Agent tooling"},
	{"design", "<gate|ack> ...", "design gates for the project in the current directory", "Agent tooling"},
	{"config", "[list | get <key> | set <key> <value> | path]", "show or change aval settings (theme, glyphs, motion)", "Setup"},
	{"restore", "[--list | latest | <id> | --pin <id>]", "list or restore backup runs", "Undo"},
	{"uninstall", "[--dry-run] [--yes]", "remove aval's links and restore what they displaced", "Undo"},
	{"dev", "[run | diff | reset | ls | rm | project]", "sandboxed HOME for developing aval", "Setup"},
	{"hook", "<lessons|design-gate|pre-push|...>", "agent hooks and the git pre-push gate", "Plumbing"},
	{"statusline", "", "Claude Code status line", "Plumbing"},
	{"tmux", "<helper>", "tmux, Ghostty and zsh helpers", "Plumbing"},
	{"macos", "<helper>", "macOS defaults and LaunchAgents", "Plumbing"},
	{"reap-orphan-tests", "[--min-age] [--dry-run]", "end orphaned test processes", "Plumbing"},
	{"kiri", "up", "make Docker ready on Kiri", "Plumbing"},
	{"docker", "cleanup [--dry-run]", "safe Docker prunes on the kiri context", "Plumbing"},
	{"deck", "<build|check|example> ...", "turn a plan into a PDF slide deck, rendered locally", "Agent tooling"},
	{"pdf", "<init|assets> ...", "scaffold PDF report projects and their brand assets", "Agent tooling"},
	{"release", "<status|next|patch|minor|major|rc|beta|promote>", "cut a release", "Agent tooling"},
	{"version", "", "version, commit, build date and channel", "Health"},
	{"help", "", "show this help", "Health"},
}

func knownCommand(name string) bool {
	for _, c := range commands {
		if c.name == name {
			return true
		}
	}
	return false
}

// printUsage renders the usage for this build, noting a missing config.
func printUsage(w io.Writer) {
	renderUsage(w, currentBuild().Version, configExists())
}

// configExists reports whether aval's config file is on disk.
func configExists() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return true // cannot tell, so do not nag
	}
	_, err = os.Stat(config.Path(os.Getenv, home, runtime.GOOS))
	return err == nil
}

// renderUsage writes the plain-text usage. configured is false on a first
// run, which adds the install hint at the end.
func renderUsage(w io.Writer, version string, configured bool) {
	fmt.Fprintf(w, "aval %s\nNothing passes without evidence.\n\n", version)
	fmt.Fprint(w, "aval installs and maintains this machine's dotfiles and agent configs.\n\n")
	fmt.Fprint(w, "Usage:\n  aval <command> [flags]\n")
	width := 0
	for _, c := range commands {
		width = max(width, len(c.name))
	}
	for _, g := range usageGroups {
		fmt.Fprintf(w, "\n%s\n", g)
		if g == "Plumbing" {
			fmt.Fprint(w, "  Called by hooks and scripts, rarely by hand.\n")
		}
		for _, c := range commands {
			if c.group != g {
				continue
			}
			fmt.Fprintf(w, "  aval %-*s  %s\n", width, c.name, c.summary)
			if c.args != "" {
				fmt.Fprintf(w, "       %-*s    %s\n", width, "", c.args)
			}
		}
	}
	fmt.Fprint(w, `
Global flags:
  --repo <path>   the aval repository to use
  --theme <name>  colour theme for this run (aval, terminal, mono, catppuccin, ...)
  --plain         monochrome, ASCII glyphs and no motion

Settings: theme, background, glyphs, motion, update_check. Precedence is flag,
then AVAL_* environment, then ~/.config/aval/config. NO_COLOR turns colour off.

The repository is found from, in order: --repo, $AVAL_REPO, the location
'aval install' recorded in ~/.config/aval/config, then the current directory.
First time: clone the repo, cd into it and run 'aval install'.
`)
	if !configured {
		fmt.Fprint(w, "\nNot set up yet. Run: aval install\n")
	}
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
