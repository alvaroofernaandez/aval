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

  status                 latest tags, unreleased commits, next suggested version
  patch | minor | major  cut a stable release
  rc | beta              cut the next prerelease of the upcoming version
  promote                turn the latest rc into the stable release (same commit)

  cut commands take --yes (skip the prompt) and --dry-run (show only)`

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
	switch kind := args[0]; kind {
	case "status":
		return releaseStatus(g, out)
	case "patch", "minor", "major", "rc", "beta", "promote":
		fs := flag.NewFlagSet("release "+kind, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		yes := fs.Bool("yes", false, "")
		dry := fs.Bool("dry-run", false, "")
		if err := fs.Parse(args[1:]); err != nil {
			return fmt.Errorf("%w\n%s", err, releaseUsage)
		}
		gate := newReleaseGate(g, wd)
		gate.SkipTests = *dry
		return release.Cut(g, release.CutOptions{
			Kind: kind, Yes: *yes, DryRun: *dry, In: in, Out: out, Gate: gate,
		})
	}
	return errors.New(releaseUsage)
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
