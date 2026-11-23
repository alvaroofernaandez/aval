package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/devmode"
)

// runDev is `aval dev [shell|run|reset|ls|rm|diff|project] [--name N] [--nested]`.
// All the logic lives in internal/devmode; this file only wires the commands
// to it and to the install and sync code the sandbox exists to exercise.
func runDev(repoFlag string, args []string) error {
	sub := "shell"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs := newFlagSet("dev "+sub, os.Stderr)
	name := fs.String("name", "default", "sandbox name")
	nested := fs.Bool("nested", false, "allow `aval dev` inside a dev shell")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := devmode.GuardNested(os.Getenv, *nested); err != nil {
		return err
	}
	rest := fs.Args()

	realHome := os.Getenv("AVAL_DEV_REAL_HOME") // set inside a dev shell, so --nested still knows the real one
	if realHome == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		realHome = h
	}
	base := devmode.BaseDir(os.Getenv, realHome)

	switch sub {
	case "ls":
		return devLs(base)
	case "rm":
		return devmode.Remove(base, *name, realHome)
	case "diff":
		if os.Getenv("AVAL_DEV") == "1" {
			return errors.New("dev diff compares against the real machine: run it from your normal shell")
		}
		return devDiff(repoFlag, realHome, os.Stdout)
	case "shell", "run", "reset", "project":
	default:
		return fmt.Errorf("unknown dev command %q (shell, run, reset, ls, rm, diff, project)", sub)
	}

	repo, err := resolveRepo(repoFlag)
	if err != nil {
		return err
	}
	if sub == "reset" {
		if s, err := devmode.Open(base, *name, realHome); err == nil && exists(s.Root) {
			if err := devmode.Remove(base, *name, realHome); err != nil {
				return err
			}
		}
	}
	s, env, err := devmode.Prepare(devmode.PrepareOptions{
		Base: base, Name: *name, RealHome: realHome, SrcRepo: repo,
		Environ: os.Environ(), Out: os.Stdout, ErrOut: os.Stderr,
	})
	if err != nil {
		return err
	}

	switch sub {
	case "run":
		code, err := devmode.Run(s, env, rest, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			return err
		}
		if code != 0 {
			return exitCode(code)
		}
	case "project":
		if len(rest) != 1 {
			return errors.New("usage: aval dev project <path-to-git-repo>")
		}
		dest, err := devmode.Project(s, rest[0], env)
		if err != nil {
			return err
		}
		fmt.Printf("cloned into %s\nthe original was only read; the clone has no remote, so nothing pushed from it can reach the original or its upstream.\nuncommitted changes are not copied.\n", dest)
	case "shell":
		spec := devmode.Shell(s, runtime.GOOS, os.Getenv("SHELL"), exec.LookPath)
		if err := devmode.WriteFiles(s.Root, spec.Files); err != nil {
			return err
		}
		fmt.Printf("%s sandbox at %s; exit to leave it.\n", devmode.Marker(s.Name), s.Root)
		_, err := devmode.Run(s, append(env, spec.Env...), append([]string{spec.Path}, spec.Args...), os.Stdin, os.Stdout, os.Stderr)
		return err
	case "reset":
		fmt.Printf("sandbox %s recreated from scratch.\n", s.Name)
	}
	return nil
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func devLs(base string) error {
	list, err := devmode.List(base)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("no dev sandboxes (aval dev creates one)")
	}
	for _, i := range list {
		state := "ready"
		if !i.Ready {
			state = "incomplete"
		}
		fmt.Printf("%-16s %-10s %s\n", i.Name, state, i.Root)
	}
	return nil
}

// devDiff shows what `aval install --yes` and `aval sync` would change on the real
// machine, using the same dry-run paths as `aval --dry-run` and
// `aval sync --dry-run`. The symlink probe runs in a temp dir, not in HOME.
func devDiff(repoFlag, home string, out io.Writer) error {
	repo, err := resolveRepo(repoFlag)
	if err != nil {
		return err
	}
	_, p, pf, err := buildPlan(repo, home)
	if err != nil {
		return err
	}
	probe, err := os.MkdirTemp("", "aval-dev-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(probe)
	symlinks := pf.SupportsSymlinks(probe)

	fmt.Fprintf(out, "aval dev diff (read-only, nothing is written): what would change in %s\n\n== aval install --yes ==\n", home)
	if err := runHeadless(p, pf, install.New(install.Options{Home: home, Symlinks: symlinks, DryRun: true}), symlinks, true, uiSettings{}); err != nil {
		return err
	}
	fmt.Fprintln(out, "\n== aval sync ==")
	return runSync(repo, home, []string{"--dry-run"}, true, out)
}
