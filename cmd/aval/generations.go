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
	"github.com/alvaroofernaandez/aval/internal/core/generation"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
	"github.com/alvaroofernaandez/aval/internal/core/txsync"
)

const generationsUsage = "aval generations <build | list | diff <a> <b> | activate <id> | rollback [<id>]> [--json]"

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
	switch {
	case args[0] == "build" && len(args) == 1:
		return generationsBuild(repoFlag, home, asJSON, c)
	case args[0] == "list" && len(args) == 1:
		return generationsList(home, asJSON, c)
	case args[0] == "diff" && len(args) == 3:
		return generationsDiff(home, args[1], args[2], asJSON, c)
	case args[0] == "activate" && len(args) == 2:
		return generationsActivate(repoFlag, home, args[1], asJSON, c)
	case args[0] == "rollback" && len(args) <= 2:
		return generationsRollback(repoFlag, home, args[1:], asJSON, c)
	}
	return newUsage(generationsUsage)
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
