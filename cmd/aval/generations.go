package main

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/pi"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
	"github.com/alvaroofernaandez/aval/internal/core/txsync"
)

const generationsUsage = "aval generations <build | list | diff <a> <b> | activate <id> | rollback [<id>] | pin <id> | unpin <id> | prune | consumer <name> [<id>]> [--json]"

// runGenerations is `aval generations`: build an immutable generation of
// what aval renders for every adapter, list the stored ones, diff two.
func runGenerations(sub []string, repoFlag string, c cmdIO) error {
	asJSON := slices.Contains(sub, "--json")
	args := slices.DeleteFunc(slices.Clone(sub), func(a string) bool { return a == "--json" })
	if len(args) == 0 {
		return newUsage(generationsUsage)
	}
	home, err := userHomeDir(c.env)
	if err != nil {
		return err
	}
	cmd, ok := generationsCommands[args[0]]
	if !ok || len(args[1:]) < cmd.min || len(args[1:]) > cmd.max {
		return newUsage(generationsUsage)
	}
	return cmd.run(generationsCall{repo: repoFlag, home: home, args: args[1:], asJSON: asJSON, c: c})
}

// generationsCall is what a generations subcommand receives.
type generationsCall struct {
	repo, home string
	args       []string
	asJSON     bool
	c          cmdIO
}

// generationsCommands maps a subcommand to its arity and handler.
var generationsCommands = map[string]struct {
	min, max int
	run      func(generationsCall) error
}{
	"build":    {0, 0, func(g generationsCall) error { return generationsBuild(g.repo, g.home, g.asJSON, g.c) }},
	"list":     {0, 0, func(g generationsCall) error { return generationsList(g.home, g.asJSON, g.c) }},
	"diff":     {2, 2, func(g generationsCall) error { return generationsDiff(g.home, g.args[0], g.args[1], g.asJSON, g.c) }},
	"activate": {1, 1, func(g generationsCall) error { return generationsActivate(g.repo, g.home, g.args[0], g.asJSON, g.c) }},
	"rollback": {0, 1, func(g generationsCall) error { return generationsRollback(g.repo, g.home, g.args, g.asJSON, g.c) }},
	"pin":      {1, 1, func(g generationsCall) error { return generationsPin(g.home, g.args[0], true, g.c) }},
	"unpin":    {1, 1, func(g generationsCall) error { return generationsPin(g.home, g.args[0], false, g.c) }},
	"prune":    {0, 0, func(g generationsCall) error { return generationsPrune(g.home, g.c) }},
	"consumer": {1, 2, func(g generationsCall) error { return generationsConsumer(g.home, g.args, g.c) }},
}

func generationsList(home string, asJSON bool, c cmdIO) error {
	gs, err := generation.List(home)
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(c.stdout, nonNil(gs))
	}
	return printGenerations(c.stdout, gs)
}

func generationsDiff(home, a, b string, asJSON bool, c cmdIO) error {
	d, err := generation.Diff(home, a, b)
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(c.stdout, nonNil(d))
	}
	return printDiff(c.stdout, d)
}

// generationsRollback re-activates the named generation, or with no id the
// one active before the current one (owner: G7 = A).
func generationsRollback(repoFlag, home string, ids []string, asJSON bool, c cmdIO) error {
	if len(ids) == 1 {
		return generationsActivate(repoFlag, home, ids[0], asJSON, c)
	}
	id, err := generation.Previous(home)
	if err != nil {
		return err
	}
	return generationsActivate(repoFlag, home, id, asJSON, c)
}

func generationsBuild(repoFlag, home string, asJSON bool, c cmdIO) error {
	repo, err := resolveRepoWith(repoFlag, c.env)
	if err != nil {
		return err
	}
	cfg, err := agentcfg.Load(repo)
	if err != nil {
		return err
	}
	g, created, err := generation.Build(generation.Options{
		Home:     home,
		Config:   cfg,
		Adapters: txsync.Adapters(txsync.Options{Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: runtime.GOOS}),
		Source:   repoCommit(repo),
	})
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(c.stdout, struct {
			generation.Generation
			Created bool `json:"created"`
		}{g, created})
	}
	if created {
		_, err = fmt.Fprintf(c.stdout, "built %s\n", g.ID)
	} else {
		_, err = fmt.Fprintf(c.stdout, "%s already built (%s)\n", g.ID, g.Built.Format(time.RFC3339))
	}
	return err
}

// generationsActivate makes generation id live through the sync
// transaction: backup, apply, doctor, restore on failure (owner: G5 = A).
func generationsActivate(repoFlag, home, id string, asJSON bool, c cmdIO) error {
	repo, err := resolveRepoWith(repoFlag, c.env)
	if err != nil {
		return err
	}
	res, err := txsync.Run(txsync.Options{Repo: repo, Home: home, PrivateDir: avalsync.PrivateDir(home), GOOS: runtime.GOOS, Generation: id})
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(c.stdout, struct {
			Generation string   `json:"generation"`
			Backup     string   `json:"backup"`
			Recovered  []string `json:"recovered"`
		}{res.Generation, res.RunID, nonNil(res.Recovered)})
	}
	_, err = fmt.Fprintf(c.stdout, "activated %s (backup %s)\n", res.Generation, res.RunID)
	return err
}

// generationsPin protects a generation from prune, or lifts it.
func generationsPin(home, id string, on bool, c cmdIO) error {
	if err := generation.Pin(home, id, on); err != nil {
		return err
	}
	verb := "pinned"
	if !on {
		verb = "unpinned"
	}
	_, err := fmt.Fprintf(c.stdout, "%s %s\n", verb, id)
	return err
}

// generationsPrune applies the retention rule now.
func generationsPrune(home string, c cmdIO) error {
	removed, err := generation.Prune(home)
	for _, id := range removed {
		fmt.Fprintf(c.stdout, "removed %s\n", id)
	}
	if err == nil && len(removed) == 0 {
		_, err = fmt.Fprintln(c.stdout, "nothing to prune")
	}
	return err
}

// generationsConsumer applies the pi/ tree of a generation (the active one
// by default) into the consumer's own dir and pins the consumer to it.
func generationsConsumer(home string, args []string, c cmdIO) error {
	id := ""
	if len(args) == 2 {
		id = args[1]
	} else if cur, err := generation.Current(home); err != nil {
		return err
	} else if cur == "" {
		return generation.ErrNoActive
	} else {
		id = cur
	}
	target, err := generation.ApplyConsumer(home, args[0], id, pi.ForHome(home))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.stdout, "%s applied to %s\n", id, target)
	return err
}

// repoCommit is the commit the config was read from; empty outside git.
func repoCommit(repo string) string {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func printGenerations(w io.Writer, gs []generation.Generation) error {
	if len(gs) == 0 {
		_, err := fmt.Fprintln(w, "no generations; aval generations build makes one")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tBUILT\tSOURCE\tADAPTERS")
	for _, g := range gs {
		var names []string
		for _, a := range g.Adapters {
			names = append(names, fmt.Sprintf("%s (%s, %s)", a.Name, count(a.Files, "file"), count(len(a.Warnings), "warning")))
		}
		src := g.Source
		if len(src) > 12 {
			src = src[:12]
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", g.ID, g.Built.UTC().Format("2006-01-02 15:04Z"), src, strings.Join(names, ", "))
	}
	return tw.Flush()
}

func printDiff(w io.Writer, d []generation.Change) error {
	if len(d) == 0 {
		_, err := fmt.Fprintln(w, "no differences")
		return err
	}
	for _, c := range d {
		if _, err := fmt.Fprintf(w, "%-8s %s\n", c.Kind, c.Path); err != nil {
			return err
		}
	}
	return nil
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
