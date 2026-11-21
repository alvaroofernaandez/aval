package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/release"
)

const releaseUsage = `usage: aval release <command>

  status                    latest tags, unreleased commits, next suggested version
  next [--pre rc|beta]      print the next version and its release notes
  tag  [--pre rc|beta]      checks, then create the annotated tag locally (no push)

  patch | minor | major     cut a stable release
  rc | beta                 cut the next prerelease of the upcoming version
  promote                   turn the latest rc into the stable release, same commit

  cut commands take --yes (skip the prompt), --dry-run (show only), --no-push`

// newReleaseGate builds the checks a release must pass. A variable so tests
// can run without invoking the real test suite or gh.
var newReleaseGate = func(g release.Git, wd string) release.Gate {
	gate := release.Gate{Git: g, Remote: "origin", Branch: "main"}
	gate.RunTests = func() error {
		cmd := exec.Command("go", "test", "./...")
		cmd.Dir = wd
		if out, err := cmd.CombinedOutput(); err != nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) > 15 {
				lines = lines[len(lines)-15:]
			}
			return fmt.Errorf("%w\n%s", err, strings.Join(lines, "\n"))
		}
		return nil
	}
	gate.CI = func(sha string) (string, error) {
		if _, err := exec.LookPath("gh"); err != nil {
			return "", fmt.Errorf("%w: gh not installed", release.ErrNoCI)
		}
		cmd := exec.Command("gh", "run", "list", "--commit", sha, "--workflow", "ci.yml",
			"--json", "status,conclusion", "--limit", "5")
		cmd.Dir = wd
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("%w: gh could not list runs", release.ErrNoCI)
		}
		return ciVerdict(sha, out)
	}
	return gate
}

// ciVerdict reads `gh run list --json status,conclusion` output, newest run
// first. It returns "" when CI is green, otherwise why it is not.
func ciVerdict(sha string, raw []byte) (string, error) {
	var runs []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	if err := json.Unmarshal(raw, &runs); err != nil {
		return "", fmt.Errorf("unreadable gh output: %w", err)
	}
	if len(runs) == 0 {
		return fmt.Sprintf("no CI run found for %.7s", sha), nil
	}
	r := runs[0]
	switch {
	case r.Status != "completed":
		return "CI is still running", nil
	case r.Conclusion != "success":
		return "CI concluded " + r.Conclusion, nil
	}
	return "", nil
}

func runRelease(wd string, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(releaseUsage)
	}
	g := release.NewGit(wd)
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "status":
		return releaseStatus(g, out)
	case "next", "tag":
		fs := flag.NewFlagSet("release "+cmd, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		pre := fs.String("pre", "", "rc or beta")
		if err := fs.Parse(rest); err != nil {
			return fmt.Errorf("%w\n%s", err, releaseUsage)
		}
		kind := "auto"
		if *pre != "" {
			if *pre != "rc" && *pre != "beta" {
				return fmt.Errorf("--pre must be rc or beta, got %q", *pre)
			}
			kind = *pre
		}
		if cmd == "next" {
			return releaseNext(g, kind, out)
		}
		return release.Cut(g, release.CutOptions{
			Kind: kind, Yes: true, NoPush: true, In: in, Out: out, Gate: newReleaseGate(g, wd),
		})
	case "patch", "minor", "major", "rc", "beta", "promote":
		fs := flag.NewFlagSet("release "+cmd, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		yes := fs.Bool("yes", false, "")
		dry := fs.Bool("dry-run", false, "")
		noPush := fs.Bool("no-push", false, "")
		if err := fs.Parse(rest); err != nil {
			return fmt.Errorf("%w\n%s", err, releaseUsage)
		}
		gate := newReleaseGate(g, wd)
		gate.SkipTests = *dry
		return release.Cut(g, release.CutOptions{
			Kind: cmd, Yes: *yes, DryRun: *dry, NoPush: *noPush, In: in, Out: out, Gate: gate,
		})
	}
	return errors.New(releaseUsage)
}

func releaseNext(g release.Git, kind string, out io.Writer) error {
	st, err := release.Inspect(g)
	if err != nil {
		return err
	}
	p, err := release.Resolve(g, st, kind)
	if err != nil {
		return err
	}
	channel := release.ChannelStable
	if p.Version.IsPre() {
		channel = release.ChannelPrerelease
	}
	fmt.Fprintf(out, "%s (%s)\n\n", p.Version.Tag(), channel)
	if p.Notes == "" {
		fmt.Fprintln(out, "(no user-facing changes to list)")
		return nil
	}
	fmt.Fprint(out, p.Notes)
	return nil
}

func releaseStatus(g release.Git, out io.Writer) error {
	st, err := release.Inspect(g)
	if err != nil {
		return err
	}
	stable := release.LastStable(st.Tags)
	stableName := "none yet"
	if stable != (release.Version{}) {
		stableName = stable.Tag()
	}
	fmt.Fprintf(out, "Latest stable:      %s\n", stableName)

	var pre release.Version
	hasPre := false
	for _, t := range st.Tags {
		if t.IsPre() && t.Compare(stable) > 0 && (!hasPre || t.Compare(pre) > 0) {
			pre, hasPre = t, true
		}
	}
	if hasPre {
		fmt.Fprintf(out, "Latest prerelease:  %s (not promoted)\n", pre.Tag())
	}

	since := "the start"
	if st.HasLastTag {
		since = st.LastTag.Tag()
	}
	fmt.Fprintf(out, "Unreleased:         %d commit(s) since %s\n", len(st.SinceTag), since)

	if next, err := release.Next(st.Tags, st.Unreleased, ""); err == nil {
		fmt.Fprintf(out, "Next suggested:     %s  (aval release %s | rc)\n", next.Tag(), suggestedLevel(stable, next))
	} else if errors.Is(err, release.ErrNothingToRelease) {
		fmt.Fprintln(out, "Next suggested:     nothing to release (no feat, fix or perf since the last stable)")
	} else {
		return err
	}
	if hasPre && strings.HasPrefix(pre.Pre, "rc.") {
		fmt.Fprintf(out, "Promote with:       aval release promote  (%s -> %s)\n", pre.Tag(),
			release.Version{Major: pre.Major, Minor: pre.Minor, Patch: pre.Patch}.Tag())
	}
	return nil
}

func suggestedLevel(from, to release.Version) string {
	switch {
	case to.Major != from.Major:
		return "major"
	case to.Minor != from.Minor:
		return "minor"
	}
	return "patch"
}

var _ = os.Stdout
