// Package prepush is the git pre-push gate: it refuses outgoing commits that
// carry employer material or secrets.
//
// It scans only what is being pushed (the commits between the remote's tip and
// the local one), so uncommitted work never blocks a push and an old commit is
// not re-judged on every push. Bypass deliberately with `git push --no-verify`.
package prepush

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

const zeroSHA = "0000000000000000000000000000000000000000"

// Options are the gate's inputs. Zero values are valid: no gitleaks, no drift
// check, default patterns.
type Options struct {
	Repo     string
	Patterns []string
	// Gitleaks scans the range given as git log options. It reports whether a
	// secret was found and whether gitleaks was available at all.
	Gitleaks func(logOpts string) (found, available bool)
	// Drift runs `aval sync --check`; a non-nil error means drift.
	Drift  func() (output string, err error)
	Strict bool // drift blocks the push instead of warning
}

// Finding is one pattern that matched the outgoing lines.
type Finding struct {
	Pattern string
	Hits    string // up to three "line:text" matches, newline-separated
}

// Scan matches every pattern, case-insensitively, against the added lines of a
// `git log -p` stream and keeps the first three hits per pattern.
func Scan(logPatch string, patterns []string) []Finding {
	var added []string
	for _, l := range strings.Split(logPatch, "\n") {
		if strings.HasPrefix(l, "+") {
			added = append(added, l)
		}
	}
	var out []Finding
	for _, p := range patterns {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			continue
		}
		var hits []string
		for i, l := range added {
			if re.MatchString(l) {
				hits = append(hits, fmt.Sprintf("%d:%s", i+1, l))
				if len(hits) == 3 {
					break
				}
			}
		}
		if len(hits) > 0 {
			out = append(out, Finding{Pattern: p, Hits: strings.Join(hits, "\n")})
		}
	}
	return out
}

// Run reads git's pre-push stdin (`<lref> <lsha> <rref> <rsha>` per line),
// reports problems on errOut and returns whether the push may go ahead.
func Run(in io.Reader, errOut io.Writer, opt Options) bool {
	patterns := opt.Patterns
	if patterns == nil {
		patterns = Patterns()
	}
	ok := true

	sc := bufio.NewScanner(in)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 4 || f[1] == zeroSHA { // branch deletion
			continue
		}
		lsha, rsha := f[1], f[3]
		rangeArgs := []string{rsha + ".." + lsha}
		if rsha == zeroSHA {
			rangeArgs = []string{lsha, "--not", "--remotes"}
		}

		patch, _ := exec.Command("git", append([]string{"-C", opt.Repo, "log", "-p", "--no-color"}, rangeArgs...)...).Output()
		for _, fd := range Scan(string(patch), patterns) {
			fmt.Fprintf(errOut, "pre-push: employer material matching /%s/ in outgoing commits:\n%s\n", fd.Pattern, fd.Hits)
			ok = false
		}

		if opt.Gitleaks != nil {
			if found, avail := opt.Gitleaks(strings.Join(rangeArgs, " ")); avail && found {
				fmt.Fprintln(errOut, "pre-push: gitleaks found a secret in outgoing commits (run gitleaks detect for details)")
				ok = false
			}
		}
	}

	// Drift between the repo and the live agent config is a WARNING by default:
	// a dev pushing from another machine has a different ~/.claude and must not
	// be blocked by it. AVAL_PREPUSH_STRICT=1 makes it blocking.
	if opt.Drift != nil {
		if out, err := opt.Drift(); err != nil {
			fmt.Fprintf(errOut, "pre-push: warning: aval sync --check found drift between the repo and your live config:\n%s\n", strings.TrimRight(out, "\n"))
			if opt.Strict {
				fmt.Fprintln(errOut, "pre-push: AVAL_PREPUSH_STRICT=1, so this blocks the push (run aval sync).")
				ok = false
			}
		}
	}

	if !ok {
		fmt.Fprintln(errOut, "pre-push: push refused. Fix the commits, or bypass deliberately with --no-verify.")
	}
	return ok
}
