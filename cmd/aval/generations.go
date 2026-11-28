package main

import (
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/claude"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/pi"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

const generationsUsage = "aval generations <build | list | diff <a> <b>> [--json]"

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
		gs, err := generation.List(home)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(c.stdout, nonNil(gs))
		}
		return printGenerations(c.stdout, gs)
	case args[0] == "diff" && len(args) == 3:
		d, err := generation.Diff(home, args[1], args[2])
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(c.stdout, nonNil(d))
		}
		return printDiff(c.stdout, d)
	}
	return newUsage(generationsUsage)
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
		Adapters: []agentcfg.Adapter{claude.New(claude.Options{Home: home}), pi.New()},
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
