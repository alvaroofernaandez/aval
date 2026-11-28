package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// installOpts are the flags install understands. They are accepted after the
// subcommand (`aval install --yes`) and, for scripts written before install
// was a subcommand, as global flags (`aval --yes`).
type installOpts struct{ dryRun, yes, copy bool }

type command struct{ name, args, group string }

// usageGroups is the order of the headings in the usage text.
var usageGroups = []string{"Setup", "Health", "Undo", "Agent tooling", "Plumbing"}

// commands is the single list of subcommands: the usage text is rendered from
// it and anything not in it is rejected before the repository is looked up.
var commands = []command{
	{"install", "[--dry-run] [--yes] [--copy]", "Setup"},
	{"update", "[--dry-run]", "Setup"},
	{"doctor", "[--fix] [--json]", "Health"},
	{"sync", "[--dry-run]", "Setup"},
	{"rules", "sync [--check]", "Agent tooling"},
	{"budget", "[--json]", "Health"},
	{"deps", "[--json] [--layer harness|agent|env]", "Health"},
	{"review", "assess [--base] [--json]", "Agent tooling"},
	{"design", "<gate|ack> ...", "Agent tooling"},
	{"config", "[list | get <key> | set <key> <value> | path]", "Setup"},
	{"restore", "[--list | latest | <id> | --pin <id>]", "Undo"},
	{"generations", "<build | list | diff <a> <b>> [--json]", "Undo"},
	{"uninstall", "[--dry-run] [--yes]", "Undo"},
	{"dev", "[run | diff | reset | ls | rm | project]", "Setup"},
	{"hook", "<lessons|design-gate|pre-push|...>", "Plumbing"},
	{"statusline", "", "Plumbing"},
	{"relay", "<subcommand>", "Plumbing"},
	{"context", "floor [--session <id>] [--project-dir <dir>]", "Health"},
	{"tmux", "<helper>", "Plumbing"},
	{"macos", "<helper>", "Plumbing"},
	{"reap-orphan-tests", "[--min-age] [--dry-run]", "Plumbing"},
	{"kiri", "up", "Plumbing"},
	{"docker", "cleanup [--dry-run]", "Plumbing"},
	{"deck", "<build|check|example> ...", "Agent tooling"},
	{"pdf", "<init|assets> ...", "Agent tooling"},
	{"release", "<status|next|patch|minor|major|rc|beta|promote>", "Agent tooling"},
	{"version", "", "Health"},
	{"help", "", "Health"},
}

func knownCommand(name string) bool {
	for _, c := range commands {
		if c.name == name {
			return true
		}
	}
	return false
}

// printUsage renders the usage for this build in the language the environment
// and the config file select, noting a missing config.
func printUsage(w io.Writer, getenv func(string) string) { printUsageFor(w, getenv, "") }

// printUsageFor is printUsage with the --lang value: a flag that names no
// language is skipped, since the usage is also what a bad flag prints.
func printUsageFor(w io.Writer, getenv func(string) string, langFlag string) {
	renderUsage(w, i18n.New(usageLang(getenv, langFlag)), currentBuild().Version, configExists(getenv))
}

// usageLang resolves the language without a warning for a bad config value:
// the usage must print wherever the person is stuck.
func usageLang(getenv func(string) string, langFlag string) i18n.Lang {
	if config.Validate("language", langFlag) != "" {
		langFlag = ""
	}
	f, _, _ := config.Load("")
	if home, err := userHomeDir(getenv); err == nil {
		if loaded, _, err := config.Load(config.Path(getenv, home, runtime.GOOS)); err == nil {
			f = loaded
		}
	}
	return resolveLang(f, langFlag, getenv)
}

// configExists reports whether aval's config file is on disk.
func configExists(getenv func(string) string) bool {
	home, err := userHomeDir(getenv)
	if err != nil {
		return true // cannot tell, so do not nag
	}
	_, err = os.Stat(config.Path(getenv, home, runtime.GOOS))
	return err == nil
}

// groupLabel is the heading of a usage group in tr's language. The group ids
// in usageGroups stay English: they are keys, not text.
func groupLabel(tr i18n.Translator, group string) string {
	switch group {
	case "Setup":
		return tr.T("cli.usage.group.setup", nil)
	case "Health":
		return tr.T("cli.usage.group.health", nil)
	case "Undo":
		return tr.T("cli.usage.group.undo", nil)
	case "Agent tooling":
		return tr.T("cli.usage.group.agent", nil)
	case "Plumbing":
		return tr.T("cli.usage.group.plumb", nil)
	}
	return group
}

// commandSummaries maps a command to the catalog key of its one-line
// description. The values are i18n.Key literals so the catalog tests count
// them as uses.
var commandSummaries = map[string]string{
	"install":           i18n.Key("cli.usage.cmd.install"),
	"update":            i18n.Key("cli.usage.cmd.update"),
	"doctor":            i18n.Key("cli.usage.cmd.doctor"),
	"sync":              i18n.Key("cli.usage.cmd.sync"),
	"rules":             i18n.Key("cli.usage.cmd.rules"),
	"budget":            i18n.Key("cli.usage.cmd.budget"),
	"deps":              i18n.Key("cli.usage.cmd.deps"),
	"relay":             i18n.Key("cli.usage.cmd.relay"),
	"context":           i18n.Key("cli.usage.cmd.context"),
	"review":            i18n.Key("cli.usage.cmd.review"),
	"design":            i18n.Key("cli.usage.cmd.design"),
	"config":            i18n.Key("cli.usage.cmd.config"),
	"restore":           i18n.Key("cli.usage.cmd.restore"),
	"generations":       i18n.Key("cli.usage.cmd.generations"),
	"uninstall":         i18n.Key("cli.usage.cmd.uninstall"),
	"dev":               i18n.Key("cli.usage.cmd.dev"),
	"hook":              i18n.Key("cli.usage.cmd.hook"),
	"statusline":        i18n.Key("cli.usage.cmd.statusline"),
	"tmux":              i18n.Key("cli.usage.cmd.tmux"),
	"macos":             i18n.Key("cli.usage.cmd.macos"),
	"reap-orphan-tests": i18n.Key("cli.usage.cmd.reap"),
	"kiri":              i18n.Key("cli.usage.cmd.kiri"),
	"docker":            i18n.Key("cli.usage.cmd.docker"),
	"deck":              i18n.Key("cli.usage.cmd.deck"),
	"pdf":               i18n.Key("cli.usage.cmd.pdf"),
	"release":           i18n.Key("cli.usage.cmd.release"),
	"version":           i18n.Key("cli.usage.cmd.version"),
	"help":              i18n.Key("cli.usage.cmd.help"),
}

// commandSummary is the one-line description of a command in tr's language.
func commandSummary(tr i18n.Translator, name string) string {
	key, ok := commandSummaries[name]
	if !ok {
		return ""
	}
	return tr.T(key, nil)
}

// renderUsage writes the plain-text usage. configured is false on a first
// run, which adds the install hint at the end.
func renderUsage(w io.Writer, tr i18n.Translator, version string, configured bool) {
	fmt.Fprintf(w, "aval %s\n%s\n\n", version, tr.T("cli.usage.tagline", nil))
	fmt.Fprintf(w, "%s\n\n", tr.T("cli.usage.about", nil))
	fmt.Fprintf(w, "%s\n  %s\n", tr.T("cli.usage.heading", nil), tr.T("cli.usage.invocation", nil))
	width := 0
	for _, c := range commands {
		width = max(width, len(c.name))
	}
	for _, g := range usageGroups {
		fmt.Fprintf(w, "\n%s\n", groupLabel(tr, g))
		if g == "Plumbing" {
			fmt.Fprintf(w, "  %s\n", tr.T("cli.usage.plumbing", nil))
		}
		for _, c := range commands {
			if c.group != g {
				continue
			}
			fmt.Fprintf(w, "  aval %-*s  %s\n", width, c.name, commandSummary(tr, c.name))
			if c.args != "" {
				fmt.Fprintf(w, "       %-*s    %s\n", width, "", c.args)
			}
		}
	}
	fmt.Fprintf(w, "\n%s\n", tr.T("cli.usage.flags", nil))
	for _, f := range []struct{ flag, desc string }{
		{"--repo <path>", tr.T("cli.usage.flag.repo", nil)},
		{"--theme <name>", tr.T("cli.usage.flag.theme", nil)},
		{"--plain", tr.T("cli.usage.flag.plain", nil)},
		{"--lang <code>", tr.T("cli.usage.flag.lang", nil)},
	} {
		fmt.Fprintf(w, "  %-15s %s\n", f.flag, f.desc)
	}
	fmt.Fprintf(w, "\n%s\n\n%s\n", tr.T("cli.usage.settings", nil), tr.T("cli.usage.repo", nil))
	if !configured {
		fmt.Fprintf(w, "\n%s\n", tr.T("cli.usage.first_run", nil))
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
		return base, &usageError{msg: usage + ": " + err.Error(), err: err}
	}
	if fs.NArg() > 0 {
		return base, newUsage(fmt.Sprintf("%s: unexpected argument %q", usage, strings.Join(fs.Args(), " ")))
	}
	return o, nil
}
