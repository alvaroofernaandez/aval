package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/agent/prepush"
)

// runPrePush is `aval hook pre-push [remote url]`, the body of .githooks/pre-push.
// `aval hook pre-push patterns` prints the employer-material patterns, one per
// line, for the shell suites that scan the tree and the history with them.
func runPrePush(args []string, in io.Reader, out io.Writer) error {
	if len(args) > 0 && args[0] == "patterns" {
		for _, p := range prepush.Patterns() {
			fmt.Fprintln(out, p)
		}
		return nil
	}

	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("pre-push: not inside a git work tree")
	}
	repo := strings.TrimSpace(string(top))

	opt := prepush.Options{
		Repo:     repo,
		Gitleaks: runGitleaks(repo),
		Drift:    prePushDrift(repo),
		Strict:   os.Getenv("AVAL_PREPUSH_STRICT") == "1",
	}
	if !prepush.Run(in, os.Stderr, opt) {
		return errChecksFailed // already reported
	}
	return nil
}

func runGitleaks(repo string) func(string) (bool, bool) {
	bin, err := exec.LookPath("gitleaks")
	if err != nil {
		return nil
	}
	return func(logOpts string) (bool, bool) {
		cmd := exec.Command(bin, "detect", "--no-banner", "--redact", "--log-opts="+logOpts)
		cmd.Dir = repo
		return cmd.Run() != nil, true
	}
}

// prePushDrift runs `aval --repo <repo> sync --check`. AVAL_BIN overrides the
// binary (tests); when it names a binary that is not there, there is nothing to
// check. Without it the running binary checks itself.
func prePushDrift(repo string) func() (string, error) {
	bin := os.Getenv("AVAL_BIN")
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil
		}
		bin = exe
	} else if p, err := exec.LookPath(bin); err != nil {
		return nil
	} else {
		bin = p
	}
	return func() (string, error) {
		out, err := exec.Command(bin, "--repo", repo, "sync", "--check").CombinedOutput()
		return string(out), err
	}
}
